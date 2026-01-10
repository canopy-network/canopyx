package admin

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "strings"
    "time"

    "github.com/canopy-network/canopyx/pkg/db/clickhouse"
    adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
    "github.com/canopy-network/canopyx/pkg/utils"
)

// generateNamespaceUID generates a short unique identifier for Temporal namespace naming.
// Returns a 6-character hex string (e.g., "a1b2c3").
func generateNamespaceUID() string {
    b := make([]byte, 3) // 3 bytes = 6 hex chars
    if _, err := rand.Read(b); err != nil {
        // Fallback to timestamp-based if crypto/rand fails
        return fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
    }
    return hex.EncodeToString(b)
}

// initChains creates the chain table using raw SQL.
// Table: ReplicatedReplacingMergeTree(updated_at) ORDER BY (chain_id)
func (db *DB) initChains(ctx context.Context) error {
    schemaSQL := adminmodels.ColumnsToSchemaSQL(adminmodels.ChainColumns)
    engine := clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "updated_at")

    query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id)
	`, db.Name, adminmodels.ChainsTableName, schemaSQL, engine)

    err := db.Db.Exec(ctx, query)
    if err != nil {
        return fmt.Errorf("failed to create chains table: %w", err)
    }
    return nil
}

// UpsertChain creates or updates a chain in the database.
func (db *DB) UpsertChain(ctx context.Context, c *adminmodels.Chain) error {
    now := time.Now()

    // Check if chain already exists to preserve NamespaceUID and CreatedAt
    existing, err := db.GetChain(ctx, c.ChainID)
    if err == nil && existing != nil {
        // Chain exists - preserve NamespaceUID (never regenerate for existing chains)
        c.NamespaceUID = existing.NamespaceUID
        // Preserve original CreatedAt
        c.CreatedAt = existing.CreatedAt
    } else {
        // New chain - generate NamespaceUID and set CreatedAt
        if c.NamespaceUID == "" {
            c.NamespaceUID = generateNamespaceUID()
        }
        if c.CreatedAt.IsZero() {
            c.CreatedAt = now
        }
    }
    c.UpdatedAt = now

    // Dedup endpoints
    c.RPCEndpoints = utils.Dedup(c.RPCEndpoints)

    if c.MinReplicas == 0 {
        c.MinReplicas = 1
    }
    if c.MaxReplicas == 0 {
        c.MaxReplicas = 1 // Changed from 3 to 1 for stability
    }
    if c.MaxReplicas < c.MinReplicas {
        return fmt.Errorf("max_replicas (%d) must be >= min_replicas (%d)", c.MaxReplicas, c.MinReplicas)
    }

    // Reindex replicas default to match normal replicas
    if c.ReindexMinReplicas == 0 {
        c.ReindexMinReplicas = 1
    }
    if c.ReindexMaxReplicas == 0 {
        c.ReindexMaxReplicas = 1
    }

    c.Image = strings.TrimSpace(c.Image)

    if c.RPCHealthStatus == "" {
        c.RPCHealthStatus = adminmodels.ChainStatusUnknown
    }
    if c.QueueHealthStatus == "" {
        c.QueueHealthStatus = adminmodels.ChainStatusUnknown
    }
    if c.DeploymentHealthStatus == "" {
        c.DeploymentHealthStatus = adminmodels.ChainStatusUnknown
    }
    if c.OverallHealthStatus == "" {
        c.OverallHealthStatus = adminmodels.ChainStatusUnknown
    }

    // Insert (ReplacingMergeTree will treat the same (chain_id) as an upsert by latest UpdatedAt)
    return db.InsertChain(ctx, c)
}

// InsertChain inserts a new chain record.
// ReplacingMergeTree will handle deduplication based on updated_at.
func (db *DB) InsertChain(ctx context.Context, c *adminmodels.Chain) error {
    query := fmt.Sprintf(`
		INSERT INTO "%s"."%s" (
			chain_id, chain_name, namespace_uid, rpc_endpoints, paused, deleted, image,
			min_replicas, max_replicas, reindex_min_replicas, reindex_max_replicas, reindex_scale_threshold,
			notes, created_at, updated_at,
			rpc_health_status, rpc_health_message, rpc_health_updated_at,
			queue_health_status, queue_health_message, queue_health_updated_at,
			deployment_health_status, deployment_health_message, deployment_health_updated_at,
			overall_health_status, overall_health_updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, db.Name, adminmodels.ChainsTableName)

    return db.Db.Exec(ctx, query,
        c.ChainID,
        c.ChainName,
        c.NamespaceUID,
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

// GetChain returns the latest (deduped) row for the given chain_id.
func (db *DB) GetChain(ctx context.Context, id uint64) (*adminmodels.Chain, error) {
    query := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s" FINAL
		WHERE chain_id = ?
		LIMIT 1
	`, db.Name, adminmodels.ChainsTableName)

    var chains []adminmodels.Chain
    if err := db.Select(ctx, &chains, query, id); err != nil {
        return nil, fmt.Errorf("failed to query chain %d: %w", id, err)
    }

    if len(chains) == 0 {
        return nil, fmt.Errorf("chain %d not found", id)
    }

    return &chains[0], nil
}

// ChainRPCEndpoints is a lightweight struct for RPC endpoint queries.
// Contains only chain_id (primary key) + rpc_endpoints.
type ChainRPCEndpoints struct {
    ChainID      uint64   `ch:"chain_id"`
    RPCEndpoints []string `ch:"rpc_endpoints"`
}

// ChainNamespaceInfo is a lightweight struct for Temporal namespace queries.
// Contains chain_id and namespace_uid for constructing namespace names.
type ChainNamespaceInfo struct {
    ChainID      uint64 `ch:"chain_id"`
    NamespaceUID string `ch:"namespace_uid"`
}

// GetChainNamespaceInfo returns the chain_id and namespace_uid for a chain.
// This is a lightweight query optimized for Temporal namespace naming.
// The namespace name format is "{chain_id}-{namespace_uid}" (e.g., "5-a1b2c3").
func (db *DB) GetChainNamespaceInfo(ctx context.Context, chainID uint64) (*ChainNamespaceInfo, error) {
    query := fmt.Sprintf(`
		SELECT chain_id, namespace_uid
		FROM "%s"."%s" FINAL
		WHERE chain_id = ?
		LIMIT 1
	`, db.Name, adminmodels.ChainsTableName)

    var results []ChainNamespaceInfo
    if err := db.Select(ctx, &results, query, chainID); err != nil {
        return nil, fmt.Errorf("failed to query chain %d namespace_info: %w", chainID, err)
    }

    if len(results) == 0 {
        return nil, fmt.Errorf("chain %d not found", chainID)
    }

    return &results[0], nil
}

// GetChainRPCEndpoints returns only the chain_id and rpc_endpoints for a chain.
// This is a lightweight query optimized for RPC client creation (2 columns instead of 25).
// Use this when only RPC endpoints are needed (e.g., in indexer activities).
func (db *DB) GetChainRPCEndpoints(ctx context.Context, chainID uint64) (*ChainRPCEndpoints, error) {
    query := fmt.Sprintf(`
		SELECT chain_id, rpc_endpoints
		FROM "%s"."%s" FINAL
		WHERE chain_id = ?
		LIMIT 1
	`, db.Name, adminmodels.ChainsTableName)

    var results []ChainRPCEndpoints
    if err := db.Select(ctx, &results, query, chainID); err != nil {
        return nil, fmt.Errorf("failed to query chain %d rpc_endpoints: %w", chainID, err)
    }

    if len(results) == 0 {
        return nil, fmt.Errorf("chain %d not found", chainID)
    }

    return &results[0], nil
}

// ListChain returns the latest (deduped) row per chain_id.
// If includeDeleted is false (default), only returns active chains (deleted = 0).
// If includeDeleted is true, returns all chains including soft-deleted ones.
func (db *DB) ListChain(ctx context.Context, includeDeleted bool) ([]adminmodels.Chain, error) {
    query := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s" FINAL
	`, db.Name, adminmodels.ChainsTableName)

    if !includeDeleted {
        query += ` WHERE deleted = 0`
    }

    query += ` ORDER BY chain_id`

    var out []adminmodels.Chain
    if err := db.Select(ctx, &out, query); err != nil {
        return nil, err
    }

    return out, nil
}

