package indexer

import (
	"time"
)

const TxsProductionTableName = "txs"
const TxsStagingTableName = "txs_staging"

// Transaction stores ALL transaction data in a single table.
// Common queryable fields are typed columns.
// Type-specific fields are stored in the compressed 'msg' JSON field.
// ClickHouse's columnar storage ensures list queries only read the columns they need.
type Transaction struct {
	// Primary key
	Height uint64 `ch:"height" json:"height"`
	TxHash string `ch:"tx_hash" json:"tx_hash"`

	// Time fields for queries
	Time       time.Time `ch:"time" json:"time"`               // Transaction timestamp
	HeightTime time.Time `ch:"height_time" json:"height_time"` // Block timestamp for range queries

	// Common filterable fields
	MessageType  string  `ch:"message_type" json:"message_type"`           // LowCardinality(String) for efficient filtering
	Signer       string  `ch:"signer" json:"signer"`                       // Transaction signer address
	Counterparty *string `ch:"counterparty" json:"counterparty,omitempty"` // Recipient/validator/pool/contract address
	Amount       *uint64 `ch:"amount" json:"amount,omitempty"`             // Amount transferred/staked/delegated (null for votes, etc.)
	Fee          uint64  `ch:"fee" json:"fee"`                             // Transaction fee

	// ===== NEW EXTRACTED FIELDS (for efficient querying) =====

	// Validator-related (stake, unstake, editStake)
	ValidatorAddress *string  `ch:"validator_address" json:"validator_address,omitempty"`
	Commission       *float64 `ch:"commission" json:"commission,omitempty"`

	// DEX-related (dexLimitOrder, dexLiquidityDeposit, dexLiquidityWithdraw)
	ChainID      *uint64 `ch:"chain_id" json:"chain_id,omitempty"`
	SellAmount   *uint64 `ch:"sell_amount" json:"sell_amount,omitempty"`
	BuyAmount    *uint64 `ch:"buy_amount" json:"buy_amount,omitempty"`
	LiquidityAmt *uint64 `ch:"liquidity_amount" json:"liquidity_amount,omitempty"`

	// Order-related (createOrder, editOrder, deleteOrder)
	OrderID *string  `ch:"order_id" json:"order_id,omitempty"`
	Price   *float64 `ch:"price" json:"price,omitempty"`

	// Governance-related (changeParameter)
	ParamKey   *string `ch:"param_key" json:"param_key,omitempty"`
	ParamValue *string `ch:"param_value" json:"param_value,omitempty"`

	// Other
	CommitteeID *uint64 `ch:"committee_id" json:"committee_id,omitempty"` // For: subsidy
	Recipient   *string `ch:"recipient" json:"recipient,omitempty"`       // For: daoTransfer

	// Full message as compressed JSON (ALL type-specific fields)
	Msg string `ch:"msg" json:"msg"` // Complete message data with ZSTD compression

	// Signature fields (compressed, permanent audit trail)
	PublicKey *string `ch:"public_key" json:"public_key,omitempty"` // Compressed with ZSTD
	Signature *string `ch:"signature" json:"signature,omitempty"`   // Compressed with ZSTD
}
