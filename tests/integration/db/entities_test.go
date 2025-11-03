package db

import (
	"encoding/json"
	"testing"

	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntities_All tests that All() returns all 18 entities.
func TestEntities_All(t *testing.T) {

	allEntities := entities.All()
	assert.Len(t, allEntities, 18, "should have exactly 18 entities")

	// Verify all entities are non-empty
	for _, entity := range allEntities {
		assert.NotEmpty(t, entity.String(), "entity should not be empty")
	}
}

// TestEntities_AllStrings tests AllStrings() returns string representations.
func TestEntities_AllStrings(t *testing.T) {

	allStrings := entities.AllStrings()
	assert.Len(t, allStrings, 18, "should have exactly 18 entity strings")

	// Verify all strings are non-empty
	for _, s := range allStrings {
		assert.NotEmpty(t, s, "entity string should not be empty")
	}
}

// TestEntities_Count tests Count() returns the correct number.
func TestEntities_Count(t *testing.T) {

	count := entities.Count()
	assert.Equal(t, 18, count, "should have exactly 18 entities")
}

// TestEntities_TableName tests TableName() returns correct production table names.
func TestEntities_TableName(t *testing.T) {

	tests := []struct {
		entity   entities.Entity
		expected string
	}{
		{entities.Blocks, "blocks"},
		{entities.Transactions, "txs"},
		{entities.BlockSummaries, "block_summaries"},
		{entities.Accounts, "accounts"},
		{entities.Events, "events"},
		{entities.Orders, "orders"},
		{entities.Pools, "pools"},
		{entities.DexPrices, "dex_prices"},
		{entities.DexOrders, "dex_orders"},
		{entities.DexDeposits, "dex_deposits"},
		{entities.DexWithdrawals, "dex_withdrawals"},
		{entities.DexPoolPointsByHolder, "dex_pool_points_by_holder"},
		{entities.Params, "params"},
		{entities.Validators, "validators"},
		{entities.ValidatorSigningInfo, "validator_signing_info"},
		{entities.Committees, "committees"},
		{entities.CommitteeValidators, "committee_validators"},
		{entities.PollSnapshots, "poll_snapshots"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			tableName := tt.entity.TableName()
			assert.Equal(t, tt.expected, tableName)
		})
	}
}

// TestEntities_StagingTableName tests StagingTableName() returns correct staging table names.
func TestEntities_StagingTableName(t *testing.T) {

	tests := []struct {
		entity   entities.Entity
		expected string
	}{
		{entities.Blocks, "blocks_staging"},
		{entities.Transactions, "txs_staging"},
		{entities.BlockSummaries, "block_summaries_staging"},
		{entities.Accounts, "accounts_staging"},
		{entities.Events, "events_staging"},
		{entities.Orders, "orders_staging"},
		{entities.Pools, "pools_staging"},
		{entities.DexPrices, "dex_prices_staging"},
		{entities.DexOrders, "dex_orders_staging"},
		{entities.DexDeposits, "dex_deposits_staging"},
		{entities.DexWithdrawals, "dex_withdrawals_staging"},
		{entities.DexPoolPointsByHolder, "dex_pool_points_by_holder_staging"},
		{entities.Params, "params_staging"},
		{entities.Validators, "validators_staging"},
		{entities.ValidatorSigningInfo, "validator_signing_info_staging"},
		{entities.Committees, "committees_staging"},
		{entities.CommitteeValidators, "committee_validators_staging"},
		{entities.PollSnapshots, "poll_snapshots_staging"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			stagingName := tt.entity.StagingTableName()
			assert.Equal(t, tt.expected, stagingName)
		})
	}
}

// TestEntities_IsValid tests IsValid() correctly validates entities.
func TestEntities_IsValid(t *testing.T) {

	// Valid entities
	validEntities := []entities.Entity{
		entities.Blocks,
		entities.Transactions,
		entities.Accounts,
		entities.Events,
		entities.Pools,
		entities.Orders,
		entities.DexPrices,
		entities.DexOrders,
		entities.DexDeposits,
		entities.DexWithdrawals,
		entities.DexPoolPointsByHolder,
		entities.Params,
		entities.Validators,
		entities.ValidatorSigningInfo,
		entities.Committees,
		entities.CommitteeValidators,
		entities.PollSnapshots,
		entities.BlockSummaries,
	}

	for _, entity := range validEntities {
		assert.True(t, entity.IsValid(), "entity %s should be valid", entity)
	}

	// Invalid entities
	invalidEntities := []entities.Entity{
		entities.Entity("invalid"),
		entities.Entity("unknown"),
		entities.Entity(""),
		entities.Entity("blocks_staging"), // staging suffix not allowed in entity names
	}

	for _, entity := range invalidEntities {
		assert.False(t, entity.IsValid(), "entity %s should be invalid", entity)
	}
}

