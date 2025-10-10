package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"

	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/zap"
)

// AdminDB represents a database connection for handling indexing operations and tracking progress.
type AdminDB struct {
	Client
	Name string
}

// Close terminates the underlying ClickHouse connection.
func (db *AdminDB) Close() error {
	return db.Db.Close()
}

// Gap represents a missing height range (gap) in the indexing progress for a specific blockchain.
type Gap struct {
	From uint64 `ch:"from_h"`
	To   uint64 `ch:"to_h"`
}

// InitializeDB ensures the required database and tables for indexing are created if they do not already exist.
func (db *AdminDB) InitializeDB(ctx context.Context) error {
	db.Logger.Debug("Initializing indexer database", zap.String("name", db.Name))

	db.Logger.Debug("Initialize chains model", zap.String("name", db.Name))
	if err := admin.InitChains(ctx, db.Db); err != nil {
		return err
	}

	db.Logger.Debug("Initialize index_progress model", zap.String("name", db.Name))
	err := admin.InitIndexProgress(ctx, db.Db, db.Name)
	if err != nil {
		return err
	}

	return nil
}

// RecordIndexed records the height of the last indexed block for the provided chain.
func (db *AdminDB) RecordIndexed(ctx context.Context, chainID string, height uint64) error {
	ip := &admin.IndexProgress{
		ChainID:   chainID,
		Height:    height,
		IndexedAt: time.Now().UTC(),
	}

	_, err := db.Db.NewInsert().Model(ip).Exec(ctx)
	return err
}

// LastIndexed returns the latest indexed height for a chain.
// 1) Prefer the summarized ReplacingMergeTree table (index_progress_agg).
// 2) Fallback to max(height) from the raw index_progress if the summary is empty.
func (db *AdminDB) LastIndexed(ctx context.Context, chainID string) (uint64, error) {
	// Try the aggregate first:
	var h uint64
	err := db.Db.NewSelect().
		TableExpr(fmt.Sprintf(`"%s"."index_progress_agg"`, db.Name)).
		ColumnExpr("maxMerge(max_height)").
		Where("chain_id = ?", chainID).
		Scan(ctx, &h)

	if err == nil && h != 0 {
		return h, nil
	}

	// Fallback to the base table if agg is empty (e.g., very first rows)
	var fallback uint64
	if err := db.Db.NewSelect().
		Model((*admin.IndexProgress)(nil)).
		Where("chain_id = ?", chainID).
		ColumnExpr("max(height)").
		Scan(ctx, &fallback); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return fallback, nil
}

// FindGaps returns missing [From, To] heights strictly inside observed heights,
// and does NOT include the trailing gap to 'up to'. The caller should add a tail gap separately.
func (db *AdminDB) FindGaps(ctx context.Context, chainID string) ([]Gap, error) {
	q := fmt.Sprintf(`
		SELECT prev_h + 1 AS from_h, h - 1 AS to_h
		FROM (
		  SELECT
		    height AS h,
		    lagInFrame(height) OVER (
		      PARTITION BY chain_id
		      ORDER BY height
		      ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
		    ) AS prev_h
		  FROM %s.index_progress
		  WHERE chain_id = ?
		  ORDER BY height
		)
		WHERE prev_h IS NOT NULL AND h > prev_h + 1
		ORDER BY from_h
	`, db.Name)

	var rows []Gap
	// Use the ORM's raw + Scan to hydrate into a slice.
	if err := db.Db.NewRaw(q, chainID).Scan(ctx, &rows); err != nil {
		return nil, err
	}

	return rows, nil
}

// EnsureChainsDbs ensures the required database and tables for indexing are created if they do not already exist.
func (db *AdminDB) EnsureChainsDbs(ctx context.Context) (*xsync.Map[string, ChainStore], error) {
	chainDbMap := xsync.NewMap[string, ChainStore]()

	chains, err := db.ListChain(ctx)
	if err != nil {
		return nil, err
	}

	for _, c := range chains {
		chainDb, chainDbErr := NewChainDb(ctx, db.Logger, c.ChainID)
		if chainDbErr != nil {
			return nil, chainDbErr
		}
		chainDbMap.Store(c.ChainID, chainDb)
	}

	return chainDbMap, nil
}

// <verb><actor>

// UpsertChain creates or updates a chain in the database.
func (db *AdminDB) UpsertChain(ctx context.Context, c *admin.Chain) error {
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
		c.MaxReplicas = c.MinReplicas
	}
	if c.MaxReplicas < c.MinReplicas {
		return fmt.Errorf("max_replicas (%d) must be >= min_replicas (%d)", c.MaxReplicas, c.MinReplicas)
	}
	c.Image = strings.TrimSpace(c.Image)

	// Insert (ReplacingMergeTree will treat the same (chain_id) as an upsert by latest UpdatedAt)
	_, err := db.Db.NewInsert().Model(c).Exec(ctx)
	return err
}

// ListChain returns the latest (deduped) row per chain_id.
func (db *AdminDB) ListChain(ctx context.Context) ([]admin.Chain, error) {
	var out []admin.Chain

	err := db.Db.NewSelect().
		Model(&out).
		Final().
		OrderExpr("chain_id").
		Scan(ctx, &out)

	return out, err
}

// GetChain returns the latest (deduped) row for the given chain_id.
func (db *AdminDB) GetChain(ctx context.Context, id string) (*admin.Chain, error) {
	// Get chain by chain_id
	c, err := admin.GetChain(ctx, db.Db, id)

	if err != nil {
		// normalize "no rows" into a friendly error
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("chain %s not found", id)
		}
		return nil, err
	}

	return c, nil
}

// PatchChains applies bulk partial updates by inserting new versioned rows
// into ReplacingMergeTree (models.Chain). It preserves created_at and bumps updated_at.
func (db *AdminDB) PatchChains(ctx context.Context, patches []admin.Chain) error {
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
		if _, insertErr := db.Db.NewInsert().Model(cur).Exec(ctx); insertErr != nil {
			return insertErr
		}
	}
	return nil
}
