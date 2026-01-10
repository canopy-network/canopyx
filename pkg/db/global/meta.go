package global

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Column represents a database column with its name and type information.
type Column struct {
	Name string `json:"name" ch:"name"`
	Type string `json:"type" ch:"type"`
}

// DescribeTable returns column information for a table using DESCRIBE TABLE.
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

// GetTableSchema retrieves column information for a table from ClickHouse system.columns
func (db *DB) GetTableSchema(ctx context.Context, tableName string) ([]Column, error) {
	query := `
		SELECT name, type
		FROM system.columns
		WHERE database = ? AND table = ?
		ORDER BY position
	`

	var columns []Column
	if err := db.Db.Select(ctx, &columns, query, db.Name, tableName); err != nil {
		return nil, fmt.Errorf("query system.columns: %w", err)
	}

	return columns, nil
}

// GetTableDataPaginated retrieves paginated data from a table with optional height filters.
// Filters by chain_id automatically using PREWHERE for efficient filtering (chain_id is first in ORDER BY).
func (db *DB) GetTableDataPaginated(ctx context.Context, tableName string, limit, offset int, fromHeight, toHeight *uint64) ([]map[string]interface{}, int64, bool, error) {
	schema, err := db.GetTableSchema(ctx, tableName)
	if err != nil {
		return nil, 0, false, fmt.Errorf("get schema: %w", err)
	}

	// Build PREWHERE clause with chain_id filter (most efficient for ClickHouse)
	prewhere := "PREWHERE chain_id = ?"
	args := []interface{}{db.ChainID}

	// Build WHERE clause for height filters
	var whereClause string
	if fromHeight != nil && toHeight != nil {
		whereClause = "WHERE height >= ? AND height <= ?"
		args = append(args, *fromHeight, *toHeight)
	} else if fromHeight != nil {
		whereClause = "WHERE height >= ?"
		args = append(args, *fromHeight)
	} else if toHeight != nil {
		whereClause = "WHERE height <= ?"
		args = append(args, *toHeight)
	}

	// Count total (use PREWHERE for efficiency)
	countQuery := fmt.Sprintf(`SELECT count(*) FROM "%s"."%s" %s %s`, db.Name, tableName, prewhere, whereClause)
	var totalCount uint64
	if err := db.Db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, false, fmt.Errorf("count rows: %w", err)
	}
	total := int64(totalCount)

	// Query data with PREWHERE for efficient chain_id filtering
	dataQuery := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s"
		%s
		%s
		ORDER BY height DESC
		LIMIT ? OFFSET ?
	`, db.Name, tableName, prewhere, whereClause)

	args = append(args, limit, offset)
	rows, err := db.Db.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, false, fmt.Errorf("query data: %w", err)
	}
	defer func() { _ = rows.Close() }()

	columnNames := rows.Columns()
	typeMap := make(map[string]string)
	for _, col := range schema {
		typeMap[col.Name] = col.Type
	}

	var results []map[string]interface{}
	for rows.Next() {
		valuePtrs := make([]interface{}, len(columnNames))
		for i, colName := range columnNames {
			colType := typeMap[colName]
			valuePtrs[i] = createTypedPointer(colType)
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, 0, false, fmt.Errorf("scan row: %w", err)
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columnNames {
			rowMap[colName] = derefPointer(valuePtrs[i])
		}
		results = append(results, rowMap)
	}

	hasMore := int64(offset+len(results)) < total

	return results, total, hasMore, rows.Err()
}

func createTypedPointer(colType string) interface{} {
	switch {
	case colType == "UInt8":
		return new(uint8)
	case colType == "UInt16":
		return new(uint16)
	case colType == "UInt32":
		return new(uint32)
	case colType == "UInt64":
		return new(uint64)
	case colType == "Int8":
		return new(int8)
	case colType == "Int16":
		return new(int16)
	case colType == "Int32":
		return new(int32)
	case colType == "Int64":
		return new(int64)
	case colType == "Float32":
		return new(float32)
	case colType == "Float64":
		return new(float64)
	case colType == "String":
		return new(string)
	case len(colType) >= 10 && colType[:10] == "DateTime64":
		return new(time.Time)
	case len(colType) >= 8 && colType[:8] == "DateTime":
		return new(time.Time)
	case len(colType) > 9 && colType[:9] == "Nullable(":
		innerType := colType[9 : len(colType)-1]
		return createTypedPointer(innerType)
	case len(colType) > 16 && colType[:16] == "LowCardinality(":
		innerType := colType[16 : len(colType)-1]
		return createTypedPointer(innerType)
	default:
		return new(string)
	}
}

func derefPointer(ptr interface{}) interface{} {
	switch v := ptr.(type) {
	case *uint8:
		return *v
	case *uint16:
		return *v
	case *uint32:
		return *v
	case *uint64:
		return *v
	case *int8:
		return *v
	case *int16:
		return *v
	case *int32:
		return *v
	case *int64:
		return *v
	case *float32:
		return *v
	case *float64:
		return *v
	case *string:
		return *v
	case *time.Time:
		return *v
	case *interface{}:
		return *v
	default:
		return ptr
	}
}
