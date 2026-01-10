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

// EntityQueryRequest represents query parameters for entity list endpoints
type EntityQueryRequest struct {
	Limit      int    `json:"limit"`
	Cursor     uint64 `json:"cursor"`
	SortDesc   bool   `json:"sort_desc"`
	UseStaging bool   `json:"use_staging"`
}

// EntityQueryResponse represents the paginated response for entity queries
type EntityQueryResponse struct {
	Data       []map[string]interface{} `json:"data"`
	Limit      int                      `json:"limit"`
	NextCursor *uint64                  `json:"next_cursor,omitempty"`
}

// EntityGetRequest represents query parameters for single entity lookups
type EntityGetRequest struct {
	Property   string  `json:"property"`         // Column name to query (hash, address, order_id, etc.) - REQUIRED
	Value      string  `json:"value"`            // Value to search for - REQUIRED
	Height     *uint64 `json:"height,omitempty"` // Optional - if 0 or omitted, get latest; if specified, get at that exact height
	UseStaging bool    `json:"use_staging"`
}
