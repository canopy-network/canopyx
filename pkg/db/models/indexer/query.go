package indexer

import "time"

// BlockRow represents a block record returned from query operations.
// This is separate from Block to avoid coupling query results to ClickHouse-specific types.
type BlockRow struct {
	Height          uint64
	Hash            string
	Time            time.Time
	ProposerAddress string
	NumTxs          uint32
}

// TransactionRow represents a transaction record returned from query operations.
// This is separate from Transaction to avoid coupling query results to ClickHouse-specific types.
type TransactionRow struct {
	Height       uint64
	TxHash       string
	Time         time.Time
	MessageType  string
	Counterparty *string
	Signer       string
	Amount       *uint64
	Fee          uint64
}