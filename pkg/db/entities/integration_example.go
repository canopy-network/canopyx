package entities

// This file contains integration examples showing how to use entities
// with the actual CanopyX codebase patterns (Temporal workflows, activities, database operations).
//
// These examples are reference implementations demonstrating best practices.

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Example workflow input/output types that use entities
// =====================================================

// PromoteDataInput contains parameters for promoting entity data from staging to production.
type PromoteDataInput struct {
	ChainID    string `json:"chainId"`
	EntityName string `json:"entityName"` // Serialized Entity for Temporal
	Height     uint64 `json:"height"`
}

// PromoteDataOutput contains the result of a promotion operation.
type PromoteDataOutput struct {
	RowsPromoted int     `json:"rowsPromoted"`
	DurationMs   float64 `json:"durationMs"`
}

// CleanPromotedDataInput contains parameters for cleaning staging tables.
type CleanPromotedDataInput struct {
	ChainID    string `json:"chainId"`
	EntityName string `json:"entityName"` // Serialized Entity
	Height     uint64 `json:"height"`
}

// CleanPromotedDataOutput contains the result of a cleanup operation.
type CleanPromotedDataOutput struct {
	RowsDeleted int     `json:"rowsDeleted"`
	DurationMs  float64 `json:"durationMs"`
}

// Example: Activity implementation with entity validation
// =======================================================

// PromoteDataActivity promotes entity data from staging to production tables.
// This demonstrates proper entity validation at activity entry points.
func ExamplePromoteDataActivity(ctx context.Context, in PromoteDataInput) (PromoteDataOutput, error) {
	start := time.Now()

	// Step 1: Validate entity from external input
	entity, err := FromString(in.EntityName)
	if err != nil {
		// Return clear error with list of valid entities
		return PromoteDataOutput{}, fmt.Errorf("invalid entity: %w", err)
	}

	// Step 2: Construct type-safe SQL query
	query := fmt.Sprintf(
		`INSERT INTO %s
		 SELECT * FROM %s
		 WHERE height = ?`,
		entity.TableName(),
		entity.StagingTableName(),
	)

	// Step 3: Execute query (simulated)
	// In real implementation: db.Exec(ctx, query, in.Height)
	fmt.Printf("Executing: %s with height=%d\n", query, in.Height)

	// Step 4: Return results with timing
	elapsed := time.Since(start)
	return PromoteDataOutput{
		RowsPromoted: 100, // Example value
		DurationMs:   float64(elapsed.Milliseconds()),
	}, nil
}

// CleanPromotedDataActivity removes promoted data from staging tables.
// Demonstrates staging table operations.
func ExampleCleanPromotedDataActivity(ctx context.Context, in CleanPromotedDataInput) (CleanPromotedDataOutput, error) {
	start := time.Now()

	// Validate entity
	entity, err := FromString(in.EntityName)
	if err != nil {
		return CleanPromotedDataOutput{}, fmt.Errorf("invalid entity: %w", err)
	}

	// ClickHouse DELETE syntax
	query := fmt.Sprintf(
		`ALTER TABLE %s DELETE WHERE height = ?`,
		entity.StagingTableName(),
	)

	// Execute query (simulated)
	fmt.Printf("Executing: %s with height=%d\n", query, in.Height)

	elapsed := time.Since(start)
	return CleanPromotedDataOutput{
		RowsDeleted: 100,
		DurationMs:  float64(elapsed.Milliseconds()),
	}, nil
}

// Example: Workflow implementation using entities
// ===============================================

// ExamplePromoteAllEntitiesWorkflow demonstrates iterating all entities in a workflow.
// This is the safest pattern: compile-time entity list, runtime validation in activities.
func ExamplePromoteAllEntitiesWorkflow(ctx context.Context, chainID string, height uint64) error {
	// Iterate through all entities type-safely
	for _, entity := range All() {
		// Convert entity to string for serialization to Temporal
		input := PromoteDataInput{
			ChainID:    chainID,
			EntityName: entity.String(), // Serialize for workflow
			Height:     height,
		}

		// Execute promote activity
		// In real workflow: workflow.ExecuteActivity(ctx, activities.PromoteData, input)
		fmt.Printf("Scheduling promote for entity=%s, height=%d, chainID=%s\n", entity, height, input.ChainID)

		// Execute cleanup activity
		cleanInput := CleanPromotedDataInput{
			ChainID:    chainID,
			EntityName: entity.String(),
			Height:     height,
		}
		fmt.Printf("Scheduling cleanup for entity=%s, height=%d, chainID=%s\n", entity, height, cleanInput.ChainID)
	}

	return nil
}

