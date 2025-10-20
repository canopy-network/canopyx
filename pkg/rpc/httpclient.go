package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
)

const (
	headPath           = "/v1/query/height"
	blockByHeightPath  = "/v1/query/block-by-height"
	txsByHeightPath    = "/v1/query/txs-by-height"
	eventsByHeightPath = "/v1/query/events-by-height"
	dexPricePath       = "/v1/query/dex-price"
	dexPricesPath      = "/v1/query/dex-prices"
)

// HTTPClient is a wrapper around an http.Client that implements a circuit-breaker and token-bucket.
type HTTPClient struct {
	endpoints []string
	client    *http.Client

	// token-bucket
	tokens      int64
	maxTokens   int64
	refillEvery time.Duration
	lastRefill  atomic.Value // time.Time

	// circuit-breaker
	mu       sync.Mutex
	failures map[string]int
	opened   map[string]time.Time

	breakerThreshold int
	breakerCooldown  time.Duration
}

// Opts is the set of options for a new HTTPClient.
type Opts struct {
	Endpoints       []string
	Timeout         time.Duration
	RPS             int
	Burst           int
	BreakerFailures int
	BreakerCooldown time.Duration
}

// NewHTTPWithOpts creates a new HTTPClient with the given options.
func NewHTTPWithOpts(o Opts) *HTTPClient {
	if o.RPS <= 0 {
		o.RPS = 20
	}
	if o.Burst <= 0 {
		o.Burst = 40
	}
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Second
	}
	if o.BreakerFailures <= 0 {
		o.BreakerFailures = 3
	}
	if o.BreakerCooldown <= 0 {
		o.BreakerCooldown = 5 * time.Second
	}

	c := &HTTPClient{
		endpoints:        utils.Dedup(o.Endpoints),
		client:           &http.Client{Timeout: o.Timeout},
		maxTokens:        int64(o.Burst),
		refillEvery:      time.Second / time.Duration(o.RPS),
		failures:         map[string]int{},
		opened:           map[string]time.Time{},
		breakerThreshold: o.BreakerFailures,
		breakerCooldown:  o.BreakerCooldown,
	}
	c.tokens = c.maxTokens
	c.lastRefill.Store(time.Now())
	return c
}

// refill refills the token-bucket with new tokens if necessary.
func (c *HTTPClient) refill() {
	last := c.lastRefill.Load().(time.Time)
	now := time.Now()
	if now.Sub(last) >= c.refillEvery {
		if atomic.LoadInt64(&c.tokens) < c.maxTokens {
			atomic.AddInt64(&c.tokens, 1)
		}
		c.lastRefill.Store(now)
	}
}

// acquire acquires a token from the token-bucket, blocking if necessary.
func (c *HTTPClient) acquire() {
	for {
		c.refill()
		if atomic.LoadInt64(&c.tokens) > 0 {
			atomic.AddInt64(&c.tokens, -1)
			return
		}
		time.Sleep(c.refillEvery / 2)
	}
}

// isOpen returns true if the endpoint is not in the OPEN state.
func (c *HTTPClient) isOpen(ep string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	until, ok := c.opened[ep]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(c.opened, ep)
		c.failures[ep] = 0
		return false
	}
	return true
}

// noteFailure marks an endpoint as failed and opens the circuit-breaker if the failure count exceeds the threshold.
func (c *HTTPClient) noteFailure(ep string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures[ep]++
	if c.failures[ep] >= c.breakerThreshold {
		c.opened[ep] = time.Now().Add(c.breakerCooldown)
	}
}

// doJSON sends an HTTP request to a configured endpoint with the given method, path, and JSON payload and processes the response.
// It retries across multiple endpoints if the primary attempt fails due to circuit-breaker or server-side errors.
// The response body is optionally unmarshalled into the `out` parameter if provided, and JSON decoding errors are returned.
// Returns an error if the request creation, response handling, or circuit-breaker exceeds available endpoints.
func (c *HTTPClient) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	if len(c.endpoints) == 0 {
		return fmt.Errorf("no endpoints configured")
	}

	var lastErr error
	for i := 0; i < len(c.endpoints); i++ {
		ep := c.endpoints[i%len(c.endpoints)]
		// Skip endpoints whose breaker is OPEN.
		if c.isOpen(ep) {
			continue
		}

		c.acquire()

		var body *bytes.Reader
		if payload != nil {
			b, mErr := json.Marshal(payload)
			if mErr != nil {
				// Fatal for this attempt; don't mark the endpoint as failed.
				return mErr
			}
			body = bytes.NewReader(b)
		} else {
			body = bytes.NewReader(nil)
		}

		req, reqErr := http.NewRequestWithContext(ctx, method, ep+path, body)
		if reqErr != nil {
			// Request creation failed: not an endpoint failure, just return.
			return reqErr
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			c.noteFailure(ep)
			continue
		}

		// From here on, always drain+close the body before continuing/returning.
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server %d", resp.StatusCode)
			c.noteFailure(ep)
			_ = utils.DrainAndClose(resp.Body)
			continue
		}
		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			_ = utils.DrainAndClose(resp.Body)
			continue
		}

		if out != nil {
			// decode into raw first
			var raw json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
				_ = utils.DrainAndClose(resp.Body)
				lastErr = err
				continue
			}

			// raw can be a primitive (like `12345`) or an object
			// re-unmarshal accordingly into your out
			if len(raw) > 0 && raw[0] != '{' && raw[0] != '[' {
				// likely a bare number/string → wrap into object if your `out` expects struct
				// e.g. {"height": <raw>}
				wrapped := []byte(`{"height":` + string(raw) + `}`)
				if err := json.Unmarshal(wrapped, out); err != nil {
					lastErr = err
					continue
				}
			} else {
				if err := json.Unmarshal(raw, out); err != nil {
					lastErr = err
					continue
				}
			}
		}

		// Success: drain+close (close error rarely actionable; log if you want).
		if cerr := utils.DrainAndClose(resp.Body); cerr != nil && lastErr == nil {
			lastErr = cerr
		}
		return lastErr
	}

	return lastErr
}

// Generic pagination: free function to avoid method type params

// pageResp is the response for a paged query.
type pageResp[T any] struct {
	PageNumber int    `json:"pageNumber"`
	PerPage    int    `json:"perPage"`
	Results    []T    `json:"results"`
	Count      int    `json:"count"`
	TotalPages int    `json:"totalPages"`
	TotalCount int    `json:"totalCount"`
	Type       string `json:"type"`
}

// ListPaged lists all pages of a given path
func ListPaged[T any](ctx context.Context, c *HTTPClient, path string, args map[string]any) ([]T, error) {
	var first pageResp[T]
	if err := c.doJSON(ctx, http.MethodPost, path, args, &first); err != nil {
		return nil, err
	}
	all := make([]T, 0, first.TotalCount)
	all = append(all, first.Results...)
	if first.TotalPages <= 1 {
		return all, nil
	}
	type res struct {
		items []T
		err   error
	}
	ch := make(chan res, first.TotalPages-1)
	for p := 2; p <= first.TotalPages; p++ {
		go func(page int) {
			var pr pageResp[T]
			payload := map[string]any{}
			for k, v := range args {
				payload[k] = v
			}
			payload["pageNumber"] = page
			if err := c.doJSON(ctx, http.MethodPost, path, payload, &pr); err != nil {
				ch <- res{nil, err}
				return
			}
			ch <- res{pr.Results, nil}
		}(p)
	}
	for i := 0; i < first.TotalPages-1; i++ {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		all = append(all, r.items...)
	}
	return all, nil
}
