package types

// EntityInfo describes a database entity
type EntityInfo struct {
	Name        string `json:"name"`
	TableName   string `json:"table_name"`
	StagingName string `json:"staging_name"`
	RoutePath   string `json:"route_path"`
}
