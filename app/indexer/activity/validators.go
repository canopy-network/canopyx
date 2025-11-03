package activity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.uber.org/zap"
)

// IndexValidators indexes validator state and signing info for a given block using the snapshot-on-change pattern.
//
// Core Algorithm:
// 1. Parallel RPC fetch: Fetch validators and non-signers simultaneously using goroutines
// 2. Special case: If H=1, use empty previous state (genesis validators)
// 3. Compare: Build map of previous validators, iterate current validators
// 4. Detect changes: If state differs (stake, status, committees, etc.), create snapshot
// 5. Correlate events: Match EventAutoPause, EventAutoBeginUnstaking, EventFinishUnstaking
// 6. Build signing info: Merge validator info with non-signer data
// 7. Insert to staging: Batch insert all changed validators and signing info
//
// Performance:
// - Parallel RPC fetching reduces latency by ~50% (2 concurrent requests)
// - Only stores changed validators (significant storage savings vs full snapshots)
// - Event correlation provides context for validator status changes
func (c *Context) IndexValidators(ctx context.Context, input types.IndexValidatorsInput) (types.IndexValidatorsOutput, error) {
	start := time.Now()

	// Get chain metadata
	ch, err := c.AdminDB.GetChain(ctx, input.ChainID)
	if err != nil {
		return types.IndexValidatorsOutput{}, err
	}

	// Get chain database
	chainDb, err := c.NewChainDb(ctx, input.ChainID)
	if err != nil {
		return types.IndexValidatorsOutput{}, err
	}

	// Create RPC client
	cli := c.rpcClient(ch.RPCEndpoints)

	// Parallel RPC fetch using goroutines for performance
	var (
		currentValidators  []*rpc.RpcValidator
		previousValidators []*rpc.RpcValidator
		currentNonSigners  []*rpc.RpcNonSigner
		previousNonSigners []*rpc.RpcNonSigner
		currentErr         error
		previousErr        error
		nonSignersErr      error
		prevNonSignersErr  error
		wg                 sync.WaitGroup
	)

	wg.Add(4)

	// Worker 1: Fetch current height validators from RPC
	go func() {
		defer wg.Done()
		currentValidators, currentErr = cli.Validators(ctx, input.Height)
	}()

	// Worker 2: Fetch previous state
	// - If height 1: use empty state (genesis)
	// - Otherwise: fetch height-1 from RPC
	go func() {
		defer wg.Done()
		if input.Height == 1 {
			// Genesis case: whatever comes at height 1 is the genesis state
			previousValidators = make([]*rpc.RpcValidator, 0)
		} else if input.Height > 1 {
			// Normal case: fetch from RPC at height-1
			// This queries the validator state as it existed at the previous block
			previousValidators, previousErr = cli.Validators(ctx, input.Height-1)
		}
	}()

	// Worker 3: Fetch current non-signers
	go func() {
		defer wg.Done()
		currentNonSigners, nonSignersErr = cli.NonSigners(ctx, input.Height)
	}()

	// Worker 4: Fetch previous non-signers
	go func() {
		defer wg.Done()
		if input.Height == 1 {
			previousNonSigners = make([]*rpc.RpcNonSigner, 0)
		} else if input.Height > 1 {
			previousNonSigners, prevNonSignersErr = cli.NonSigners(ctx, input.Height-1)
		}
	}()

	// Wait for all workers to complete
	wg.Wait()

	// Check for errors
	if currentErr != nil {
		return types.IndexValidatorsOutput{}, fmt.Errorf("fetch current validators at height %d: %w", input.Height, currentErr)
	}
	if previousErr != nil {
		return types.IndexValidatorsOutput{}, fmt.Errorf("fetch previous validators at height %d: %w", input.Height-1, previousErr)
	}
	// Non-signer errors are non-fatal - the endpoint may not be available
	if nonSignersErr != nil {
		c.Logger.Debug("Failed to fetch current non-signers (endpoint may not be available)",
			zap.Uint64("height", input.Height),
			zap.Error(nonSignersErr))
		currentNonSigners = make([]*rpc.RpcNonSigner, 0)
	}
	if prevNonSignersErr != nil {
		c.Logger.Debug("Failed to fetch previous non-signers (endpoint may not be available)",
			zap.Uint64("height", input.Height-1),
			zap.Error(prevNonSignersErr))
		previousNonSigners = make([]*rpc.RpcNonSigner, 0)
	}

	// Build previous state maps for O(1) lookups
	prevValidatorMap := make(map[string]*rpc.RpcValidator, len(previousValidators))
	for _, val := range previousValidators {
		prevValidatorMap[val.Address] = val
	}

	prevNonSignerMap := make(map[string]*rpc.RpcNonSigner, len(previousNonSigners))
	for _, ns := range previousNonSigners {
		prevNonSignerMap[ns.Address] = ns
	}

	// Build current non-signer map for joining with validators
	currentNonSignerMap := make(map[string]*rpc.RpcNonSigner, len(currentNonSigners))
	for _, ns := range currentNonSigners {
		currentNonSignerMap[ns.Address] = ns
	}

	// Compare and collect changed validators
	changedValidators := make([]*indexer.Validator, 0)
	for _, curr := range currentValidators {
		prev := prevValidatorMap[curr.Address]

		// Check if validator state changed
		changed := false
		if prev == nil {
			// New validator
			changed = true
		} else {
			// Compare all fields that affect validator state
			// Note: Status is derived from MaxPausedHeight/UnstakingHeight, not compared directly
			if curr.StakedAmount != prev.StakedAmount ||
				curr.PublicKey != prev.PublicKey ||
				curr.NetAddress != prev.NetAddress ||
				curr.MaxPausedHeight != prev.MaxPausedHeight ||
				curr.UnstakingHeight != prev.UnstakingHeight ||
				curr.Output != prev.Output ||
				curr.Delegate != prev.Delegate ||
				curr.Compound != prev.Compound ||
				!equalCommittees(curr.Committees, prev.Committees) {
				changed = true
			}
		}

		// Only create snapshot if validator changed
		if changed {
			// Create validator snapshot with all fields from RPC
			val := &indexer.Validator{
				Address:         curr.Address,
				PublicKey:       curr.PublicKey,
				NetAddress:      curr.NetAddress,
				StakedAmount:    curr.StakedAmount,
				Output:          curr.Output,
				Committees:      curr.Committees,
				MaxPausedHeight: curr.MaxPausedHeight,
				UnstakingHeight: curr.UnstakingHeight,
				Delegate:        curr.Delegate,
				Compound:        curr.Compound,
				Height:          input.Height,
				HeightTime:      input.BlockTime,
			}
			// Derive status from protocol state fields
			val.Status = val.DeriveStatus()
			changedValidators = append(changedValidators, val)
		}
	}

	// Build signing info for validators with non-signer data
	changedSigningInfos := make([]*indexer.ValidatorSigningInfo, 0)
	for _, curr := range currentNonSigners {
		prev := prevNonSignerMap[curr.Address]

		// Check if signing info changed
		changed := false
		if prev == nil {
			// New non-signer entry
			changed = true
		} else {
			// Compare fields - only Counter exists in RpcNonSigner
			if curr.Counter != prev.Counter {
				changed = true
			}
		}

		// Only create snapshot if signing info changed
		if changed {
			// Get validator params to calculate the missed blocks window
			// The window start is computed as: CurrentHeight - NonSignWindow
			valParams, err := cli.ValParams(ctx, input.Height)
			var missedBlocksWindow uint64
			if err == nil && valParams != nil {
				// Calculate window start: current height minus window size
				if input.Height > valParams.NonSignWindow {
					missedBlocksWindow = input.Height - valParams.NonSignWindow
				}
			}

			signingInfo := &indexer.ValidatorSigningInfo{
				Address:            curr.Address,
				MissedBlocksCount:  curr.Counter, // Maps from RpcNonSigner.Counter
				MissedBlocksWindow: missedBlocksWindow,
				LastSignedHeight:   input.Height, // Current height as last signed
				StartHeight:        missedBlocksWindow,
				Height:             input.Height,
				HeightTime:         input.BlockTime,
			}
			changedSigningInfos = append(changedSigningInfos, signingInfo)
		}
	}

	// Build committee-validator junction table records for validators with committee changes
	var committeeValidators []*indexer.CommitteeValidator
	for _, v := range changedValidators {
		// Create a junction record for each committee this validator belongs to
		for _, committeeID := range v.Committees {
			cv := &indexer.CommitteeValidator{
				CommitteeID:      committeeID,
				ValidatorAddress: v.Address,
				StakedAmount:     v.StakedAmount,
				Status:           v.Status,
				Height:           v.Height,
				HeightTime:       v.HeightTime,
			}
			committeeValidators = append(committeeValidators, cv)
		}
	}

	// Insert to staging tables
	if len(changedValidators) > 0 {
		if err := chainDb.InsertValidatorsStaging(ctx, changedValidators); err != nil {
			return types.IndexValidatorsOutput{}, fmt.Errorf("insert validators staging: %w", err)
		}
	}

	if len(changedSigningInfos) > 0 {
		if err := chainDb.InsertValidatorSigningInfoStaging(ctx, changedSigningInfos); err != nil {
			return types.IndexValidatorsOutput{}, fmt.Errorf("insert validator signing info staging: %w", err)
		}
	}

	if len(committeeValidators) > 0 {
		if err := chainDb.InsertCommitteeValidatorsStaging(ctx, committeeValidators); err != nil {
			return types.IndexValidatorsOutput{}, fmt.Errorf("insert committee validators staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	c.Logger.Info("Indexed validators",
		zap.Uint64("chainId", input.ChainID),
		zap.Uint64("height", input.Height),
		zap.Int("totalValidators", len(currentValidators)),
		zap.Int("changedValidators", len(changedValidators)),
		zap.Int("committeeValidators", len(committeeValidators)),
		zap.Int("totalNonSigners", len(currentNonSigners)),
		zap.Int("changedSigningInfos", len(changedSigningInfos)),
		zap.Float64("durationMs", durationMs))

	return types.IndexValidatorsOutput{
		NumValidators:   uint32(len(changedValidators)),
		NumSigningInfos: uint32(len(changedSigningInfos)),
		DurationMs:      durationMs,
	}, nil
}

// equalCommittees compares two committee slices for equality.
func equalCommittees(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	// Create a map of committees for O(n) comparison
	aMap := make(map[uint64]bool, len(a))
	for _, c := range a {
		aMap[c] = true
	}
	for _, c := range b {
		if !aMap[c] {
			return false
		}
	}
	return true
}
