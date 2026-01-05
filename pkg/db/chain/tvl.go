package chain

import (
	"context"
	"fmt"

	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// SumAccountBalances returns the total balance across all accounts.
// Uses argMax to get the latest balance per address, then sums.
func (db *DB) SumAccountBalances(ctx context.Context) (uint64, error) {
	query := fmt.Sprintf(`
		SELECT SUM(latest_amount) as total
		FROM (
			SELECT argMax(amount, height) as latest_amount
			FROM "%s"."%s"
			GROUP BY address
		)
	`, db.Name, indexermodels.AccountsProductionTableName)

	var total uint64
	if err := db.QueryRow(ctx, query).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum account balances: %w", err)
	}

	return total, nil
}

// SumPoolAmounts returns the total amount across all pools.
// Uses argMax to get the latest amount per pool, then sums.
func (db *DB) SumPoolAmounts(ctx context.Context) (uint64, error) {
	query := fmt.Sprintf(`
		SELECT SUM(latest_amount) as total
		FROM (
			SELECT argMax(amount, height) as latest_amount
			FROM "%s"."%s"
			GROUP BY pool_id
		)
	`, db.Name, indexermodels.PoolsProductionTableName)

	var total uint64
	if err := db.QueryRow(ctx, query).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum pool amounts: %w", err)
	}

	return total, nil
}

// SumValidatorStakes returns the total staked amount across all validators.
// Uses argMax to get the latest staked_amount per validator, then sums.
func (db *DB) SumValidatorStakes(ctx context.Context) (uint64, error) {
	query := fmt.Sprintf(`
		SELECT SUM(latest_amount) as total
		FROM (
			SELECT argMax(staked_amount, height) as latest_amount
			FROM "%s"."%s"
			GROUP BY address
		)
	`, db.Name, indexermodels.ValidatorsProductionTableName)

	var total uint64
	if err := db.QueryRow(ctx, query).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum validator stakes: %w", err)
	}

	return total, nil
}

// SumOpenOrderAmounts returns the total amount locked in open orders.
// Uses argMax to get the latest state per order, filters for open status, then sums.
func (db *DB) SumOpenOrderAmounts(ctx context.Context) (uint64, error) {
	query := fmt.Sprintf(`
		SELECT SUM(latest_amount) as total
		FROM (
			SELECT argMax(if(status = 'open', amount_for_sale, 0), height) as latest_amount
			FROM "%s"."%s"
			GROUP BY order_id
		)
	`, db.Name, indexermodels.OrdersProductionTableName)

	var total uint64
	if err := db.QueryRow(ctx, query).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum open order amounts: %w", err)
	}

	return total, nil
}

// SumActiveDexOrderAmounts returns the total amount locked in active DEX orders.
// Active orders are those in 'future' or 'locked' state.
// Uses argMax to get the latest state per order, filters for active states, then sums.
func (db *DB) SumActiveDexOrderAmounts(ctx context.Context) (uint64, error) {
	query := fmt.Sprintf(`
		SELECT SUM(latest_amount) as total
		FROM (
			SELECT argMax(if(state IN ('future', 'locked'), amount_for_sale, 0), height) as latest_amount
			FROM "%s"."%s"
			GROUP BY order_id
		)
	`, db.Name, indexermodels.DexOrdersProductionTableName)

	var total uint64
	if err := db.QueryRow(ctx, query).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum active dex order amounts: %w", err)
	}

	return total, nil
}
