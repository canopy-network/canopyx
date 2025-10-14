package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

type Chain struct {
	ch.CHModel `ch:"table:chains"`

	ChainID      string    `json:"chain_id" ch:"chain_id"` // ORDER BY set via builder
	ChainName    string    `json:"chain_name" ch:"chain_name"`
	RPCEndpoints []string  `json:"rpc_endpoints" ch:"rpc_endpoints"` // []string -> Array(String)
	Paused       uint8     `json:"paused" ch:"paused,default:0"`     // defaults are applied by CreateTable
	Deleted      uint8     `json:"deleted" ch:"deleted,default:0"`
	Image        string    `json:"image" ch:"image,default:''"`
	MinReplicas  uint16    `json:"min_replicas" ch:"min_replicas,default:1"`
	MaxReplicas  uint16    `json:"max_replicas" ch:"max_replicas,default:1"`
	Notes        string    `json:"notes,omitempty" ch:"notes,default:''"`
	CreatedAt    time.Time `json:"created_at" ch:"created_at,default:now()"`
	UpdatedAt    time.Time `json:"updated_at" ch:"updated_at,default:now()"`

	// Health Status Fields - Updated by various subsystems
	// RPC Health: Updated by headscan workflow
	RPCHealthStatus    string    `json:"rpc_health_status" ch:"rpc_health_status,default:'unknown'"` // unknown, healthy, degraded, unreachable
	RPCHealthMessage   string    `json:"rpc_health_message" ch:"rpc_health_message,default:''"`      // error message or details
	RPCHealthUpdatedAt time.Time `json:"rpc_health_updated_at" ch:"rpc_health_updated_at,default:now()"`

	// Queue Health: Updated by queue monitor workflow
	QueueHealthStatus    string    `json:"queue_health_status" ch:"queue_health_status,default:'unknown'"` // unknown, healthy, warning, critical
	QueueHealthMessage   string    `json:"queue_health_message" ch:"queue_health_message,default:''"`      // details about queue state
	QueueHealthUpdatedAt time.Time `json:"queue_health_updated_at" ch:"queue_health_updated_at,default:now()"`

	// Deployment Health: Updated by controller
	DeploymentHealthStatus    string    `json:"deployment_health_status" ch:"deployment_health_status,default:'unknown'"` // unknown, healthy, degraded, failed
	DeploymentHealthMessage   string    `json:"deployment_health_message" ch:"deployment_health_message,default:''"`      // pod status details
	DeploymentHealthUpdatedAt time.Time `json:"deployment_health_updated_at" ch:"deployment_health_updated_at,default:now()"`

	// Overall Health: Computed from subsystems
	OverallHealthStatus    string    `json:"overall_health_status" ch:"overall_health_status,default:'unknown'"` // unknown, healthy, degraded, critical
	OverallHealthUpdatedAt time.Time `json:"overall_health_updated_at" ch:"overall_health_updated_at,default:now()"`
}

// InitChains creates the chain table using the models.Chain definition.
// Table: ReplacingMergeTree(updated_at) ORDER BY (chain_id)
func InitChains(ctx context.Context, db *ch.DB) error {
	_, err := db.NewCreateTable().
		Model((*Chain)(nil)).
		IfNotExists().
		Engine("ReplacingMergeTree(updated_at)").
		// explicit ORDER BY to match your prior SQL
		Order("(chain_id)").
		Exec(ctx)
	return err
}

// GetChain returns the latest (deduped) row for the given chain_id.
func GetChain(ctx context.Context, db *ch.DB, id string) (*Chain, error) {
	var c Chain

	// Use FINAL to collapse versions from ReplacingMergeTree(updated_at)
	err := db.NewSelect().
		Model(&c).
		Final().
		Where("chain_id = ?", id).
		Limit(1).
		Scan(ctx)

	return &c, err
}

// UpdateRPCHealth updates the RPC health status for a chain.
// This method should be called by the headscan workflow when it checks RPC endpoint reachability.
// It inserts a new row with updated RPC health fields, leveraging ClickHouse's ReplacingMergeTree.
func UpdateRPCHealth(ctx context.Context, db *ch.DB, chainID, status, message string) error {
	if chainID == "" {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = "unknown"
	}

	// Fetch the current chain record to preserve other fields
	current, err := GetChain(ctx, db, chainID)
	if err != nil {
		return fmt.Errorf("get chain %s: %w", chainID, err)
	}

	// Update RPC health fields
	current.RPCHealthStatus = status
	current.RPCHealthMessage = message
	current.RPCHealthUpdatedAt = time.Now()
	current.UpdatedAt = time.Now()

	// Insert new row (ReplacingMergeTree will handle deduplication)
	_, err = db.NewInsert().Model(current).Exec(ctx)
	if err != nil {
		return fmt.Errorf("insert rpc health update for chain %s: %w", chainID, err)
	}

	// After updating RPC health, recompute overall health
	return UpdateOverallHealth(ctx, db, chainID)
}