// Example: Database helper functions
// ==================================

// DatabaseHelper demonstrates common database operations with entities.
type DatabaseHelper struct {
	logger *zap.Logger
}

// PromoteEntity promotes a single entity's data from staging to production.
func (h *DatabaseHelper) PromoteEntity(ctx context.Context, entity Entity, height uint64) error {
	// Log with structured logging
	h.logger.Info("promoting entity",
		zap.Stringer("entity", entity), // Entity implements Stringer
		zap.Uint64("height", height),
		zap.String("from_table", entity.StagingTableName()),
		zap.String("to_table", entity.TableName()),
	)

	// Construct query safely
	query := fmt.Sprintf(
		"INSERT INTO %s SELECT * FROM %s WHERE height = ?",
		entity.TableName(),
		entity.StagingTableName(),
	)

	// Execute (simulated)
	_ = query
	return nil
}

// CleanStagingEntity removes data from staging table after promotion.
func (h *DatabaseHelper) CleanStagingEntity(ctx context.Context, entity Entity, height uint64) error {
	h.logger.Info("cleaning staging table",
		zap.Stringer("entity", entity),
		zap.Uint64("height", height),
		zap.String("table", entity.StagingTableName()),
	)

	query := fmt.Sprintf(
		"ALTER TABLE %s DELETE WHERE height = ?",
		entity.StagingTableName(),
	)

	_ = query
	return nil
}

// VerifyEntityData checks if data exists in both staging and production tables.
func (h *DatabaseHelper) VerifyEntityData(ctx context.Context, entity Entity, height uint64) (bool, error) {
	h.logger.Debug("verifying entity data",
		zap.Stringer("entity", entity),
		zap.Uint64("height", height),
	)

	// Check staging
	stagingQuery := fmt.Sprintf(
		"SELECT count(*) FROM %s WHERE height = ?",
		entity.StagingTableName(),
	)

	// Check production
	productionQuery := fmt.Sprintf(
		"SELECT count(*) FROM %s WHERE height = ?",
		entity.TableName(),
	)

	_, _ = stagingQuery, productionQuery
	return true, nil
}

// Example: Batch operations across all entities
// =============================================

// BatchPromoteAllEntities promotes all entities for a given height.
// Demonstrates iteration pattern with error handling.
func ExampleBatchPromoteAllEntities(ctx context.Context, height uint64) error {
	logger := zap.NewExample()

	logger.Info("starting batch promotion",
		zap.Uint64("height", height),
		zap.Int("entity_count", Count()),
	)

	// Track results per entity
	results := make(map[Entity]error)

	// Process each entity
	for _, entity := range All() {
		logger.Info("promoting entity",
			zap.Stringer("entity", entity),
			zap.Uint64("height", height),
		)

		// Promote data
		err := promoteEntityData(ctx, entity, height)
		results[entity] = err

		if err != nil {
			logger.Error("promotion failed",
				zap.Stringer("entity", entity),
				zap.Error(err),
			)
			// Continue processing other entities
			continue
		}

		// Clean staging if promotion succeeded
		if err := cleanStagingData(ctx, entity, height); err != nil {
			logger.Warn("cleanup failed",
				zap.Stringer("entity", entity),
				zap.Error(err),
			)
		}
	}

	// Check for failures
	var failedEntities []Entity
	for entity, err := range results {
		if err != nil {
			failedEntities = append(failedEntities, entity)
		}
	}

	if len(failedEntities) > 0 {
		return fmt.Errorf("promotion failed for entities: %v", failedEntities)
	}

	logger.Info("batch promotion completed",
		zap.Uint64("height", height),
		zap.Int("promoted_count", Count()),
	)

	return nil
}

// Helper functions for batch operations
func promoteEntityData(ctx context.Context, entity Entity, height uint64) error {
	query := fmt.Sprintf(
		"INSERT INTO %s SELECT * FROM %s WHERE height = %d",
		entity.TableName(),
		entity.StagingTableName(),
		height,
	)
	_ = query
	return nil
}

func cleanStagingData(ctx context.Context, entity Entity, height uint64) error {
	query := fmt.Sprintf(
		"ALTER TABLE %s DELETE WHERE height = %d",
		entity.StagingTableName(),
		height,
	)
	_ = query
	return nil
}

// Example: Configuration and validation
// =====================================

