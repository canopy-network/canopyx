package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/puzpuzpuz/xsync/v4"
)

// initChains creates the chain table using raw SQL.
// Table: ReplacingMergeTree(updated_at) ORDER BY (chain_id)
func (db *DB) initChains(ctx context.Context) error {
	schemaSQL := admin.ColumnsToSchemaSQL(admin.ChainColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS chains (
			%s
		) ENGINE = ReplacingMergeTree(updated_at)
		ORDER BY (chain_id)
	`, schemaSQL)
	return db.Db.Exec(ctx, query)
}

// EnsureChainsDbs ensures the required database and tables for indexing are created if they do not already exist.
func (db *DB) EnsureChainsDbs(ctx context.Context) (*xsync.Map[string, chain.Store], error) {
	chainDbMap := xsync.NewMap[string, chain.Store]()

	chains, err := db.ListChain(ctx)
	if err != nil {
		return nil, err
	}

	for _, c := range chains {
		chainDb, chainDbErr := chain.New(ctx, db.Logger, c.ChainID)
		if chainDbErr != nil {
			return nil, chainDbErr
		}
		// Store with string key for map compatibility
		chainDbMap.Store(fmt.Sprintf("%d", c.ChainID), chainDb)
	}

	return chainDbMap, nil
}

// UpsertChain creates or updates a chain in the database.
func (db *DB) UpsertChain(ctx context.Context, c *admin.Chain) error {
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now

	// Dedup endpoints
	c.RPCEndpoints = utils.Dedup(c.RPCEndpoints)

	if c.MinReplicas == 0 {
		c.MinReplicas = 1
	}
	if c.MaxReplicas == 0 {
		c.MaxReplicas = 3 // Default to 3 as per schema
	}
	if c.MaxReplicas < c.MinReplicas {
		return fmt.Errorf("max_replicas (%d) must be >= min_replicas (%d)", c.MaxReplicas, c.MinReplicas)
	}
	c.Image = strings.TrimSpace(c.Image)

	if c.RPCHealthStatus == "" {
		c.RPCHealthStatus = "unknown"
	}
	if c.QueueHealthStatus == "" {
		c.QueueHealthStatus = "unknown"
	}
	if c.DeploymentHealthStatus == "" {
		c.DeploymentHealthStatus = "unknown"
	}
	if c.OverallHealthStatus == "" {
		c.OverallHealthStatus = "unknown"
	}

	// Insert (ReplacingMergeTree will treat the same (chain_id) as an upsert by latest UpdatedAt)
	return db.InsertChain(ctx, c)
}

// InsertChain inserts a new chain record.
// ReplacingMergeTree will handle deduplication based on updated_at.
func (db *DB) InsertChain(ctx context.Context, c *admin.Chain) error {
	query := `
		INSERT INTO chains (
			chain_id, chain_name, rpc_endpoints, paused, deleted, image,
			min_replicas, max_replicas, notes, created_at, updated_at,
			rpc_health_status, rpc_health_message, rpc_health_updated_at,
			queue_health_status, queue_health_message, queue_health_updated_at,
			deployment_health_status, deployment_health_message, deployment_health_updated_at,
			overall_health_status, overall_health_updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return db.Db.Exec(ctx, query,
		c.ChainID,
		c.ChainName,
		c.RPCEndpoints,
		c.Paused,
		c.Deleted,
		c.Image,
		c.MinReplicas,
		c.MaxReplicas,
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
func (db *DB) GetChain(ctx context.Context, id uint64) (*admin.Chain, error) {
	query := `
		SELECT
			chain_id, chain_name, rpc_endpoints, paused, deleted, image,
			min_replicas, max_replicas, notes, created_at, updated_at,
			rpc_health_status, rpc_health_message, rpc_health_updated_at,
			queue_health_status, queue_health_message, queue_health_updated_at,
			deployment_health_status, deployment_health_message, deployment_health_updated_at,
			overall_health_status, overall_health_updated_at
		FROM chains FINAL
		WHERE chain_id = ?
		LIMIT 1
	`

	var c admin.Chain
	err := db.Db.QueryRow(ctx, query, id).Scan(
		&c.ChainID,
		&c.ChainName,
		&c.RPCEndpoints,
		&c.Paused,
		&c.Deleted,
		&c.Image,
		&c.MinReplicas,
		&c.MaxReplicas,
		&c.Notes,
		&c.CreatedAt,
		&c.UpdatedAt,
		&c.RPCHealthStatus,
		&c.RPCHealthMessage,
		&c.RPCHealthUpdatedAt,
		&c.QueueHealthStatus,
		&c.QueueHealthMessage,
		&c.QueueHealthUpdatedAt,
		&c.DeploymentHealthStatus,
		&c.DeploymentHealthMessage,
		&c.DeploymentHealthUpdatedAt,
		&c.OverallHealthStatus,
		&c.OverallHealthUpdatedAt,
	)
	if err != nil {
		// normalize "no rows" into a friendly error
		return nil, fmt.Errorf("chain %d not found: %w", id, err)
	}

	return &c, nil
}

// ListChain returns the latest (deduped) row per chain_id.
func (db *DB) ListChain(ctx context.Context) ([]admin.Chain, error) {
	query := `
		SELECT
			chain_id, chain_name, rpc_endpoints, paused, deleted, image,
			min_replicas, max_replicas, notes, created_at, updated_at,
			rpc_health_status, rpc_health_message, rpc_health_updated_at,
			queue_health_status, queue_health_message, queue_health_updated_at,
			deployment_health_status, deployment_health_message, deployment_health_updated_at,
			overall_health_status, overall_health_updated_at
		FROM chains FINAL
		ORDER BY chain_id
	`

	var out []admin.Chain
	if err := db.Select(ctx, &out, query); err != nil {
		return nil, err
	}

	return out, nil
}

// PatchChains applies bulk partial updates by inserting new versioned rows
// into ReplacingMergeTree (models.Chain). It preserves created_at and bumps updated_at.
func (db *DB) PatchChains(ctx context.Context, patches []admin.Chain) error {
	for _, p := range patches {
		cur, err := db.GetChain(ctx, p.ChainID)
		if err != nil {
			return err
		}

		// Apply partial updates
		if p.Paused != cur.Paused {
			cur.Paused = p.Paused
		}
		if p.Deleted != cur.Deleted {
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