// TestEntities_FromString tests FromString() correctly parses entity names.
func TestEntities_FromString(t *testing.T) {

	tests := []struct {
		name        string
		input       string
		expected    entities.Entity
		shouldError bool
	}{
		{"valid blocks", "blocks", entities.Blocks, false},
		{"valid txs", "txs", entities.Transactions, false},
		{"valid accounts", "accounts", entities.Accounts, false},
		{"valid events", "events", entities.Events, false},
		{"invalid entity", "invalid", entities.Entity(""), true},
		{"empty string", "", entities.Entity(""), true},
		{"staging suffix", "blocks_staging", entities.Entity(""), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, err := entities.FromString(tt.input)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, entity)
			}
		})
	}
}

// TestEntities_MustFromString tests MustFromString() panics on invalid input.
func TestEntities_MustFromString(t *testing.T) {

	// Valid input should not panic
	t.Run("valid", func(t *testing.T) {
		entity := entities.MustFromString("blocks")
		assert.Equal(t, entities.Blocks, entity)
	})

	// Invalid input should panic
	t.Run("invalid", func(t *testing.T) {
		assert.Panics(t, func() {
			entities.MustFromString("invalid")
		})
	})
}

// TestEntities_String tests String() returns correct string representation.
func TestEntities_String(t *testing.T) {

	tests := []struct {
		entity   entities.Entity
		expected string
	}{
		{entities.Blocks, "blocks"},
		{entities.Transactions, "txs"},
		{entities.Accounts, "accounts"},
		{entities.Events, "events"},
		{entities.Pools, "pools"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			str := tt.entity.String()
			assert.Equal(t, tt.expected, str)
		})
	}
}

// TestEntities_MarshalText tests JSON marshaling of entities.
func TestEntities_MarshalText(t *testing.T) {

	entity := entities.Blocks
	data, err := entity.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, []byte("blocks"), data)

	// Test JSON marshaling
	type wrapper struct {
		Entity entities.Entity `json:"entity"`
	}

	w := wrapper{Entity: entities.Transactions}
	jsonData, err := json.Marshal(w)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), `"entity":"txs"`)
}

// TestEntities_UnmarshalText tests JSON unmarshaling of entities.
func TestEntities_UnmarshalText(t *testing.T) {

	var entity entities.Entity
	err := entity.UnmarshalText([]byte("blocks"))
	require.NoError(t, err)
	assert.Equal(t, entities.Blocks, entity)

	// Test invalid entity
	err = entity.UnmarshalText([]byte("invalid"))
	assert.Error(t, err)

	// Test JSON unmarshaling
	type wrapper struct {
		Entity entities.Entity `json:"entity"`
	}

	jsonData := []byte(`{"entity":"txs"}`)
	var w wrapper
	err = json.Unmarshal(jsonData, &w)
	require.NoError(t, err)
	assert.Equal(t, entities.Transactions, w.Entity)
}

// TestEntities_AllEntitiesUnique tests that all entities have unique names.
func TestEntities_AllEntitiesUnique(t *testing.T) {

	allEntities := entities.All()
	seen := make(map[string]bool)

	for _, entity := range allEntities {
		name := entity.String()
		assert.False(t, seen[name], "entity name %s should be unique", name)
		seen[name] = true
	}
}

// TestEntities_TableNamesDoNotContainStaging tests that entity names don't contain "_staging".
func TestEntities_TableNamesDoNotContainStaging(t *testing.T) {

	allEntities := entities.All()
	for _, entity := range allEntities {
		tableName := entity.TableName()
		assert.NotContains(t, tableName, "_staging", "table name %s should not contain '_staging'", tableName)

		// But staging table names should
		stagingName := entity.StagingTableName()
		assert.Contains(t, stagingName, "_staging", "staging table name %s should contain '_staging'", stagingName)
	}
}

// TestEntities_AllEntitiesAreValid tests that all returned entities pass IsValid().
func TestEntities_AllEntitiesAreValid(t *testing.T) {

	allEntities := entities.All()
	for _, entity := range allEntities {
		assert.True(t, entity.IsValid(), "entity %s from All() should be valid", entity)
	}
}

// TestEntities_RoundTripConversion tests converting entity to string and back.
func TestEntities_RoundTripConversion(t *testing.T) {

	allEntities := entities.All()
	for _, original := range allEntities {
		// Convert to string
		str := original.String()

		// Convert back to entity
		converted, err := entities.FromString(str)
		require.NoError(t, err, "should be able to convert back entity %s", original)

		// Should match original
		assert.Equal(t, original, converted, "round-trip conversion should preserve entity")
	}
}