// IndexerConfig demonstrates configuration with entity validation.
type IndexerConfig struct {
	// EntitiesToIndex specifies which entities to process.
	// If empty, all entities are processed.
	EntitiesToIndex []string `json:"entitiesToIndex"`
}

// ValidateAndConvertEntities validates configuration entities.
// This shows how to handle user-provided entity lists.
func (c *IndexerConfig) ValidateAndConvertEntities() ([]Entity, error) {
	// If not specified, use all entities
	if len(c.EntitiesToIndex) == 0 {
		return All(), nil
	}

	// Validate each configured entity
	entities := make([]Entity, 0, len(c.EntitiesToIndex))
	for _, name := range c.EntitiesToIndex {
		entity, err := FromString(name)
		if err != nil {
			return nil, fmt.Errorf("invalid entity in config: %w", err)
		}
		entities = append(entities, entity)
	}

	return entities, nil
}

// Example: Metrics and observability
// ==================================

// EntityMetrics demonstrates entity-based metrics collection.
type EntityMetrics struct {
	PromotedRows   map[Entity]int64
	ProcessingTime map[Entity]time.Duration
}

// NewEntityMetrics creates a metrics collector for all entities.
func NewEntityMetrics() *EntityMetrics {
	return &EntityMetrics{
		PromotedRows:   make(map[Entity]int64),
		ProcessingTime: make(map[Entity]time.Duration),
	}
}

// RecordPromotion records metrics for a promotion operation.
func (m *EntityMetrics) RecordPromotion(entity Entity, rows int64, duration time.Duration) {
	m.PromotedRows[entity] += rows
	m.ProcessingTime[entity] += duration
}

// LogMetrics outputs metrics for all entities.
func (m *EntityMetrics) LogMetrics(logger *zap.Logger) {
	logger.Info("entity metrics summary", zap.Int("entity_count", Count()))

	for _, entity := range All() {
		logger.Info("entity metrics",
			zap.Stringer("entity", entity),
			zap.Int64("promoted_rows", m.PromotedRows[entity]),
			zap.Duration("processing_time", m.ProcessingTime[entity]),
		)
	}
}

// Example: Testing utilities
// ==========================

// TestEntityOperations demonstrates test patterns with entities.
func ExampleTestEntityOperations() {
	// Test each entity has correct table names
	for _, entity := range All() {
		// Verify naming conventions
		if entity.StagingTableName() != entity.TableName()+"_staging" {
			fmt.Printf("ERROR: Staging name mismatch for %s\n", entity)
		}
	}

	// Test entity validation
	validEntity, err := FromString("blocks")
	if err != nil {
		fmt.Println("ERROR: Failed to validate 'blocks'")
	} else {
		fmt.Printf("Valid entity: %s\n", validEntity)
	}

	// Test invalid entity
	_, err = FromString("invalid_entity")
	if err == nil {
		fmt.Println("ERROR: Should have rejected invalid entity")
	} else {
		fmt.Println("Correctly rejected invalid entity")
	}

	// Output:
	// Valid entity: blocks
	// Correctly rejected invalid entity
}

// Example: Entity-specific behavior (extension pattern)
// ====================================================

// EntityProcessor defines entity-specific processing logic.
// This demonstrates how to extend entities with custom behavior.
type EntityProcessor struct {
	entity Entity
}

// NewEntityProcessor creates a processor for a specific entity.
func NewEntityProcessor(entity Entity) *EntityProcessor {
	return &EntityProcessor{entity: entity}
}

// GetPartitionKey returns the partition key for this entity.
// Different entities may have different partitioning strategies.
func (p *EntityProcessor) GetPartitionKey() string {
	switch p.entity {
	case Blocks:
		return "height"
	case Transactions:
		return "height"
	case BlockSummaries:
		return "height"
	default:
		return "height" // Default partitioning
	}
}

// GetSortingKey returns the sorting key for this entity.
func (p *EntityProcessor) GetSortingKey() string {
	switch p.entity {
	case Blocks:
		return "height"
	case Transactions:
		return "height, index"
	case BlockSummaries:
		return "height"
	default:
		return "height"
	}
}

// Example usage demonstrating entity-specific configuration.
func ExampleEntityProcessorUsage() {
	for _, entity := range All() {
		processor := NewEntityProcessor(entity)

		fmt.Printf("Entity: %s, Partition: %s, Sort: %s\n",
			entity,
			processor.GetPartitionKey(),
			processor.GetSortingKey(),
		)
	}
}
