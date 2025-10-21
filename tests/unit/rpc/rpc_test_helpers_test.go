package rpc_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/canopy-network/canopyx/pkg/rpc"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestRPCClient(handler http.Handler) *rpc.HTTPClient {
	return newTestRPCClientWithOpts(handler, rpc.Opts{})
}

func newTestRPCClientWithOpts(handler http.Handler, opts rpc.Opts) *rpc.HTTPClient {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			resp := rec.Result()
			if resp.Body == nil {
				resp.Body = http.NoBody
			}
			return resp, nil
		}),
		Timeout: 5 * time.Second,
	}

	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	if len(opts.Endpoints) == 0 {
		opts.Endpoints = []string{"http://mock"}
	}
	opts.HTTPClient = httpClient

	return rpc.NewHTTPWithOpts(opts)
}
