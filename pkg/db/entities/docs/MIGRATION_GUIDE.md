# Migration Guide: Adopting Type-Safe Entities

This guide helps you migrate existing code to use the new type-safe entity system.

## Migration Overview

**Before**: String literals scattered throughout codebase → typos cause runtime errors
**After**: Type-safe constants → typos caught at compile time

**Effort**: Low (most changes are straightforward string replacements)
**Risk**: Low (compile-time verification prevents breaking changes)
**Benefits**: High (eliminates entire class of runtime errors)

## Step-by-Step Migration

### Phase 1: Workflows (Low Risk)

Workflows iterate over entities and pass them to activities.

#### Before

```go
// Hardcoded entity list - easy to typo
entities := []string{"blocks", "txs", "accounts"}

for _, entity := range entities {
    input := PromoteInput{
        Entity: entity,
        Height: height,
    }
    workflow.ExecuteActivity(ctx, activities.PromoteData, input).Get(ctx, nil)
}
```

**Problems**:
- Typo in list: `"acocunts"` → runtime error
- Missing entity: forgot to add `"validators"` → silent failure
- Inconsistent naming: `"txs"` vs `"transactions"`

#### After

```go
import "github.com/canopy-network/canopyx/pkg/db/entities"

// Type-safe iteration - compiler ensures correctness
for _, entity := range entities.All() {
    input := PromoteInput{
        Entity: entity.String(), // Serialize for Temporal
        Height: height,
    }
    workflow.ExecuteActivity(ctx, activities.PromoteData, input).Get(ctx, nil)
}
```

**Benefits**:
- No typos possible (compile-time checked)
- Automatically includes all entities
- Adding new entity: just update `entities.go` (single location)

### Phase 2: Activities (Medium Risk - Validation Required)

Activities receive entity names from workflows and must validate them.

#### Before

```go
type PromoteInput struct {
    Entity string `json:"entity"`
    Height uint64 `json:"height"`
}

func (ac *ActivityContext) PromoteData(ctx context.Context, in PromoteInput) error {
    // No validation - typos cause runtime errors!
    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s_staging WHERE height = ?",
        in.Entity, // Dangerous: could be anything
        in.Entity,
    )
    return ac.db.Exec(ctx, query, in.Height)
}
```

**Problems**:
- No validation → invalid entities cause SQL errors
- Hardcoded `_staging` suffix → easy to make mistakes
- No type safety → SQL injection risk (if entity comes from user input)

#### After

```go
import "github.com/canopy-network/canopyx/pkg/db/entities"

type PromoteInput struct {
    Entity string `json:"entity"` // Still string for Temporal serialization
    Height uint64 `json:"height"`
}

func (ac *ActivityContext) PromoteData(ctx context.Context, in PromoteInput) error {
    // Step 1: Validate entity at entry point
    entity, err := entities.FromString(in.Entity)
    if err != nil {
        // Clear error message listing valid entities
        return fmt.Errorf("invalid entity: %w", err)
    }

    // Step 2: Use type-safe table names
    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
        entity.TableName(),        // Type-safe
        entity.StagingTableName(), // Correct suffix guaranteed
    )

    return ac.db.Exec(ctx, query, in.Height)
}
```

**Benefits**:
- Validation at entry point → fails fast with clear error
- Type-safe table names → no suffix mistakes
- Self-documenting → error message lists valid entities

### Phase 3: Database Operations (High Value)

Replace string concatenation with type-safe methods.

#### Before

```go
// Dangerous: hardcoded suffixes, easy to make mistakes
func PromoteEntity(ctx context.Context, entity string, height uint64) error {
    // Is it "_staging" or "_stage"? Easy to typo.
    stagingTable := entity + "_staging"

    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
        entity,
        stagingTable,
    )
    return db.Exec(ctx, query, height)
}

func CleanStaging(ctx context.Context, entity string, height uint64) error {
    // Another place to typo the suffix
    stagingTable := entity + "_staging"

    query := fmt.Sprintf(
        "ALTER TABLE %s DELETE WHERE height = ?",
        stagingTable,
    )
    return db.Exec(ctx, query, height)
}
```

**Problems**:
- Suffix duplication → maintenance burden
- Typo risk → `"_stage"` vs `"_staging"`
- No validation → any string accepted

#### After

```go
import "github.com/canopy-network/canopyx/pkg/db/entities"

// Type-safe parameter - only valid entities accepted
func PromoteEntity(ctx context.Context, entity entities.Entity, height uint64) error {
    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
        entity.TableName(),        // Consistent, correct
        entity.StagingTableName(), // Consistent, correct
    )
    return db.Exec(ctx, query, height)
}

func CleanStaging(ctx context.Context, entity entities.Entity, height uint64) error {
    query := fmt.Sprintf(
        "ALTER TABLE %s DELETE WHERE height = ?",
        entity.StagingTableName(), // Same method, guaranteed correct
    )
    return db.Exec(ctx, query, height)
}
```

