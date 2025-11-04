package types

// TableSchemaResponse contains table column information
type TableSchemaResponse struct {
	Columns []ColumnSchema `json:"columns"`
}
