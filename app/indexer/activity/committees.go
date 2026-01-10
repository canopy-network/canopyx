package activity

import (
    "context"
    "fmt"
    globalstore "github.com/canopy-network/canopyx/pkg/db/global"
    "time"

    "github.com/canopy-network/canopyx/app/indexer/types"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// bytesToHex converts a byte slice to a hex-encoded string.
// Returns empty string if bytes are nil or empty.
func bytesToHex(b []byte) string {
    if len(b) == 0 {
        return ""
    }
    return fmt.Sprintf("%x", b)
}

// indexCommitteesFromBlob indexes committees from the provided blob data and stores them in the database, returning the duration in ms.
func (ac *Context) indexCommitteesFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexCommitteesOutput, float64, error) {
    start := time.Now()
    committeesAtH := currentData.committeesData
    committeesAtH1 := previousData.committeesData
    numSubsidizedTotal := uint8(len(currentData.subsidizedCommittees))
    numRetiredTotal := uint8(len(currentData.retiredCommittees))

    currentCommittees := make(map[uint64]*indexermodels.Committee)
    for _, rpcCommittee := range committeesAtH {
        currentCommittees[rpcCommittee.ChainId] = &indexermodels.Committee{
            ChainID:                uint16(rpcCommittee.ChainId),
            LastRootHeightUpdated:  rpcCommittee.LastRootHeightUpdated,
            LastChainHeightUpdated: rpcCommittee.LastChainHeightUpdated,
            NumberOfSamples:        rpcCommittee.NumberOfSamples,
            Subsidized:             numSubsidizedTotal,
            Retired:                numRetiredTotal,
            Height:                 height,
            HeightTime:             heightTime,
        }
    }

    var changedCommittees []*indexermodels.Committee
    var numCommitteesNew uint32
    if height == 1 {
        for _, committee := range currentCommittees {
            changedCommittees = append(changedCommittees, committee)
        }
        numCommitteesNew = uint32(len(currentCommittees))
    } else {
        numSubsidizedTotalH1 := uint8(len(previousData.subsidizedCommittees))
        numRetiredTotalH1 := uint8(len(previousData.retiredCommittees))
        prevMap := make(map[uint64]*indexermodels.Committee)
        for _, rpcCommittee := range committeesAtH1 {
            prevMap[rpcCommittee.ChainId] = &indexermodels.Committee{
                ChainID:                uint16(rpcCommittee.ChainId),
                LastRootHeightUpdated:  rpcCommittee.LastRootHeightUpdated,
                LastChainHeightUpdated: rpcCommittee.LastChainHeightUpdated,
                NumberOfSamples:        rpcCommittee.NumberOfSamples,
                Subsidized:             numSubsidizedTotalH1,
                Retired:                numRetiredTotalH1,
            }
        }

        for chainID, currentCommittee := range currentCommittees {
            prevCommittee, existed := prevMap[chainID]
            if !existed {
                changedCommittees = append(changedCommittees, currentCommittee)
                numCommitteesNew++
                continue
            }
            if !committeesEqual(prevCommittee, currentCommittee) {
                changedCommittees = append(changedCommittees, currentCommittee)
            }
        }
    }

    var payments []*indexermodels.CommitteePayment
    for _, rpcCommittee := range committeesAtH {
        for _, pp := range rpcCommittee.PaymentPercents {
            payments = append(payments, &indexermodels.CommitteePayment{
                CommitteeID: rpcCommittee.ChainId,
                Address:     bytesToHex(pp.Address),
                Percent:     pp.Percent,
                Height:      height,
                HeightTime:  heightTime,
            })
        }
    }

    if len(changedCommittees) > 0 {
        if err := chainDb.InsertCommittees(ctx, changedCommittees); err != nil {
            return types.ActivityIndexCommitteesOutput{}, 0, err
        }
    }
    if len(payments) > 0 {
        if err := chainDb.InsertCommitteePayments(ctx, payments); err != nil {
            return types.ActivityIndexCommitteesOutput{}, 0, err
        }
    }
    out := types.ActivityIndexCommitteesOutput{
        NumCommittees:           uint32(len(changedCommittees)),
        NumCommitteesNew:        numCommitteesNew,
        NumCommitteesSubsidized: uint32(len(currentData.subsidizedCommittees)),
        NumCommitteesRetired:    uint32(len(currentData.retiredCommittees)),
        NumCommitteePayments:    uint32(len(payments)),
    }
    durationMs := float64(time.Since(start).Microseconds()) / 1000.0
    return out, durationMs, nil
}

// committeesEqual compares all fields of two Committee instances (excluding Height and HeightTime).
// Returns true if all committee data values are identical.
func committeesEqual(a, b *indexermodels.Committee) bool {
    return a.ChainID == b.ChainID &&
            a.LastRootHeightUpdated == b.LastRootHeightUpdated &&
            a.LastChainHeightUpdated == b.LastChainHeightUpdated &&
            a.NumberOfSamples == b.NumberOfSamples &&
            a.Subsidized == b.Subsidized &&
            a.Retired == b.Retired
}
