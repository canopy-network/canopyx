package admin

import (
	"context"
	"fmt"
	"time"

	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// UpdateRPCHealth updates the RPC health status for a chain.
// This method should be called by the headscan workflow when it checks RPC endpoint reachability.
// It inserts a new row with updated RPC health fields, leveraging ClickHouse's ReplacingMergeTree.
func (db *DB) UpdateRPCHealth(ctx context.Context, chainID uint64, status, message string) error {
	if chainID == 0 {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = adminmodels.ChainStatusUnknown
	}

	// Fetch the current chain record to preserve other fields
	current, err := db.GetChain(ctx, chainID)
	if err != nil {
		return fmt.Errorf("get chain %d: %w", chainID, err)
	}

	// Update RPC health fields
	current.RPCHealthStatus = status
	current.RPCHealthMessage = message
	current.RPCHealthUpdatedAt = time.Now()
	current.UpdatedAt = time.Now()

	// Insert a new row (ReplacingMergeTree will handle deduplication)
	if err := db.InsertChain(ctx, current); err != nil {
		return fmt.Errorf("insert rpc health update for chain %d: %w", chainID, err)
	}

	// After updating RPC health, recompute overall health
	return db.updateOverallHealth(ctx, chainID)
}

// UpdateQueueHealth updates the queue health status for a chain.
// This method should be called by the controller when it monitors queue backlog metrics.
// It inserts a new row with updated queue health fields.
func (db *DB) UpdateQueueHealth(ctx context.Context, chainID uint64, status, message string) error {
	if chainID == 0 {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = adminmodels.ChainStatusUnknown
	}

	// Fetch the current chain record to preserve other fields
	current, err := db.GetChain(ctx, chainID)
	if err != nil {
		return fmt.Errorf("get chain %d: %w", chainID, err)
	}

	// Update queue health fields
	current.QueueHealthStatus = status
	current.QueueHealthMessage = message
	current.QueueHealthUpdatedAt = time.Now()
	current.UpdatedAt = time.Now()

	// Insert a new row (ReplacingMergeTree will handle deduplication)
	if err := db.InsertChain(ctx, current); err != nil {
		return fmt.Errorf("insert queue health update for chain %d: %w", chainID, err)
	}

	// After updating queue health, recompute overall health
	return db.updateOverallHealth(ctx, chainID)
}

// UpdateDeploymentHealth updates the deployment health status for a chain.
// This method should be called by the controller when it checks k8s pod/deployment status.
// It inserts a new row with updated deployment health fields.
func (db *DB) UpdateDeploymentHealth(ctx context.Context, chainID uint64, status, message string) error {
	if chainID == 0 {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = adminmodels.ChainStatusUnknown
	}

	// Fetch the current chain record to preserve other fields
	current, err := db.GetChain(ctx, chainID)
	if err != nil {
		return fmt.Errorf("get chain %d: %w", chainID, err)
	}

	// Update deployment health fields
	current.DeploymentHealthStatus = status
	current.DeploymentHealthMessage = message
	current.DeploymentHealthUpdatedAt = time.Now()
	current.UpdatedAt = time.Now()

	// Insert a new row (ReplacingMergeTree will handle deduplication)
	if err := db.InsertChain(ctx, current); err != nil {
		return fmt.Errorf("insert deployment health update for chain %d: %w", chainID, err)
	}

	// After updating deployment health, recompute overall health
	return db.updateOverallHealth(ctx, chainID)
}

// updateOverallHealth computes and updates the overall health status for a chain
// based on its RPC, queue, and deployment health statuses.
// Overall health is the worst status among the three subsystems:
// unknown < healthy < degraded/warning < critical/unreachable/failed
func (db *DB) updateOverallHealth(ctx context.Context, chainID uint64) error {
	if chainID == 0 {
		return fmt.Errorf("chain_id is required")
	}

	// Fetch the current chain record with all health fields
	current, err := db.GetChain(ctx, chainID)
	if err != nil {
		return fmt.Errorf("get chain %d: %w", chainID, err)
	}

	// Compute overall health as the worst of the three subsystems
	overallStatus := db.computeOverallHealthStatus(
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

	// Insert a new row (ReplacingMergeTree will handle deduplication)
	if err := db.InsertChain(ctx, current); err != nil {
		return fmt.Errorf("insert overall health update for chain %d: %w", chainID, err)
	}

	return nil
}

// computeOverallHealthStatus determines the worst health status among multiple subsystems.
// The priority order (worst to best) is: failed/critical/unreachable > warning/degraded > healthy > unknown
func (db *DB) computeOverallHealthStatus(statuses ...string) string {
	// Priority map (higher number = worse health)
	priority := map[string]int{
		adminmodels.ChainStatusFailed:      5,
		adminmodels.ChainStatusCritical:    5,
		adminmodels.ChainStatusUnreachable: 5,
		adminmodels.ChainStatusWarning:     3,
		adminmodels.ChainStatusDegraded:    3,
		adminmodels.ChainStatusHealthy:     2,
		adminmodels.ChainStatusUnknown:     1,
	}

	worstStatus := adminmodels.ChainStatusUnknown
	worstPriority := priority[adminmodels.ChainStatusUnknown]

	for _, status := range statuses {
		if status == "" {
			status = adminmodels.ChainStatusUnknown
		}
		p, exists := priority[status]
		if !exists {
			p = priority[adminmodels.ChainStatusUnknown]
		}
		if p > worstPriority {
			worstPriority = p
			worstStatus = status
		}
	}

	return worstStatus
}
