package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// initRPCEndpoints creates the rpc_endpoints table for tracking per-endpoint health.
// Table: ReplicatedReplacingMergeTree(updated_at) ORDER BY (chain_id, endpoint)
// ON CLUSTER ensures a table is created on all replicas.
// DDL completion is controlled by distributed_ddl_task_timeout (default 180s).
func (db *DB) initRPCEndpoints(ctx context.Context) error {
	schemaSQL := adminmodels.ColumnsToSchemaSQL(adminmodels.RPCEndpointColumns)
	engine := clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "updated_at")

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, endpoint)
	`, db.Name, adminmodels.RPCEndpointsTableName, schemaSQL, engine)

	err := db.Db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create %s table: %w", adminmodels.RPCEndpointsTableName, err)
	}
	return nil
}

// UpsertEndpointHealth updates or inserts an endpoint health record.
// ReplacingMergeTree will handle deduplication based on updated_at.
func (db *DB) UpsertEndpointHealth(ctx context.Context, ep *adminmodels.RPCEndpoint) error {
	if ep.Endpoint == "" {
		return fmt.Errorf("endpoint URL cannot be empty")
	}
	if ep.ChainID == 0 {
		return fmt.Errorf("chain ID cannot be zero")
	}
	if ep.UpdatedAt.IsZero() {
		ep.UpdatedAt = time.Now().UTC()
	}

	query := fmt.Sprintf(`
		INSERT INTO "%s"."%s" (
			chain_id, endpoint, status, height, latency_ms, error, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, db.Name, adminmodels.RPCEndpointsTableName)

	return db.Db.Exec(ctx, query,
		ep.ChainID,
		ep.Endpoint,
		ep.Status,
		ep.Height,
		ep.LatencyMs,
		ep.Error,
		ep.UpdatedAt,
	)
}

// GetEndpointsForChain returns all endpoint health records for a chain, ordered by height descending.
func (db *DB) GetEndpointsForChain(ctx context.Context, chainID uint64) ([]adminmodels.RPCEndpoint, error) {
	query := fmt.Sprintf(`
		SELECT
			chain_id, endpoint, status, height, latency_ms, error, updated_at
		FROM "%s"."%s" FINAL
		WHERE chain_id = ?
		ORDER BY height DESC, latency_ms ASC
	`, db.Name, adminmodels.RPCEndpointsTableName)

	var out []adminmodels.RPCEndpoint
	if err := db.SelectWithFinal(ctx, &out, query, chainID); err != nil {
		return nil, err
	}

	return out, nil
}

// GetEndpointsWithMinHeight returns healthy endpoints with height >= minHeight, ordered by height descending.
// This is used by activities to select endpoints that have the required block height.
func (db *DB) GetEndpointsWithMinHeight(ctx context.Context, chainID uint64, minHeight uint64) ([]adminmodels.RPCEndpoint, error) {
	query := fmt.Sprintf(`
		SELECT
			chain_id, endpoint, status, height, latency_ms, error, updated_at
		FROM "%s"."%s" FINAL
		WHERE chain_id = ? AND status = 'healthy' AND height >= ?
		ORDER BY height DESC, latency_ms ASC
	`, db.Name, adminmodels.RPCEndpointsTableName)

	var out []adminmodels.RPCEndpoint
	if err := db.SelectWithFinal(ctx, &out, query, chainID, minHeight); err != nil {
		return nil, err
	}

	return out, nil
}

// DeleteEndpointsForChain removes all endpoint health records for a chain.
// This should be called when a chain is deleted.
func (db *DB) DeleteEndpointsForChain(ctx context.Context, chainID uint64) error {
	query := fmt.Sprintf(`
        DELETE FROM "%s"."%s" WHERE chain_id = ?
	`, db.Name, adminmodels.RPCEndpointsTableName)

	return db.Db.Exec(ctx, query, chainID)
}
