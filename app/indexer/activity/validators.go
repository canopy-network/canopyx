package activity

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/app/indexer/types"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
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

	// Get RPC client with height-aware endpoint selection
	cli, err := ac.rpcClientForHeight(ctx, input.Height)
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
		currentValidators       []*fsm.Validator
		previousValidators      []*fsm.Validator
		currentNonSigners       []*fsm.NonSigner
		previousNonSigners      []*fsm.NonSigner
		currentDoubleSigners    []*lib.DoubleSigner
		previousDoubleSigners   []*lib.DoubleSigner
		subsidizedCommittees    []uint64
		retiredCommittees       []uint64
		currentErr              error
		previousErr             error
		nonSignersErr           error
		prevNonSignersErr       error
		doubleSignersErr        error
		prevDoubleSignersErr    error
		subsidizedCommitteesErr error
		retiredCommitteesErr    error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(8) // 8 workers: validators(2) + non-signers(2) + double-signers(2) + committees(2)
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
			previousValidators = make([]*fsm.Validator, 0)
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
			previousNonSigners = make([]*fsm.NonSigner, 0)
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
			previousDoubleSigners = make([]*lib.DoubleSigner, 0)
		} else if input.Height > 1 {
			previousDoubleSigners, prevDoubleSignersErr = cli.DoubleSignersByHeight(groupCtx, input.Height-1)
		}
	})

	// Worker 7: Fetch subsidized committees (for committee_validator denormalization)
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		subsidizedCommittees, subsidizedCommitteesErr = cli.SubsidizedCommitteesByHeight(groupCtx, input.Height)
	})

	// Worker 8: Fetch retired committees (for committee_validator denormalization)
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		retiredCommittees, retiredCommitteesErr = cli.RetiredCommitteesByHeight(groupCtx, input.Height)
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
	if subsidizedCommitteesErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("fetch subsidized committees at height %d: %w", input.Height, subsidizedCommitteesErr)
	}
	if retiredCommitteesErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("fetch retired committees at height %d: %w", input.Height, retiredCommitteesErr)
	}

	// Build committee status maps for denormalization (O(1) lookup)
	subsidizedMap := make(map[uint64]bool, len(subsidizedCommittees))
	for _, id := range subsidizedCommittees {
		subsidizedMap[id] = true
	}
	retiredMap := make(map[uint64]bool, len(retiredCommittees))
	for _, id := range retiredCommittees {
		retiredMap[id] = true
	}

	// Build previous state maps for O(1) lookups
	prevValidatorMap := make(map[string]*fsm.Validator, len(previousValidators))
	for _, val := range previousValidators {
		prevValidatorMap[hex.EncodeToString(val.Address)] = val
	}

	prevNonSignerMap := make(map[string]*fsm.NonSigner, len(previousNonSigners))
	for _, ns := range previousNonSigners {
		addrHex := hex.EncodeToString(ns.Address)
		prevNonSignerMap[addrHex] = ns
	}

	// Build current non-signer map for joining with validators
	currentNonSignerMap := make(map[string]*fsm.NonSigner, len(currentNonSigners))
	for _, ns := range currentNonSigners {
		addrHex := hex.EncodeToString(ns.Address)
		currentNonSignerMap[addrHex] = ns
	}

	// Query validator lifecycle events from staging table using lightweight query
	// Returns only 3 columns (height, address, event_type) instead of 21 (~85% reduction)
	// These events define state transitions in the validator lifecycle:
	// - EventTypeAutomaticPause: validator transitions to paused state
	// - EventTypeAutomaticBeginUnstaking: validator transitions to unstaking state
	// - EventTypeAutomaticFinishUnstaking: validator deleted (unstaked)
	// - EventTypeSlash: validator slashed (affects staked amount)
	// - EventTypeReward: validator rewarded (informational, doesn't change state)
	validatorEvents, err := chainDb.GetValidatorLifecycleEvents(ctx, input.Height, true)
	if err != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("query validator events at height %d: %w", input.Height, err)
	}

	// Build event maps by validator address for O(1) lookup
	// Using lightweight struct - only need address for existence check
	rewardEvents := make(map[string]*chainstore.EventLifecycle)
	pauseEvents := make(map[string]*chainstore.EventLifecycle)
	beginUnstakingEvents := make(map[string]*chainstore.EventLifecycle)
	finishUnstakingEvents := make(map[string]*chainstore.EventLifecycle)
	slashEvents := make(map[string]*chainstore.EventLifecycle)

	for i := range validatorEvents {
		event := &validatorEvents[i]
		addr := event.Address

		switch event.EventType {
		case string(lib.EventTypeAutoPause):
			pauseEvents[addr] = event
		case string(lib.EventTypeAutoBeginUnstaking):
			beginUnstakingEvents[addr] = event
		case string(lib.EventTypeFinishUnstaking):
			finishUnstakingEvents[addr] = event
		case string(lib.EventTypeSlash):
			slashEvents[addr] = event
		case string(lib.EventTypeReward):
			rewardEvents[addr] = event
		}
	}

	// Compare and collect changed validators
	// Also count status breakdowns from all current validators (not just changed ones)
	changedValidators := make([]*indexer.Validator, 0)
	var numValidatorsNew, numValidatorsActive, numValidatorsPaused, numValidatorsUnstaking uint32

	for _, curr := range currentValidators {
		addrHex := hex.EncodeToString(curr.Address)
		prev := prevValidatorMap[addrHex]

		// Check if validator state changed
		changed := false
		hasEvent := false

		// Check for lifecycle events (state transitions)
		if _, hasPause := pauseEvents[addrHex]; hasPause {
			changed = true
			hasEvent = true
		}
		if _, hasBeginUnstake := beginUnstakingEvents[addrHex]; hasBeginUnstake {
			changed = true
			hasEvent = true
		}
		if _, hasSlash := slashEvents[addrHex]; hasSlash {
			changed = true
			hasEvent = true
		}
		if _, hasReward := rewardEvents[addrHex]; hasReward {
			changed = true
			hasEvent = true
		}

		if prev == nil {
			// New validator
			changed = true
			numValidatorsNew++
		} else if !hasEvent {
			// Only compare RPC fields if no event occurred
			// If an event occurred, we already marked changed=true above
			// Compare all fields that affect validator state
			// Note: Status is derived from MaxPausedHeight/UnstakingHeight, not compared directly
			if curr.StakedAmount != prev.StakedAmount ||
				hex.EncodeToString(curr.PublicKey) != hex.EncodeToString(prev.PublicKey) ||
				curr.NetAddress != prev.NetAddress ||
				curr.MaxPausedHeight != prev.MaxPausedHeight ||
				curr.UnstakingHeight != prev.UnstakingHeight ||
				hex.EncodeToString(curr.Output) != hex.EncodeToString(prev.Output) ||
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
				Address:         addrHex,
				PublicKey:       hex.EncodeToString(curr.PublicKey),
				NetAddress:      curr.NetAddress,
				StakedAmount:    curr.StakedAmount,
				Output:          hex.EncodeToString(curr.Output),
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

		// Count status breakdowns for ALL current validators (for BlockSummary aggregation)
		// Note: We derive status here from RPC fields, not from the validator object
		if curr.UnstakingHeight > 0 {
			numValidatorsUnstaking++
		} else if curr.MaxPausedHeight > 0 {
			numValidatorsPaused++
		} else {
			numValidatorsActive++
		}
	}

	// Build non-signing info for validators with non-signer data
	// Bidirectional iteration to track resets: current vs prev AND prev vs current
	changedNonSigningInfos := make([]*indexer.ValidatorNonSigningInfo, 0)
	var numNonSigningInfosNew uint32

	// Phase 1: Iterate current non-signers (new entries and updates)
	for _, curr := range currentNonSigners {
		addrHex := hex.EncodeToString(curr.Address)
		prev := prevNonSignerMap[addrHex]

		// Check if non-signing info changed
		changed := false
		if prev == nil {
			// New non-signer entry
			changed = true
			numNonSigningInfosNew++
		} else {
			// Compare fields - only Counter exists in RpcNonSigner
			if curr.Counter != prev.Counter {
				changed = true
			}
		}

		// Only create snapshot if non-signing info changed
		if changed {
			nonSigningInfo := &indexer.ValidatorNonSigningInfo{
				Address:           addrHex,
				MissedBlocksCount: curr.Counter, // Maps from RpcNonSigner.Counter
				LastSignedHeight:  input.Height, // Current height as last signed
				Height:            input.Height,
				HeightTime:        input.BlockTime,
			}
			changedNonSigningInfos = append(changedNonSigningInfos, nonSigningInfo)
		}
	}

	// Phase 2: Iterate previous non-signers to detect resets/reboots
	// If a non-signer existed in previous but not in current, create a "reset" record with zeros
	for _, prev := range previousNonSigners {
		addrHex := hex.EncodeToString(prev.Address)
		if _, exists := currentNonSignerMap[addrHex]; !exists {
			// Validator was a non-signer previously but not anymore (counter was reset)
			// Create a reset record with all zeros to track this state change
			resetInfo := &indexer.ValidatorNonSigningInfo{
				Address:           addrHex,
				MissedBlocksCount: 0, // Reset to zero
				LastSignedHeight:  input.Height,
				Height:            input.Height,
				HeightTime:        input.BlockTime,
			}
			changedNonSigningInfos = append(changedNonSigningInfos, resetInfo)
		}
	}

	// Build previous double-signers map for O(1) lookups
	prevDoubleSignersMap := make(map[string]uint64, len(previousDoubleSigners))
	for _, ds := range previousDoubleSigners {
		addrHex := hex.EncodeToString(ds.Id)
		prevDoubleSignersMap[addrHex] = uint64(len(ds.Heights))
	}

	// Build current double-signers map for reset detection
	currentDoubleSignersMap := make(map[string]*lib.DoubleSigner, len(currentDoubleSigners))
	for _, ds := range currentDoubleSigners {
		addrHex := hex.EncodeToString(ds.Id)
		currentDoubleSignersMap[addrHex] = ds
	}

	// Compare and collect changed double-signing info
	// Bidirectional iteration to track resets: current vs prev AND prev vs current
	changedDoubleSigningInfos := make([]*indexer.ValidatorDoubleSigningInfo, 0)

	// Phase 1: Iterate current double-signers (new entries and updates)
	for _, curr := range currentDoubleSigners {
		addrHex := hex.EncodeToString(curr.Id)
		currentCount := uint64(len(curr.Heights))
		prevCount := prevDoubleSignersMap[addrHex]

		// Snapshot-on-change: only insert if evidence count changed
		if currentCount != prevCount {
			var firstHeight, lastHeight uint64
			if len(curr.Heights) > 0 {
				firstHeight = curr.Heights[0]
				lastHeight = curr.Heights[len(curr.Heights)-1]
			}

			info := &indexer.ValidatorDoubleSigningInfo{
				Address:             addrHex,
				EvidenceCount:       currentCount,
				FirstEvidenceHeight: firstHeight,
				LastEvidenceHeight:  lastHeight,
				Height:              input.Height,
				HeightTime:          input.BlockTime,
			}
			changedDoubleSigningInfos = append(changedDoubleSigningInfos, info)
		}
	}

	// Phase 2: Iterate previous double-signers to detect resets/reboots
	// If a double-signer existed in previous but not in current, create a "reset" record with zeros
	for _, prev := range previousDoubleSigners {
		addrHex := hex.EncodeToString(prev.Id)
		if _, exists := currentDoubleSignersMap[addrHex]; !exists {
			// Validator had double-signing evidence previously but not anymore (evidence was cleared/reset)
			// Create a reset record with all zeros to track this state change
			resetInfo := &indexer.ValidatorDoubleSigningInfo{
				Address:             addrHex,
				EvidenceCount:       0, // Reset to zero
				FirstEvidenceHeight: 0,
				LastEvidenceHeight:  0,
				Height:              input.Height,
				HeightTime:          input.BlockTime,
			}
			changedDoubleSigningInfos = append(changedDoubleSigningInfos, resetInfo)
		}
	}

	// Build committee-validator junction table records for validators with committee changes
	// Denormalizes committee status (subsidized, retired) for efficient filtering without JOIN
	var committeeValidators []*indexer.CommitteeValidator
	for _, v := range changedValidators {
		// Create a junction record for each committee this validator belongs to
		for _, committeeID := range v.Committees {
			cv := &indexer.CommitteeValidator{
				CommitteeID:      committeeID,
				ValidatorAddress: v.Address,
				StakedAmount:     v.StakedAmount,
				Status:           v.Status,
				Delegate:         v.Delegate,
				Compound:         v.Compound,
				Height:           v.Height,
				HeightTime:       v.HeightTime,
				// Denormalized from Committee - enables filtering by committee status without JOIN
				Subsidized: subsidizedMap[committeeID],
				Retired:    retiredMap[committeeID],
			}
			committeeValidators = append(committeeValidators, cv)
		}
	}

	// Insert to staging tables in PARALLEL using worker pool
	// Each insert goes to a different table, so no conflicts
	insertPool := ac.WorkerPool(4) // 4 workers for 4 parallel inserts
	insertGroup := insertPool.NewGroupContext(ctx)
	insertCtx := insertGroup.Context()

	var (
		validatorsErr        error
		nonSigningInfoErr    error
		committeeValErr      error
		doubleSigningInfoErr error
	)

	// Worker 1: Insert validators
	if len(changedValidators) > 0 {
		insertGroup.Submit(func() {
			if err := insertCtx.Err(); err != nil {
				return
			}
			validatorsErr = chainDb.InsertValidatorsStaging(insertCtx, changedValidators)
		})
	}

	// Worker 2: Insert non-signing info
	if len(changedNonSigningInfos) > 0 {
		insertGroup.Submit(func() {
			if err := insertCtx.Err(); err != nil {
				return
			}
			nonSigningInfoErr = chainDb.InsertValidatorNonSigningInfoStaging(insertCtx, changedNonSigningInfos)
		})
	}

	// Worker 3: Insert committee validators
	if len(committeeValidators) > 0 {
		insertGroup.Submit(func() {
			if err := insertCtx.Err(); err != nil {
				return
			}
			committeeValErr = chainDb.InsertCommitteeValidatorsStaging(insertCtx, committeeValidators)
		})
	}

	// Worker 4: Insert double signing info
	if len(changedDoubleSigningInfos) > 0 {
		insertGroup.Submit(func() {
			if err := insertCtx.Err(); err != nil {
				return
			}
			doubleSigningInfoErr = chainDb.InsertValidatorDoubleSigningInfoStaging(insertCtx, changedDoubleSigningInfos)
		})
	}

	// Wait for all inserts to complete
	if err := insertGroup.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel insert encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", input.Height),
			zap.Error(err),
		)
	}

	// Check for insert errors
	if validatorsErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("insert validators staging: %w", validatorsErr)
	}
	if nonSigningInfoErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("insert validator non-signing info staging: %w", nonSigningInfoErr)
	}
	if committeeValErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("insert committee validators staging: %w", committeeValErr)
	}
	if doubleSigningInfoErr != nil {
		return types.ActivityIndexValidatorsOutput{}, fmt.Errorf("insert validator double signing info staging: %w", doubleSigningInfoErr)
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Indexed validators",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", input.Height),
		zap.Int("totalValidators", len(currentValidators)),
		zap.Int("changedValidators", len(changedValidators)),
		zap.Int("committeeValidators", len(committeeValidators)),
		zap.Int("totalNonSigners", len(currentNonSigners)),
		zap.Int("changedNonSigningInfos", len(changedNonSigningInfos)),
		zap.Int("totalDoubleSigners", len(currentDoubleSigners)),
		zap.Int("changedDoubleSigningInfos", len(changedDoubleSigningInfos)),
		zap.Int("pauseEvents", len(pauseEvents)),
		zap.Int("beginUnstakingEvents", len(beginUnstakingEvents)),
		zap.Int("finishUnstakingEvents", len(finishUnstakingEvents)),
		zap.Int("slashEvents", len(slashEvents)),
		zap.Int("rewardEvents", len(rewardEvents)),
		zap.Float64("durationMs", durationMs))

	return types.ActivityIndexValidatorsOutput{
		NumValidators:          uint32(len(changedValidators)),
		NumValidatorsNew:       numValidatorsNew,
		NumValidatorsActive:    numValidatorsActive,
		NumValidatorsPaused:    numValidatorsPaused,
		NumValidatorsUnstaking: numValidatorsUnstaking,
		NumNonSigningInfos:     uint32(len(changedNonSigningInfos)),
		NumNonSigningInfosNew:  numNonSigningInfosNew,
		NumDoubleSigningInfos:  uint32(len(changedDoubleSigningInfos)),
		NumCommitteeValidators: uint32(len(committeeValidators)),
		DurationMs:             durationMs,
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
