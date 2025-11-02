package admin

import (
	"time"
)

type Chain struct {
	ChainID      uint64    `json:"chain_id" ch:"chain_id"` // ORDER BY set via builder
	ChainName    string    `json:"chain_name" ch:"chain_name"`
	RPCEndpoints []string  `json:"rpc_endpoints" ch:"rpc_endpoints"` // []string -> Array(String)
	Paused       uint8     `json:"paused" ch:"paused"`
	Deleted      uint8     `json:"deleted" ch:"deleted"`
	Image        string    `json:"image" ch:"image"`
	MinReplicas  uint16    `json:"min_replicas" ch:"min_replicas"`
	MaxReplicas  uint16    `json:"max_replicas" ch:"max_replicas"`
	Notes        string    `json:"notes,omitempty" ch:"notes"`
	CreatedAt    time.Time `json:"created_at" ch:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" ch:"updated_at"`

	// Health Status Fields - Updated by various subsystems
	// RPC Health: Updated by headscan workflow
	RPCHealthStatus    string    `json:"rpc_health_status" ch:"rpc_health_status"`   // unknown, healthy, degraded, unreachable
	RPCHealthMessage   string    `json:"rpc_health_message" ch:"rpc_health_message"` // error message or details
	RPCHealthUpdatedAt time.Time `json:"rpc_health_updated_at" ch:"rpc_health_updated_at"`

	// Queue Health: Updated by queue monitor workflow
	QueueHealthStatus    string    `json:"queue_health_status" ch:"queue_health_status"`   // unknown, healthy, warning, critical
	QueueHealthMessage   string    `json:"queue_health_message" ch:"queue_health_message"` // details about queue state
	QueueHealthUpdatedAt time.Time `json:"queue_health_updated_at" ch:"queue_health_updated_at"`

	// Deployment Health: Updated by controller
	DeploymentHealthStatus    string    `json:"deployment_health_status" ch:"deployment_health_status"`   // unknown, healthy, degraded, failed
	DeploymentHealthMessage   string    `json:"deployment_health_message" ch:"deployment_health_message"` // pod status details
	DeploymentHealthUpdatedAt time.Time `json:"deployment_health_updated_at" ch:"deployment_health_updated_at"`

	// Overall Health: Computed from subsystems
	OverallHealthStatus    string    `json:"overall_health_status" ch:"overall_health_status"` // unknown, healthy, degraded, critical
	OverallHealthUpdatedAt time.Time `json:"overall_health_updated_at" ch:"overall_health_updated_at"`
}
