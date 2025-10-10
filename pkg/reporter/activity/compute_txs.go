package activity

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/temporal"
)

func (c *Context) ComputeTxsAllChains(ctx context.Context) error {
	chains, err := c.IndexerDB.ListChain(ctx)
	if err != nil {
		return err
	}
	for _, chain := range chains {
		if chain.Paused == 1 {
			continue
		}
		if computeTxsErr := c.computeTxsByChain(ctx, chain.ChainID); computeTxsErr != nil {
			return computeTxsErr
		}
	}
	return nil
}

func (c *Context) computeTxsByChain(ctx context.Context, chainID string) error {
	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := c.NewChainDb(ctx, chainID)
	if chainDbErr != nil {
		return temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	srcDB := chainDb.DatabaseName()         // per-chain database (with txs table)
	reportsDB := c.ReportsDB.DatabaseName() // reports database (with chain_tx_* tables)

	// Recompute windows slightly wider to heal late arrivals.
	const hourlyWindow = "72 HOUR"
	const dailyWindow = "60 DAY"

	// 1) Hourly tx counts
	hourlySQL := fmt.Sprintf(`
		INSERT INTO %s.chain_tx_hourly (chain_id, hour, count, version)
		SELECT
			?,
			toStartOfHour(time) AS hour,
			count()             AS count,
			toUInt64(now64())   AS version
		FROM %s.txs
		WHERE time >= now() - INTERVAL %s
		GROUP BY hour
		ORDER BY hour
	`, reportsDB, srcDB, hourlyWindow)

	if err := chainDb.Exec(ctx, hourlySQL, chainID); err != nil {
		return temporal.NewApplicationError("insert_hourly_failed", err.Error(), nil)
	}

	// 2) Daily tx counts
	dailySQL := fmt.Sprintf(`
		INSERT INTO %s.chain_tx_daily (chain_id, day, count, version)
		SELECT
			?,
			toDate(time)        AS day,
			count()             AS count,
			toUInt64(now64())   AS version
		FROM %s.txs
		WHERE time >= now() - INTERVAL %s
		GROUP BY day
		ORDER BY day
	`, reportsDB, srcDB, dailyWindow)

	if err := chainDb.Exec(ctx, dailySQL, chainID); err != nil {
		return temporal.NewApplicationError("insert_daily_failed", err.Error(), nil)
	}

	// 3) Rolling 24h tx count
	rolling24hSQL := fmt.Sprintf(`
		INSERT INTO %s.chain_tx_24h (chain_id, asof, count, version)
		SELECT
			?,
			now()                               AS asof,
			(SELECT count() FROM %s.txs
			 WHERE time >= now() - INTERVAL 24 HOUR) AS count,
			toUInt64(now64())                   AS version
	`, reportsDB, srcDB)

	if err := chainDb.Exec(ctx, rolling24hSQL, chainID); err != nil {
		return temporal.NewApplicationError("insert_24h_failed", err.Error(), nil)
	}

	return nil
}
