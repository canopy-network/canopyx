package admin

import (
	"time"
)

type ReindexRequest struct {
	ChainID     uint64    `ch:"chain_id"`
	Height      uint64    `ch:"height"`
	RequestedBy string    `ch:"requested_by"`
	Status      string    `ch:"status"`
	WorkflowID  string    `ch:"workflow_id"`
	RunID       string    `ch:"run_id"`
	RequestedAt time.Time `ch:"requested_at"`
}
