package activity

import (
	"context"
	"encoding/hex"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"time"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/app/indexer/types"
	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
)

// indexValidatorsFromBlob indexes validator state and signing info for a given block using the snapshot-on-change pattern.
func (ac *Context) indexValidatorsFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData, events *blobEventMaps) (types.ActivityIndexValidatorsOutput, float64, error) {
	start := time.Now()
	subsidizedMap := make(map[uint64]bool, len(currentData.subsidizedCommittees))
	for _, id := range currentData.subsidizedCommittees {
		subsidizedMap[id] = true
	}
	retiredMap := make(map[uint64]bool, len(currentData.retiredCommittees))
	for _, id := range currentData.retiredCommittees {
		retiredMap[id] = true
	}

	prevValidatorMap := make(map[string]*fsm.Validator, len(previousData.validators))
	for _, val := range previousData.validators {
		prevValidatorMap[hex.EncodeToString(val.Address)] = val
	}

	prevNonSignerMap := make(map[string]*fsm.NonSigner, len(previousData.nonSigners))
	for _, ns := range previousData.nonSigners {
		addrHex := hex.EncodeToString(ns.Address)
		prevNonSignerMap[addrHex] = ns
	}

	currentNonSignerMap := make(map[string]*fsm.NonSigner, len(currentData.nonSigners))
	for _, ns := range currentData.nonSigners {
		addrHex := hex.EncodeToString(ns.Address)
		currentNonSignerMap[addrHex] = ns
	}

	changedValidators := make([]*indexermodels.Validator, 0)
	var numValidatorsNew, numValidatorsActive, numValidatorsPaused, numValidatorsUnstaking uint32

	for _, curr := range currentData.validators {
		addrHex := hex.EncodeToString(curr.Address)
		prev := prevValidatorMap[addrHex]

		changed := false
		hasEvent := false

		if _, ok := events.pause[addrHex]; ok {
			changed = true
			hasEvent = true
		}
		if _, ok := events.beginUnstaking[addrHex]; ok {
			changed = true
			hasEvent = true
		}
		if _, ok := events.validatorSlash[addrHex]; ok {
			changed = true
			hasEvent = true
		}
		if _, ok := events.validatorReward[addrHex]; ok {
			changed = true
			hasEvent = true
		}

		if prev == nil {
			changed = true
			numValidatorsNew++
		} else if !hasEvent {
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

		if changed {
			val := &indexermodels.Validator{
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
				Height:          height,
				HeightTime:      heightTime,
			}
			val.Status = val.DeriveStatus()
			changedValidators = append(changedValidators, val)
		}

		if curr.UnstakingHeight > 0 {
			numValidatorsUnstaking++
		} else if curr.MaxPausedHeight > 0 {
			numValidatorsPaused++
		} else {
			numValidatorsActive++
		}
	}

	changedNonSigningInfos := make([]*indexermodels.ValidatorNonSigningInfo, 0)
	var numNonSigningInfosNew uint32
	for _, curr := range currentData.nonSigners {
		addrHex := hex.EncodeToString(curr.Address)
		prev := prevNonSignerMap[addrHex]
		changed := false
		if prev == nil {
			changed = true
			numNonSigningInfosNew++
		} else if curr.Counter != prev.Counter {
			changed = true
		}

		if changed {
			nonSigningInfo := &indexermodels.ValidatorNonSigningInfo{
				Address:           addrHex,
				MissedBlocksCount: curr.Counter,
				LastSignedHeight:  height,
				Height:            height,
				HeightTime:        heightTime,
			}
			changedNonSigningInfos = append(changedNonSigningInfos, nonSigningInfo)
		}
	}

	for _, prev := range previousData.nonSigners {
		addrHex := hex.EncodeToString(prev.Address)
		if _, exists := currentNonSignerMap[addrHex]; !exists {
			resetInfo := &indexermodels.ValidatorNonSigningInfo{
				Address:           addrHex,
				MissedBlocksCount: 0,
				LastSignedHeight:  height,
				Height:            height,
				HeightTime:        heightTime,
			}
			changedNonSigningInfos = append(changedNonSigningInfos, resetInfo)
		}
	}

	prevDoubleSignersMap := make(map[string]uint64, len(previousData.doubleSigners))
	for _, ds := range previousData.doubleSigners {
		addrHex := hex.EncodeToString(ds.Id)
		prevDoubleSignersMap[addrHex] = uint64(len(ds.Heights))
	}

	currentDoubleSignersMap := make(map[string]*lib.DoubleSigner, len(currentData.doubleSigners))
	for _, ds := range currentData.doubleSigners {
		addrHex := hex.EncodeToString(ds.Id)
		currentDoubleSignersMap[addrHex] = ds
	}

	changedDoubleSigningInfos := make([]*indexermodels.ValidatorDoubleSigningInfo, 0)
	for _, curr := range currentData.doubleSigners {
		addrHex := hex.EncodeToString(curr.Id)
		currentCount := uint64(len(curr.Heights))
		prevCount := prevDoubleSignersMap[addrHex]

		if currentCount != prevCount {
			var firstHeight, lastHeight uint64
			if len(curr.Heights) > 0 {
				firstHeight = curr.Heights[0]
				lastHeight = curr.Heights[len(curr.Heights)-1]
			}
			info := &indexermodels.ValidatorDoubleSigningInfo{
				Address:             addrHex,
				EvidenceCount:       currentCount,
				FirstEvidenceHeight: firstHeight,
				LastEvidenceHeight:  lastHeight,
				Height:              height,
				HeightTime:          heightTime,
			}
			changedDoubleSigningInfos = append(changedDoubleSigningInfos, info)
		}
	}

	for _, prev := range previousData.doubleSigners {
		addrHex := hex.EncodeToString(prev.Id)
		if _, exists := currentDoubleSignersMap[addrHex]; !exists {
			resetInfo := &indexermodels.ValidatorDoubleSigningInfo{
				Address:             addrHex,
				EvidenceCount:       0,
				FirstEvidenceHeight: 0,
				LastEvidenceHeight:  0,
				Height:              height,
				HeightTime:          heightTime,
			}
			changedDoubleSigningInfos = append(changedDoubleSigningInfos, resetInfo)
		}
	}

	var committeeValidators []*indexermodels.CommitteeValidator
	for _, v := range changedValidators {
		for _, committeeID := range v.Committees {
			cv := &indexermodels.CommitteeValidator{
				CommitteeID:      committeeID,
				ValidatorAddress: v.Address,
				StakedAmount:     v.StakedAmount,
				Status:           v.Status,
				Delegate:         v.Delegate,
				Compound:         v.Compound,
				Height:           v.Height,
				HeightTime:       v.HeightTime,
				Subsidized:       subsidizedMap[committeeID],
				Retired:          retiredMap[committeeID],
			}
			committeeValidators = append(committeeValidators, cv)
		}
	}

	if len(changedValidators) > 0 {
		if err := chainDb.InsertValidators(ctx, changedValidators); err != nil {
			return types.ActivityIndexValidatorsOutput{}, 0, err
		}
	}
	if len(changedNonSigningInfos) > 0 {
		if err := chainDb.InsertValidatorNonSigningInfo(ctx, changedNonSigningInfos); err != nil {
			return types.ActivityIndexValidatorsOutput{}, 0, err
		}
	}
	if len(committeeValidators) > 0 {
		if err := chainDb.InsertCommitteeValidators(ctx, committeeValidators); err != nil {
			return types.ActivityIndexValidatorsOutput{}, 0, err
		}
	}
	if len(changedDoubleSigningInfos) > 0 {
		if err := chainDb.InsertValidatorDoubleSigningInfo(ctx, changedDoubleSigningInfos); err != nil {
			return types.ActivityIndexValidatorsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexValidatorsOutput{
		NumValidators:          uint32(len(changedValidators)),
		NumValidatorsNew:       numValidatorsNew,
		NumValidatorsActive:    numValidatorsActive,
		NumValidatorsPaused:    numValidatorsPaused,
		NumValidatorsUnstaking: numValidatorsUnstaking,
		NumNonSigningInfos:     uint32(len(changedNonSigningInfos)),
		NumNonSigningInfosNew:  numNonSigningInfosNew,
		NumDoubleSigningInfos:  uint32(len(changedDoubleSigningInfos)),
		NumCommitteeValidators: uint32(len(committeeValidators)),
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
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
