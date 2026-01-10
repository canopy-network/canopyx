package controller

import (
	"net/http"
	"sort"

	"github.com/canopy-network/canopyx/app/admin/controller/types"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/go-jose/go-jose/v4/json"
)

// entityRouteMapping maps entity names to their API route paths.
// Multi-word routes use dashes (e.g., block-summaries, dex-prices).
var entityRouteMapping = map[string]string{
	"accounts":                      "accounts",
	"blocks":                        "blocks",
	"block_summaries":               "block-summaries",
	"committees":                    "committees",
	"committee_payments":            "committee-payments",
	"committee_validators":          "committee-validators",
	"dex_deposits":                  "dex-deposits",
	"dex_orders":                    "dex-orders",
	"dex_prices":                    "dex-prices",
	"dex_withdrawals":               "dex-withdrawals",
	"events":                        "events",
	"lp_position_snapshots":         "lp-position-snapshots",
	"orders":                        "orders",
	"params":                        "params",
	"pools":                         "pools",
	"poll_snapshots":                "poll-snapshots",
	"pool_points_by_holder":         "pool-points",
	"proposal_snapshots":            "proposal-snapshots",
	"supply":                        "supply",
	"txs":                           "transactions",
	"validators":                    "validators",
	"validator_double_signing_info": "validator-double-signing-info",
	"validator_non_signing_info":    "validator-non-signing-info",
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

	// Sort entities alphabetically by name
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"entities": result,
	})
}