// HardDeleteChain permanently removes a chain record from the chains table.
// The record will eventually be deleted during a merge operation.
// WARNING: This operation cannot be undone.
func (db *DB) HardDeleteChain(ctx context.Context, chainID uint64) error {
    query := fmt.Sprintf(
        `DELETE FROM "%s"."%s" WHERE chain_id = ?`,
        db.Name,
        adminmodels.ChainsTableName,
    )
    return db.Exec(ctx, query, chainID)
}

// PatchChains applies bulk partial updates by inserting new versioned rows
// into ReplacingMergeTree (models.Chain). It preserves created_at and bumps updated_at.
func (db *DB) PatchChains(ctx context.Context, patches []adminmodels.Chain) error {
    for _, p := range patches {
        cur, err := db.GetChain(ctx, p.ChainID)
        if err != nil {
            return err
        }

        // CRITICAL: Protect soft-deleted chains from being accidentally undeleted
        // If a chain is deleted (cur.Deleted==1) and a patch doesn't explicitly set deleted=1,
        // skip this update to preserve the deleted status
        if cur.Deleted == 1 && p.Deleted == 0 {
            // Chain is soft-deleted, but a patch doesn't include a deleted flag
            // Skip update to prevent other processes from resetting the deleted status
            continue
        }

        // Apply partial updates
        if p.Paused != cur.Paused {
            cur.Paused = p.Paused
        }
        // Only update the deleted field if explicitly set to 1 (soft delete operation)
        // Never set it back to 0 via status updates - use recovery endpoint instead
        if p.Deleted == 1 {
            cur.Deleted = p.Deleted
        }
        if len(p.RPCEndpoints) > 0 {
            // enhance this to check the diff between cur and p.RPCEndpoints
            cur.RPCEndpoints = p.RPCEndpoints
        }

        // Preserve created_at; bump updated_at
        cur.UpdatedAt = time.Now()

        // Insert a new versioned row (ReplacingMergeTree(updated_at))
        if err := db.InsertChain(ctx, cur); err != nil {
            return err
        }
    }
    return nil
}
