package controller

import (
	"net/http"

	"github.com/canopy-network/canopyx/pkg/db/entities"
)

// EntityInfo represents metadata about a database entity.
type EntityInfo struct {
	Name        string `json:"name"`         // "accounts"
	TableName   string `json:"table_name"`   // "accounts"
	StagingName string `json:"staging_name"` // "accounts_staging"
}

// HandleEntities returns list of all available entities.
// This endpoint provides metadata about all database entities in the system,
// including their table names and staging table names.
// GET /entities
func (c *Controller) HandleEntities(w http.ResponseWriter, r *http.Request) {
	allEntities := entities.All()

	result := make([]EntityInfo, len(allEntities))
	for i, entity := range allEntities {
		result[i] = EntityInfo{
			Name:        entity.String(),
			TableName:   entity.TableName(),
			StagingName: entity.StagingTableName(),
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entities": result,
	})
}