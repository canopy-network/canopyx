package chain

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Column represents a database column with its name and type information.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// DescribeTable returns column information for a table using DESCRIBE TABLE.
// Returns a slice of Column structs with name and type information.
func (db *DB) DescribeTable(ctx context.Context, tableName string) ([]Column, error) {
	query := fmt.Sprintf(`DESCRIBE TABLE "%s"."%s"`, db.Name, tableName)

	rows, err := db.Db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("describe table failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			db.Logger.Warn("failed to close rows", zap.Error(closeErr))
		}
	}()

	var columns []Column
	for rows.Next() {
		// We only need name and type, ignore the rest of the columns
		// DESCRIBE TABLE returns: name, type, default_type, default_expression, comment, codec_expression, ttl_expression
		var col Column
		var dummy1, dummy2, dummy3, dummy4, dummy5 string

		if err := rows.Scan(&col.Name, &col.Type, &dummy1, &dummy2, &dummy3, &dummy4, &dummy5); err != nil {
			return nil, fmt.Errorf("failed to scan describe row: %w", err)
		}

		columns = append(columns, col)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return columns, nil
}

// Exec executes an arbitrary query against the chain database.
func (db *DB) Exec(ctx context.Context, query string, args ...any) error {
	return db.Db.Exec(ctx, query, args...)
}

// GetTableSchema retrieves column information for a table from ClickHouse system.columns
func (db *DB) GetTableSchema(ctx context.Context, tableName string) ([]Column, error) {
	query := `
		SELECT name, type
		FROM system.columns
		WHERE database = ? AND table = ?
		ORDER BY position
	`

	rows, err := db.Db.Query(ctx, query, db.Name, tableName)
	if err != nil {
		return nil, fmt.Errorf("query system.columns: %w", err)
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var col Column
		if err := rows.Scan(&col.Name, &col.Type); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		columns = append(columns, col)
	}

	return columns, rows.Err()
}

// GetTableDataPaginated retrieves paginated data from a table with optional height filters
func (db *DB) GetTableDataPaginated(ctx context.Context, tableName string, limit, offset int, fromHeight, toHeight *uint64) ([]map[string]interface{}, int64, bool, error) {
	// Build WHERE clause
	var whereClause string
	var args []interface{}

	if fromHeight != nil || toHeight != nil {
		whereClause = "WHERE "
		if fromHeight != nil && toHeight != nil {
			whereClause += "height >= ? AND height <= ?"
			args = append(args, *fromHeight, *toHeight)
		} else if fromHeight != nil {
			whereClause += "height >= ?"
			args = append(args, *fromHeight)
		} else {
			whereClause += "height <= ?"
			args = append(args, *toHeight)
		}
	}

	// Count total
	countQuery := fmt.Sprintf(`SELECT count(*) FROM "%s"."%s" %s`, db.Name, tableName, whereClause)
	var totalCount uint64
	if err := db.Db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, false, fmt.Errorf("count rows: %w", err)
	}
	total := int64(totalCount)

	// Query data
	dataQuery := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s"
		%s
		ORDER BY height DESC
		LIMIT ? OFFSET ?
	`, db.Name, tableName, whereClause)

	args = append(args, limit, offset)
	rows, err := db.Db.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, false, fmt.Errorf("query data: %w", err)
	}
	defer rows.Close()

	// Fetch column names
	columnNames := rows.Columns()

	// Read rows
	var results []map[string]interface{}
	for rows.Next() {
		// Create slice for row values
		values := make([]interface{}, len(columnNames))
		valuePtrs := make([]interface{}, len(columnNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, 0, false, fmt.Errorf("scan row: %w", err)
		}

		// Build map
		rowMap := make(map[string]interface{})
		for i, colName := range columnNames {
			rowMap[colName] = values[i]
		}
		results = append(results, rowMap)
	}

	hasMore := int64(offset+len(results)) < total

	return results, total, hasMore, rows.Err()
}