**Benefits**:
- Single source of truth for table naming
- Type-safe parameters → compile-time verification
- Consistent everywhere → less cognitive load

### Phase 4: Logging (Easy Win)

Entity implements `fmt.Stringer` and works seamlessly with structured logging.

#### Before

```go
logger.Info("promoting entity",
    zap.String("entity", entity), // entity is string
    zap.String("staging_table", entity+"_staging"),
    zap.String("prod_table", entity),
)
```

#### After

```go
import "github.com/canopy-network/canopyx/pkg/db/entities"

logger.Info("promoting entity",
    zap.Stringer("entity", entity), // Entity implements Stringer
    zap.String("staging_table", entity.StagingTableName()),
    zap.String("prod_table", entity.TableName()),
)
```

**Benefits**:
- Works with zap's `Stringer` type (more efficient)
- Type-safe table name methods
- Consistent with rest of codebase

## Migration Checklist

Use this checklist to track migration progress:

### Workflows
- [ ] Replace hardcoded entity lists with `entities.All()`
- [ ] Use `.String()` when passing to activities
- [ ] Remove entity name constants from workflow files

### Activities
- [ ] Add `entities.FromString()` validation at entry points
- [ ] Replace `entity + "_staging"` with `entity.StagingTableName()`
- [ ] Replace table name strings with `entity.TableName()`
- [ ] Update activity tests to use entity constants

### Database Layer
- [ ] Update function signatures to accept `entities.Entity` (not `string`)
- [ ] Replace string concatenation with entity methods
- [ ] Update ClickHouse query construction
- [ ] Add entity validation in public APIs

### Logging
- [ ] Use `zap.Stringer("entity", entity)` instead of `zap.String`
- [ ] Use entity methods for table names in logs
- [ ] Ensure entity appears in structured log fields

### Tests
- [ ] Update test fixtures to use entity constants
- [ ] Test invalid entity handling
- [ ] Verify entity validation in activities
- [ ] Add table-driven tests for all entities

### Documentation
- [ ] Update API documentation to reference `entities.Entity`
- [ ] Document entity validation requirements
- [ ] Update code examples in README/docs

## Common Migration Patterns

### Pattern 1: Workflow Entity Iteration

```go
// Before
for _, entityName := range []string{"blocks", "transactions"} {
    // ...
}

// After
for _, entity := range entities.All() {
    entityName := entity.String()
    // ...
}
```

### Pattern 2: Activity Entity Validation

```go
// Before
func Activity(ctx context.Context, entityName string) error {
    // Hope entityName is valid!
}

// After
func Activity(ctx context.Context, entityName string) error {
    entity, err := entities.FromString(entityName)
    if err != nil {
        return err
    }
    // Use entity...
}
```

### Pattern 3: Database Function Parameters

```go
// Before
func PromoteData(ctx context.Context, entityName string, height uint64) error

// After - Option A: Validate at entry
func PromoteData(ctx context.Context, entityName string, height uint64) error {
    entity, err := entities.FromString(entityName)
    if err != nil {
        return err
    }
    // ...
}

// After - Option B: Type-safe parameter (preferred for internal functions)
func PromoteData(ctx context.Context, entity entities.Entity, height uint64) error {
    // entity is already validated by type system
}
```

### Pattern 4: Table Name Construction

```go
// Before
stagingTable := entityName + "_staging"
query := fmt.Sprintf("INSERT INTO %s SELECT * FROM %s", entityName, stagingTable)

// After
query := fmt.Sprintf("INSERT INTO %s SELECT * FROM %s",
    entity.TableName(),
    entity.StagingTableName())
```

## Testing Your Migration

### 1. Compile-Time Verification

After migrating, the compiler will catch errors:

```go
// This won't compile anymore:
processEntity("bloks") // ✗ Type error if processEntity accepts entities.Entity

// Must use:
processEntity(entities.Blocks) // ✓ Compile-time safe
```

### 2. Runtime Validation Testing

Test that your validation works:

```go
func TestActivityRejectsInvalidEntity(t *testing.T) {
    ctx := context.Background()

    // Test invalid entity name
    input := PromoteInput{
        Entity: "invalid_entity",
        Height: 1000,
    }

    err := activityContext.PromoteData(ctx, input)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid entity")
    assert.Contains(t, err.Error(), "valid entities") // Should list valid options
}
```

### 3. Integration Testing

Verify end-to-end workflows work:

