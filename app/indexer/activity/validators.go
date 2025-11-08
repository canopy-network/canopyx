package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopyx/app/indexer/types"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.uber.org/zap"
)

// IndexValidators indexes validator state and signing info for a given block using the snapshot-on-change pattern.
//
// Core Algorithm:
// 1. Parallel RPC fetch: Fetch validators and non-signers simultaneously using goroutines
// 2. Special case: If H=1, use an empty previous state (genesis validators)
// 3. Compare: Build a map of previous validators, iterate current validators
// 4. Detect changes: If the state differs (stake, status, committees, etc.), create snapshot
// 5. Correlate events: Match EventAutoPause, EventAutoBeginUnstaking, EventFinishUnstaking
// 6. Build signing info: Merge validator info with non-signer data
// 7. Insert to staging: Batch inserts all changed validators and signing info
//
// Performance:
// - Parallel RPC fetching reduces latency by ~50% (2 concurrent requests)
// - Only stores changed validators (significant storage savings vs full snapshots)
// - Event correlation provides context for validator status changes
func (ac *Context) IndexValidators(ctx context.Context, input types.ActivityIndexAtHeight) (types.ActivityIndexValidatorsOutput, error) {
	start := time.Now()

	// Get chain metadata
	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexValidatorsOutput{}, err
	}

	// Get chain database
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityIndexValidatorsOutput{}, err
	}

	// Parallel RPC fetch using shared worker pool for performance
	var (
		currentValidators     []*rpc.RpcValidator
		previousValidators    []*rpc.RpcValidator
		currentNonSigners     []*rpc.RpcNonSigner
		previousNonSigners    []*rpc.RpcNonSigner
		currentDoubleSigners  []*rpc.RpcDoubleSigner
		previousDoubleSigners []*rpc.RpcDoubleSigner
		currentErr            error
		previousErr           error
		nonSignersErr         error
		prevNonSignersErr     error
		doubleSignersErr      error
		prevDoubleSignersErr  error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(6)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch current height validators from RPC
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		currentValidators, currentErr = cli.ValidatorsByHeight(groupCtx, input.Height)
	})

	// Worker 2: Fetch previous state
	// - If height 1: use empty state (genesis)
	// - Otherwise: fetch height-1 from RPC
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if input.Height == 1 {
			// Genesis case: whatever comes at height 1 is the genesis state
			previousValidators = make([]*rpc.RpcValidator, 0)
		} else if input.Height > 1 {
			// Normal case: fetch from RPC at height-1
			// This queries the validator state as it existed at the previous block
			previousValidators, previousErr = cli.ValidatorsByHeight(groupCtx, input.Height-1)
		}
	})

	// Worker 3: Fetch current non-signers
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		currentNonSigners, nonSignersErr = cli.NonSignersByHeight(groupCtx, input.Height)
	})

	// Worker 4: Fetch previous non-signers
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if input.Height == 1 {
			previousNonSigners = make([]*rpc.RpcNonSigner, 0)
		} else if input.Height > 1 {
			previousNonSigners, prevNonSignersErr = cli.NonSignersByHeight(groupCtx, input.Height-1)
		}
	})

	// Worker 5: Fetch current double-signers
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		currentDoubleSigners, doubleSignersErr = cli.DoubleSignersByHeight(groupCtx, input.Height)
	})

	// Worker 6: Fetch previous double-signers
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if input.Height == 1 {
			previousDoubleSigners = make([]*rpc.RpcDoubleSigner, 0)
		} else if input.Height > 1 {
			previousDoubleSigners, prevDoubleSignersErr = cli.DoubleSignersByHeight(groupCtx, input.Height-1)
		}
	})

	// Wait for all workers to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", input.Height),
			zap.Error(err),
		)
	}

	// Check for errors - all RPC calls must succeed to ensure data integrity
	if currentErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("fetch current validators at height %d: %w", input.Height, currentErr)
	}
	if previousErr != nil && input.Height > 1 {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("fetch previous validators at height %d: %w", input.Height-1, previousErr)
	}
	if nonSignersErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("fetch current non-signers at height %d: %w", input.Height, nonSignersErr)
	}
	if prevNonSignersErr != nil && input.Height > 1 {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("fetch previous non-signers at height %d: %w", input.Height-1, prevNonSignersErr)
	}
	if doubleSignersErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("fetch current double-signers at height %d: %w", input.Height, doubleSignersErr)
	}
	if prevDoubleSignersErr != nil && input.Height > 1 {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("fetch previous double-signers at height %d: %w", input.Height-1, prevDoubleSignersErr)
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

	// Query validator lifecycle events from staging table (event-driven state tracking)
	// These events define state transitions in the validator lifecycle:
	// - EventTypeAutomaticPause: validator transitions to paused state
	// - EventTypeAutomaticBeginUnstaking: validator transitions to unstaking state
	// - EventTypeAutomaticFinishUnstaking: validator deleted (unstaked)
	// - EventTypeSlash: validator slashed (affects staked amount)
	// - EventTypeReward: validator rewarded (informational, doesn't change state)
	validatorEvents, err := chainDb.GetEventsByTypeAndHeight(
		ctx, input.Height, true,
		rpc.EventTypeAsStr(rpc.EventTypeReward),
		rpc.EventTypeAsStr(rpc.EventTypeSlash),
		rpc.EventTypeAsStr(rpc.EventTypeAutomaticPause),
		rpc.EventTypeAsStr(rpc.EventTypeAutomaticBeginUnstaking),
		rpc.EventTypeAsStr(rpc.EventTypeAutomaticFinishUnstaking),
	)
	if err != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("query validator events at height %d: %w", input.Height, err)
	}

	// Build event maps by validator address for O(1) lookup
	rewardEvents := make(map[string]*indexer.Event)
	pauseEvents := make(map[string]*indexer.Event)
	beginUnstakingEvents := make(map[string]*indexer.Event)
	finishUnstakingEvents := make(map[string]*indexer.Event)
	slashEvents := make(map[string]*indexer.Event)

	for _, event := range validatorEvents {
		// Events have an Address field for the validator address
		addr := event.Address

		switch event.EventType {
		case "automatic-pause":
			pauseEvents[addr] = event
		case "automatic-begin-unstaking":
			beginUnstakingEvents[addr] = event
		case "automatic-finish-unstaking":
			finishUnstakingEvents[addr] = event
		case "slash":
			slashEvents[addr] = event
		case "reward":
			rewardEvents[addr] = event
		}
	}

	// Compare and collect changed validators
	changedValidators := make([]*indexer.Validator, 0)
	for _, curr := range currentValidators {
		prev := prevValidatorMap[curr.Address]

		// Check if validator state changed
		changed := false
		hasEvent := false

		// Check for lifecycle events (state transitions)
		if _, hasPause := pauseEvents[curr.Address]; hasPause {
			changed = true
			hasEvent = true
		}
		if _, hasBeginUnstake := beginUnstakingEvents[curr.Address]; hasBeginUnstake {
			changed = true
			hasEvent = true
		}
		if _, hasSlash := slashEvents[curr.Address]; hasSlash {
			changed = true
			hasEvent = true
		}
		if _, hasReward := rewardEvents[curr.Address]; hasReward {
			changed = true
			hasEvent = true
		}

		if prev == nil {
			// New validator
			changed = true
		} else if !hasEvent {
			// Only compare RPC fields if no event occurred
			// If an event occurred, we already marked changed=true above
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
			valParams, err := cli.ValParamsByHeight(ctx, input.Height)
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

	// Build previous double-signers map for O(1) lookups
	prevDoubleSignersMap := make(map[string]uint64, len(previousDoubleSigners))
	for _, ds := range previousDoubleSigners {
		prevDoubleSignersMap[ds.Address] = uint64(len(ds.InfractionHeights))
	}

	// Compare and collect changed double-signing info
	changedDoubleSigningInfos := make([]*indexer.ValidatorDoubleSigningInfo, 0)
	for _, curr := range currentDoubleSigners {
		currentCount := uint64(len(curr.InfractionHeights))
		prevCount := prevDoubleSignersMap[curr.Address]

		// Snapshot-on-change: only insert if evidence count changed
		if currentCount != prevCount {
			var firstHeight, lastHeight uint64
			if len(curr.InfractionHeights) > 0 {
				firstHeight = curr.InfractionHeights[0]
				lastHeight = curr.InfractionHeights[len(curr.InfractionHeights)-1]
			}

			info := &indexer.ValidatorDoubleSigningInfo{
				Address:             curr.Address,
				EvidenceCount:       currentCount,
				FirstEvidenceHeight: firstHeight,
				LastEvidenceHeight:  lastHeight,
				Height:              input.Height,
				HeightTime:          input.BlockTime,
			}
			changedDoubleSigningInfos = append(changedDoubleSigningInfos, info)
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
			return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("insert validators staging: %w", err)
		}
	}

	if len(changedSigningInfos) > 0 {
		if err := chainDb.InsertValidatorSigningInfoStaging(ctx, changedSigningInfos); err != nil {
			return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("insert validator signing info staging: %w", err)
		}
	}

	if len(committeeValidators) > 0 {
		if err := chainDb.InsertCommitteeValidatorsStaging(ctx, committeeValidators); err != nil {
			return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("insert committee validators staging: %w", err)
		}
	}

	if len(changedDoubleSigningInfos) > 0 {
		if err := chainDb.InsertValidatorDoubleSigningInfoStaging(ctx, changedDoubleSigningInfos); err != nil {
			return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("insert validator double signing info staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Indexed validators",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", input.Height),
		zap.Int("totalValidators", len(currentValidators)),
		zap.Int("changedValidators", len(changedValidators)),
		zap.Int("committeeValidators", len(committeeValidators)),
		zap.Int("totalNonSigners", len(currentNonSigners)),
		zap.Int("changedSigningInfos", len(changedSigningInfos)),
		zap.Int("totalDoubleSigners", len(currentDoubleSigners)),
		zap.Int("changedDoubleSigningInfos", len(changedDoubleSigningInfos)),
		zap.Int("pauseEvents", len(pauseEvents)),
		zap.Int("beginUnstakingEvents", len(beginUnstakingEvents)),
		zap.Int("finishUnstakingEvents", len(finishUnstakingEvents)),
		zap.Int("slashEvents", len(slashEvents)),
		zap.Int("rewardEvents", len(rewardEvents)),
		zap.Float64("durationMs", durationMs))

	return types.ActivityIndexValidatorsOutput{
		NumValidators:         uint32(len(changedValidators)),
		NumSigningInfos:       uint32(len(changedSigningInfos)),
		NumDoubleSigningInfos: uint32(len(changedDoubleSigningInfos)),
		DurationMs:            durationMs,
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
