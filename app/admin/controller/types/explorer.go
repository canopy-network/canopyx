package types

// TableDataResponse contains paginated table data
type TableDataResponse struct {
	Rows    []map[string]interface{} `json:"rows"`
	Total   int64                    `json:"total"`
	HasMore bool                     `json:"hasMore"`
}

// ColumnSchema describes a table column
type ColumnSchema struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