// UpdateQueueHealth updates the queue health status for a chain.
// This method should be called by the controller when it monitors queue backlog metrics.
// It inserts a new row with updated queue health fields.
func UpdateQueueHealth(ctx context.Context, db *ch.DB, chainID, status, message string) error {
	if chainID == "" {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = "unknown"
	}

	// Fetch the current chain record to preserve other fields
	current, err := GetChain(ctx, db, chainID)
	if err != nil {
		return fmt.Errorf("get chain %s: %w", chainID, err)
	}

	// Update queue health fields
	current.QueueHealthStatus = status
	current.QueueHealthMessage = message
	current.QueueHealthUpdatedAt = time.Now()
	current.UpdatedAt = time.Now()

	// Insert new row (ReplacingMergeTree will handle deduplication)
	_, err = db.NewInsert().Model(current).Exec(ctx)
	if err != nil {
		return fmt.Errorf("insert queue health update for chain %s: %w", chainID, err)
	}

	// After updating queue health, recompute overall health
	return UpdateOverallHealth(ctx, db, chainID)
}

// UpdateDeploymentHealth updates the deployment health status for a chain.
// This method should be called by the controller when it checks k8s pod/deployment status.
// It inserts a new row with updated deployment health fields.
func UpdateDeploymentHealth(ctx context.Context, db *ch.DB, chainID, status, message string) error {
	if chainID == "" {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = "unknown"
	}

	// Fetch the current chain record to preserve other fields
	current, err := GetChain(ctx, db, chainID)
	if err != nil {
		return fmt.Errorf("get chain %s: %w", chainID, err)
	}

	// Update deployment health fields
	current.DeploymentHealthStatus = status
	current.DeploymentHealthMessage = message
	current.DeploymentHealthUpdatedAt = time.Now()
	current.UpdatedAt = time.Now()

	// Insert new row (ReplacingMergeTree will handle deduplication)
	_, err = db.NewInsert().Model(current).Exec(ctx)
	if err != nil {
		return fmt.Errorf("insert deployment health update for chain %s: %w", chainID, err)
	}

	// After updating deployment health, recompute overall health
	return UpdateOverallHealth(ctx, db, chainID)
}

// UpdateOverallHealth computes and updates the overall health status for a chain
// based on its RPC, queue, and deployment health statuses.
// Overall health is the worst status among the three subsystems:
// unknown < healthy < degraded/warning < critical/unreachable/failed
func UpdateOverallHealth(ctx context.Context, db *ch.DB, chainID string) error {
	if chainID == "" {
		return fmt.Errorf("chain_id is required")
	}

	// Fetch the current chain record with all health fields
	current, err := GetChain(ctx, db, chainID)
	if err != nil {
		return fmt.Errorf("get chain %s: %w", chainID, err)
	}

	// Compute overall health as the worst of the three subsystems
	overallStatus := computeOverallHealthStatus(
		current.RPCHealthStatus,
		current.QueueHealthStatus,
		current.DeploymentHealthStatus,
	)

	// Only update if the overall status has changed
	if current.OverallHealthStatus == overallStatus {
		return nil
	}

	current.OverallHealthStatus = overallStatus
	current.OverallHealthUpdatedAt = time.Now()
	current.UpdatedAt = time.Now()

	// Insert new row (ReplacingMergeTree will handle deduplication)
	_, err = db.NewInsert().Model(current).Exec(ctx)
	if err != nil {
		return fmt.Errorf("insert overall health update for chain %s: %w", chainID, err)
	}

	return nil
}

// computeOverallHealthStatus determines the worst health status among multiple subsystems.
// The priority order (worst to best) is: failed/critical/unreachable > warning/degraded > healthy > unknown
func computeOverallHealthStatus(statuses ...string) string {
	// Priority map (higher number = worse health)
	priority := map[string]int{
		"failed":      5,
		"critical":    5,
		"unreachable": 5,
		"warning":     3,
		"degraded":    3,
		"healthy":     2,
		"unknown":     1,
	}

	worstStatus := "unknown"
	worstPriority := priority["unknown"]

	for _, status := range statuses {
		if status == "" {
			status = "unknown"
		}
		p, exists := priority[status]
		if !exists {
			// Unknown status types default to "unknown"
			p = priority["unknown"]
		}
		if p > worstPriority {
			worstPriority = p
			worstStatus = status
		}
	}

	return worstStatus
}
