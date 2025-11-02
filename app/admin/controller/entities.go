package controller

import (
	"net/http"

	"github.com/canopy-network/canopyx/app/admin/controller/types"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/go-jose/go-jose/v4/json"
)

// entityRouteMapping maps entity names to their API route paths.
// Multi-word routes use dashes (e.g., block-summaries, dex-prices).
var entityRouteMapping = map[string]string{
	"blocks":          "blocks",
	"block_summaries": "block-summaries",
	"txs":             "transactions",
	"accounts":        "accounts",
	"events":          "events",
	"pools":           "pools",
	"orders":          "orders",
	"dex_prices":      "dex-prices",
}

// HandleEntities returns list of all available entities.
// This endpoint provides metadata about all database entities in the system,
// including their table names, staging table names, and API route paths.
// GET /api/admin/entities
func (c *Controller) HandleEntities(w http.ResponseWriter, r *http.Request) {
	allEntities := entities.All()

	result := make([]types.EntityInfo, len(allEntities))
	for i, entity := range allEntities {
		entityName := entity.String()
		routePath := entityRouteMapping[entityName]
		if routePath == "" {
			// Fallback: use entity name as-is if no mapping exists
			routePath = entityName
		}

		result[i] = types.EntityInfo{
			Name:        entityName,
			TableName:   entity.TableName(),
			StagingName: entity.StagingTableName(),
			RoutePath:   routePath,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"entities": result,
	})
}
