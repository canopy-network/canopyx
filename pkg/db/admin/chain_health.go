package admin

import (
	"context"
	"fmt"
	"time"
)

// UpdateRPCHealth updates the RPC health status for a chain.
// This method should be called by the headscan workflow when it checks RPC endpoint reachability.
// It inserts a new row with updated RPC health fields, leveraging ClickHouse's ReplacingMergeTree.
func (db *AdminDB) UpdateRPCHealth(ctx context.Context, chainID uint64, status, message string) error {
	if chainID == 0 {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = "unknown"
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

	// Insert new row (ReplacingMergeTree will handle deduplication)
	if err := db.insertChain(ctx, current); err != nil {
		return fmt.Errorf("insert rpc health update for chain %d: %w", chainID, err)
	}

	// After updating RPC health, recompute overall health
	return db.updateOverallHealth(ctx, chainID)
}

// UpdateQueueHealth updates the queue health status for a chain.
// This method should be called by the controller when it monitors queue backlog metrics.
// It inserts a new row with updated queue health fields.
func (db *AdminDB) UpdateQueueHealth(ctx context.Context, chainID uint64, status, message string) error {
	if chainID == 0 {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = "unknown"
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

	// Insert new row (ReplacingMergeTree will handle deduplication)
	if err := db.insertChain(ctx, current); err != nil {
		return fmt.Errorf("insert queue health update for chain %d: %w", chainID, err)
	}

	// After updating queue health, recompute overall health
	return db.updateOverallHealth(ctx, chainID)
}

// UpdateDeploymentHealth updates the deployment health status for a chain.
// This method should be called by the controller when it checks k8s pod/deployment status.
// It inserts a new row with updated deployment health fields.
func (db *AdminDB) UpdateDeploymentHealth(ctx context.Context, chainID uint64, status, message string) error {
	if chainID == 0 {
		return fmt.Errorf("chain_id is required")
	}
	if status == "" {
		status = "unknown"
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

	// Insert new row (ReplacingMergeTree will handle deduplication)
	if err := db.insertChain(ctx, current); err != nil {
		return fmt.Errorf("insert deployment health update for chain %d: %w", chainID, err)
	}

	// After updating deployment health, recompute overall health
	return db.updateOverallHealth(ctx, chainID)
}

// updateOverallHealth computes and updates the overall health status for a chain
// based on its RPC, queue, and deployment health statuses.
// Overall health is the worst status among the three subsystems:
// unknown < healthy < degraded/warning < critical/unreachable/failed
func (db *AdminDB) updateOverallHealth(ctx context.Context, chainID uint64) error {
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

	// Insert new row (ReplacingMergeTree will handle deduplication)
	if err := db.insertChain(ctx, current); err != nil {
		return fmt.Errorf("insert overall health update for chain %d: %w", chainID, err)
	}

	return nil
}

// computeOverallHealthStatus determines the worst health status among multiple subsystems.
// The priority order (worst to best) is: failed/critical/unreachable > warning/degraded > healthy > unknown
func (db *AdminDB) computeOverallHealthStatus(statuses ...string) string {
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
