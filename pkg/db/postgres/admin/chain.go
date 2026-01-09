package admin

import (
	"context"
	"fmt"
	"time"

	adminStore "github.com/canopy-network/canopyx/pkg/db/admin"
	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/db/postgres"
)

// initChains creates the chains table using PostgreSQL DDL
func (db *DB) initChains(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS chains (
			chain_id BIGINT PRIMARY KEY,
			chain_name TEXT NOT NULL,
			rpc_endpoints TEXT[] NOT NULL DEFAULT '{}',
			paused SMALLINT NOT NULL DEFAULT 0,
			deleted SMALLINT NOT NULL DEFAULT 0,
			image TEXT NOT NULL DEFAULT '',
			min_replicas INTEGER NOT NULL DEFAULT 1,
			max_replicas INTEGER NOT NULL DEFAULT 1,
			reindex_min_replicas INTEGER NOT NULL DEFAULT 1,
			reindex_max_replicas INTEGER NOT NULL DEFAULT 1,
			reindex_scale_threshold INTEGER NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			rpc_health_status TEXT NOT NULL DEFAULT 'unknown',
			rpc_health_message TEXT NOT NULL DEFAULT '',
			rpc_health_updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			queue_health_status TEXT NOT NULL DEFAULT 'unknown',
			queue_health_message TEXT NOT NULL DEFAULT '',
			queue_health_updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			deployment_health_status TEXT NOT NULL DEFAULT 'unknown',
			deployment_health_message TEXT NOT NULL DEFAULT '',
			deployment_health_updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			overall_health_status TEXT NOT NULL DEFAULT 'unknown',
			overall_health_updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`

	return db.Exec(ctx, query)
}

// GetChain returns the chain for the given chain_id
func (db *DB) GetChain(ctx context.Context, id uint64) (*adminmodels.Chain, error) {
	query := `
		SELECT chain_id, chain_name, rpc_endpoints, paused, deleted, image,
			   min_replicas, max_replicas, reindex_min_replicas, reindex_max_replicas, reindex_scale_threshold,
			   notes, created_at, updated_at,
			   rpc_health_status, rpc_health_message, rpc_health_updated_at,
			   queue_health_status, queue_health_message, queue_health_updated_at,
			   deployment_health_status, deployment_health_message, deployment_health_updated_at,
			   overall_health_status, overall_health_updated_at
		FROM chains
		WHERE chain_id = $1
	`

	var chain adminmodels.Chain
	err := db.QueryRow(ctx, query, id).Scan(
		&chain.ChainID,
		&chain.ChainName,
		&chain.RPCEndpoints,
		&chain.Paused,
		&chain.Deleted,
		&chain.Image,
		&chain.MinReplicas,
		&chain.MaxReplicas,
		&chain.ReindexMinReplicas,
		&chain.ReindexMaxReplicas,
		&chain.ReindexScaleThreshold,
		&chain.Notes,
		&chain.CreatedAt,
		&chain.UpdatedAt,
		&chain.RPCHealthStatus,
		&chain.RPCHealthMessage,
		&chain.RPCHealthUpdatedAt,
		&chain.QueueHealthStatus,
		&chain.QueueHealthMessage,
		&chain.QueueHealthUpdatedAt,
		&chain.DeploymentHealthStatus,
		&chain.DeploymentHealthMessage,
		&chain.DeploymentHealthUpdatedAt,
		&chain.OverallHealthStatus,
		&chain.OverallHealthUpdatedAt,
	)

	if err != nil {
		if postgres.IsNoRows(err) {
			return nil, fmt.Errorf("chain %d not found", id)
		}
		return nil, fmt.Errorf("failed to query chain %d: %w", id, err)
	}

	return &chain, nil
}

// GetChainRPCEndpoints returns only the chain_id and rpc_endpoints for a chain
func (db *DB) GetChainRPCEndpoints(ctx context.Context, chainID uint64) (*adminStore.ChainRPCEndpoints, error) {
	query := `
		SELECT chain_id, rpc_endpoints
		FROM chains
		WHERE chain_id = $1
	`

	var result adminStore.ChainRPCEndpoints
	err := db.QueryRow(ctx, query, chainID).Scan(&result.ChainID, &result.RPCEndpoints)
	if err != nil {
		if postgres.IsNoRows(err) {
			return nil, fmt.Errorf("chain %d not found", chainID)
		}
		return nil, fmt.Errorf("failed to query chain %d rpc_endpoints: %w", chainID, err)
	}

	return &result, nil
}

// ListChain returns all chains, optionally including deleted ones
func (db *DB) ListChain(ctx context.Context, includeDeleted bool) ([]adminmodels.Chain, error) {
	query := `
		SELECT chain_id, chain_name, rpc_endpoints, paused, deleted, image,
			   min_replicas, max_replicas, reindex_min_replicas, reindex_max_replicas, reindex_scale_threshold,
			   notes, created_at, updated_at,
			   rpc_health_status, rpc_health_message, rpc_health_updated_at,
			   queue_health_status, queue_health_message, queue_health_updated_at,
			   deployment_health_status, deployment_health_message, deployment_health_updated_at,
			   overall_health_status, overall_health_updated_at
		FROM chains
	`

	if !includeDeleted {
		query += ` WHERE deleted = 0`
	}

	query += ` ORDER BY chain_id`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chains []adminmodels.Chain
	for rows.Next() {
		var chain adminmodels.Chain
		err := rows.Scan(
			&chain.ChainID,
			&chain.ChainName,
			&chain.RPCEndpoints,
			&chain.Paused,
			&chain.Deleted,
			&chain.Image,
			&chain.MinReplicas,
			&chain.MaxReplicas,
			&chain.ReindexMinReplicas,
			&chain.ReindexMaxReplicas,
			&chain.ReindexScaleThreshold,
			&chain.Notes,
			&chain.CreatedAt,
			&chain.UpdatedAt,
			&chain.RPCHealthStatus,
			&chain.RPCHealthMessage,
			&chain.RPCHealthUpdatedAt,
			&chain.QueueHealthStatus,
			&chain.QueueHealthMessage,
			&chain.QueueHealthUpdatedAt,
			&chain.DeploymentHealthStatus,
			&chain.DeploymentHealthMessage,
			&chain.DeploymentHealthUpdatedAt,
			&chain.OverallHealthStatus,
			&chain.OverallHealthUpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		chains = append(chains, chain)
	}

	return chains, rows.Err()
}

// HardDeleteChain permanently removes a chain record from the database
func (db *DB) HardDeleteChain(ctx context.Context, chainID uint64) error {
	query := `DELETE FROM chains WHERE chain_id = $1`
	return db.Exec(ctx, query, chainID)
}

// InsertChain inserts a new chain record with ON CONFLICT handling
func (db *DB) InsertChain(ctx context.Context, c *adminmodels.Chain) error {
	query := `
		INSERT INTO chains (
			chain_id, chain_name, rpc_endpoints, paused, deleted, image,
			min_replicas, max_replicas, reindex_min_replicas, reindex_max_replicas, reindex_scale_threshold,
			notes, created_at, updated_at,
			rpc_health_status, rpc_health_message, rpc_health_updated_at,
			queue_health_status, queue_health_message, queue_health_updated_at,
			deployment_health_status, deployment_health_message, deployment_health_updated_at,
			overall_health_status, overall_health_updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
		ON CONFLICT (chain_id) DO UPDATE SET
			chain_name = EXCLUDED.chain_name,
			rpc_endpoints = EXCLUDED.rpc_endpoints,
			paused = EXCLUDED.paused,
			deleted = EXCLUDED.deleted,
			image = EXCLUDED.image,
			min_replicas = EXCLUDED.min_replicas,
			max_replicas = EXCLUDED.max_replicas,
			reindex_min_replicas = EXCLUDED.reindex_min_replicas,
			reindex_max_replicas = EXCLUDED.reindex_max_replicas,
			reindex_scale_threshold = EXCLUDED.reindex_scale_threshold,
			notes = EXCLUDED.notes,
			updated_at = EXCLUDED.updated_at,
			rpc_health_status = EXCLUDED.rpc_health_status,
			rpc_health_message = EXCLUDED.rpc_health_message,
			rpc_health_updated_at = EXCLUDED.rpc_health_updated_at,
			queue_health_status = EXCLUDED.queue_health_status,
			queue_health_message = EXCLUDED.queue_health_message,
			queue_health_updated_at = EXCLUDED.queue_health_updated_at,
			deployment_health_status = EXCLUDED.deployment_health_status,
			deployment_health_message = EXCLUDED.deployment_health_message,
			deployment_health_updated_at = EXCLUDED.deployment_health_updated_at,
			overall_health_status = EXCLUDED.overall_health_status,
			overall_health_updated_at = EXCLUDED.overall_health_updated_at
	`

	return db.Exec(ctx, query,
		c.ChainID,
		c.ChainName,
		c.RPCEndpoints,
		c.Paused,
		c.Deleted,
		c.Image,
		c.MinReplicas,
		c.MaxReplicas,
		c.ReindexMinReplicas,
		c.ReindexMaxReplicas,
		c.ReindexScaleThreshold,
		c.Notes,
		c.CreatedAt,
		c.UpdatedAt,
		c.RPCHealthStatus,
		c.RPCHealthMessage,
		c.RPCHealthUpdatedAt,
		c.QueueHealthStatus,
		c.QueueHealthMessage,
		c.QueueHealthUpdatedAt,
		c.DeploymentHealthStatus,
		c.DeploymentHealthMessage,
		c.DeploymentHealthUpdatedAt,
		c.OverallHealthStatus,
		c.OverallHealthUpdatedAt,
	)
}

// UpdateRPCHealth updates the RPC health status for a chain
func (db *DB) UpdateRPCHealth(ctx context.Context, chainID uint64, status, message string) error {
	query := `
		UPDATE chains
		SET rpc_health_status = $1,
		    rpc_health_message = $2,
		    rpc_health_updated_at = $3,
		    updated_at = $3
		WHERE chain_id = $4
	`

	now := time.Now()
	return db.Exec(ctx, query, status, message, now, chainID)
}

// DeleteEndpointsForChain deletes all RPC endpoints for a chain
func (db *DB) DeleteEndpointsForChain(ctx context.Context, chainID uint64) error {
	query := `DELETE FROM rpc_endpoints WHERE chain_id = $1`
	return db.Exec(ctx, query, chainID)
}