```go
func TestWorkflowProcessesAllEntities(t *testing.T) {
    // Verify workflow processes all expected entities
    expectedCount := entities.Count()

    // Run workflow...

    // Verify all entities were processed
    assert.Equal(t, expectedCount, processedCount)
}
```

## Rollback Plan

If you need to rollback:

1. **Keep entity names as strings in Temporal inputs/outputs** ✓ Already done
2. **Validation is additive** → removing it doesn't break anything
3. **Entity constants are just strings** → can replace with literals if needed

Rollback is safe because:
- Temporal workflows serialize entities as strings (no breaking changes)
- Database tables unchanged (just using safer names)
- Can remove `entities` import and use string literals again

## Performance Impact

**Benchmark Results** (see `go test -bench=.`):

```
BenchmarkEntityValidation-32    194602836    6.169 ns/op    0 B/op    0 allocs/op
BenchmarkTableName-32           1000000000   0.2083 ns/op   0 B/op    0 allocs/op
BenchmarkStagingTableName-32    111477273    10.65 ns/op    0 B/op    0 allocs/op
```

**Overhead**: Essentially zero
- Validation: 6 nanoseconds (once per activity)
- TableName(): Optimized to ~0.2 nanoseconds
- StagingTableName(): 10 nanoseconds (single string concat)

**Comparison**: String operations you're already doing are slower than entity operations.

## FAQ

**Q: Do I need to update Temporal workflow history?**
A: No. Entities serialize to strings, so workflow history is unchanged.

**Q: What if I have dynamic entity names from config?**
A: Use `entities.FromString()` to validate them at startup.

**Q: Can I gradually migrate, or must I do it all at once?**
A: Gradual migration is fine. Entities and strings can coexist.

**Q: What about backwards compatibility with old workflows?**
A: Fully compatible. Entities serialize/deserialize as strings.

**Q: How do I add a new entity?**
A: Two-line change in `entities.go` (see README.md "Adding a New Entity")

**Q: What if someone adds an entity but forgets to update `allEntities`?**
A: The `init()` function validates at startup and will panic with a clear error.

## Example: Complete Activity Migration

### Before

```go
type PromoteDataInput struct {
    ChainID string `json:"chainId"`
    Entity  string `json:"entity"`
    Height  uint64 `json:"height"`
}

func (ac *ActivityContext) PromoteData(ctx context.Context, in PromoteDataInput) error {
    logger := ac.logger.With(
        zap.String("entity", in.Entity),
        zap.Uint64("height", in.Height),
    )

    stagingTable := in.Entity + "_staging"

    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
        in.Entity,
        stagingTable,
    )

    logger.Info("promoting data")

    if err := ac.db.Exec(ctx, query, in.Height); err != nil {
        logger.Error("promotion failed", zap.Error(err))
        return err
    }

    logger.Info("promotion succeeded")
    return nil
}
```

### After

```go
import "github.com/canopy-network/canopyx/pkg/db/entities"

type PromoteDataInput struct {
    ChainID string `json:"chainId"`
    Entity  string `json:"entity"` // Still string for Temporal
    Height  uint64 `json:"height"`
}

func (ac *ActivityContext) PromoteData(ctx context.Context, in PromoteDataInput) error {
    // Validate entity early
    entity, err := entities.FromString(in.Entity)
    if err != nil {
        return fmt.Errorf("invalid entity: %w", err)
    }

    logger := ac.logger.With(
        zap.Stringer("entity", entity), // Use Stringer
        zap.Uint64("height", in.Height),
    )

    // Type-safe table names
    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
        entity.TableName(),
        entity.StagingTableName(),
    )

    logger.Info("promoting data")

    if err := ac.db.Exec(ctx, query, in.Height); err != nil {
        logger.Error("promotion failed", zap.Error(err))
        return err
    }

    logger.Info("promotion succeeded")
    return nil
}
```

**Changes**:
1. Added import: `"github.com/canopy-network/canopyx/pkg/db/entities"`
2. Added validation: `entity, err := entities.FromString(in.Entity)`
3. Updated logging: `zap.Stringer("entity", entity)`
4. Type-safe tables: `entity.TableName()` and `entity.StagingTableName()`

**Benefits**:
- Fails fast with clear error for invalid entities
- Guaranteed correct table names
- No more string concatenation bugs
- Better logging with Stringer

## Next Steps

1. **Start with workflows** (lowest risk, high value)
2. **Migrate activities** (add validation)
3. **Update database layer** (type-safe parameters)
4. **Add tests** (verify validation)
5. **Update documentation** (new patterns)

## Support

Questions? See:
- Package documentation: `pkg/db/entities/README.md`
- Examples: `pkg/db/entities/examples_test.go`
- Integration examples: `pkg/db/entities/integration_example.go`