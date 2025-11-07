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

// CrossChainEntityQueryRequest represents query parameters for cross-chain entity queries
type CrossChainEntityQueryRequest struct {
	ChainIDs     []uint64               `json:"chain_ids,omitempty"`      // Filter by specific chain IDs (e.g., [1,2,3])
	Limit        int                    `json:"limit"`                    // Number of records to return (default: 50, max: 1000)
	Offset       uint64                 `json:"offset"`                   // Offset for pagination
	OrderBy      string                 `json:"order_by,omitempty"`       // Column to sort by (default: height)
	SortDesc     bool                   `json:"sort_desc"`                // Sort order - true for DESC, false for ASC (default: true)
	Filters      map[string]interface{} `json:"filters,omitempty"`        // Dynamic filters based on entity fields
	Aggregate    bool                   `json:"aggregate,omitempty"`      // Whether to return aggregation stats
	GroupByChain bool                   `json:"group_by_chain,omitempty"` // Group results by chain_id
}

// CrossChainEntityQueryResponse represents the response for cross-chain entity queries
type CrossChainEntityQueryResponse struct {
	Data         []map[string]interface{} `json:"data"`                   // Entity records
	Total        uint64                   `json:"total"`                  // Total count across all chains
	Limit        int                      `json:"limit"`                  // Limit used in query
	Offset       uint64                   `json:"offset"`                 // Offset used in query
	HasMore      bool                     `json:"has_more"`               // Whether more records exist
	ChainStats   []ChainStats             `json:"chain_stats,omitempty"`  // Per-chain statistics
	Aggregations map[string]interface{}   `json:"aggregations,omitempty"` // Aggregation results
}

// ChainStats represents statistics for a specific chain in cross-chain queries
type ChainStats struct {
	ChainID      uint64                 `json:"chain_id"`
	Count        uint64                 `json:"count"`
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
}
