package chain

import (
	"context"
	"fmt"
	"time"

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
	defer func() { _ = rows.Close() }()

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
	// Get schema first to know column types
	schema, err := db.GetTableSchema(ctx, tableName)
	if err != nil {
		return nil, 0, false, fmt.Errorf("get schema: %w", err)
	}

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
	defer func() { _ = rows.Close() }()

	// Fetch column names and create type map
	columnNames := rows.Columns()
	typeMap := make(map[string]string)
	for _, col := range schema {
		typeMap[col.Name] = col.Type
	}

	// Read rows with proper type handling
	var results []map[string]interface{}
	for rows.Next() {
		// Create typed pointers based on column types
		valuePtrs := make([]interface{}, len(columnNames))
		for i, colName := range columnNames {
			colType := typeMap[colName]
			valuePtrs[i] = createTypedPointer(colType)
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, 0, false, fmt.Errorf("scan row: %w", err)
		}

		// Build map with actual values
		rowMap := make(map[string]interface{})
		for i, colName := range columnNames {
			rowMap[colName] = derefPointer(valuePtrs[i])
		}
		results = append(results, rowMap)
	}

	hasMore := int64(offset+len(results)) < total

	return results, total, hasMore, rows.Err()
}

// createTypedPointer creates a properly typed pointer for ClickHouse scanning based on column type
func createTypedPointer(colType string) interface{} {
	// Handle common ClickHouse types
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
		return new(time.Time) // DateTime64 types scan to time.Time
	case len(colType) >= 8 && colType[:8] == "DateTime":
		return new(time.Time) // DateTime types scan to time.Time
	case len(colType) > 9 && colType[:9] == "Nullable(":
		// Handle nullable types - use pointer to base type
		innerType := colType[9 : len(colType)-1]
		return createTypedPointer(innerType)
	case len(colType) > 16 && colType[:16] == "LowCardinality(":
		// Handle LowCardinality - use underlying type
		innerType := colType[16 : len(colType)-1]
		return createTypedPointer(innerType)
	default:
		// Fallback to string for unknown types
		return new(string)
	}
}

// derefPointer dereferences a typed pointer to get the actual value
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
