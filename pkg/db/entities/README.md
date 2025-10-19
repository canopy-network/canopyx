# Database Entity Management

Type-safe constants and utilities for managing database entity names throughout the CanopyX indexer system.

## Problem Statement

When working with multiple database entities (blocks, transactions, accounts, validators, etc.), string literals scattered throughout the code create several risks:

- **Runtime errors from typos**: `PromoteData("acocunts", height)` fails at runtime, not compile time
- **Inconsistent naming**: `"blocks"` vs `"block"` vs `"Blocks"`
- **Maintenance burden**: Adding a new entity requires updating multiple files
- **No validation**: Invalid entity names aren't caught until runtime
- **Hard to discover**: What are the valid entity names?

## Solution

This package provides a **single source of truth** for entity names with:

1. **Type safety**: Catch typos at compile time
2. **Compile-time constants**: Zero runtime overhead
3. **Automatic validation**: Invalid entities rejected at runtime
4. **Self-documenting**: All entities defined in one place
5. **Easy maintenance**: Adding an entity is a two-line change

## Quick Start

```go
import "github.com/canopy-network/canopyx/pkg/db/entities"

// Use entity constants (type-safe, compile-time checked)
entity := entities.Blocks

// Get table names
productionTable := entity.TableName()        // "blocks"
stagingTable := entity.StagingTableName()    // "blocks_staging"

// Validate external input
entity, err := entities.FromString("blocks")
if err != nil {
    return fmt.Errorf("invalid entity: %w", err)
}

// Iterate all entities
for _, entity := range entities.All() {
    fmt.Println("Processing:", entity)
}
```

## Available Entities

Current entities (see `entities.go` for canonical list):

- `Blocks` - Blockchain blocks
- `Transactions` - Transaction data
- `TransactionsRaw` - Raw transaction bytes
- `BlockSummaries` - Block aggregations

## API Reference

### Constants

```go
const Blocks Entity = "blocks"
const Transactions Entity = "transactions"
const TransactionsRaw Entity = "transactions_raw"
const BlockSummaries Entity = "block_summaries"
```

### Type: Entity

```go
type Entity string
```

Custom type providing type safety for entity names.

#### Methods

```go
// String returns the entity name as a string
func (e Entity) String() string

// TableName returns the production table name
func (e Entity) TableName() string

// StagingTableName returns the staging table name (entity_staging)
func (e Entity) StagingTableName() string

// IsValid checks if this is a recognized entity
func (e Entity) IsValid() bool
```

### Functions

```go
// FromString safely converts a string to an Entity with validation
func FromString(s string) (Entity, error)

// MustFromString converts a string to Entity, panicking if invalid
// Only use with hardcoded values
func MustFromString(s string) Entity

// All returns all valid entities
func All() []Entity

// AllStrings returns all entity names as strings
func AllStrings() []string

// Count returns the number of entities
func Count() int
```

## Usage Patterns

### In Workflows

```go
func (wc *Context) PromoteWorkflow(ctx workflow.Context, height uint64) error {
    // Iterate all entities type-safely
    for _, entity := range entities.All() {
        input := PromoteInput{
            Entity: entity.String(), // Convert to string for serialization
            Height: height,
        }
        err := workflow.ExecuteActivity(ctx, wc.PromoteData, input).Get(ctx, nil)
        if err != nil {
            return fmt.Errorf("promote %s: %w", entity, err)
        }
    }
    return nil
}
```

### In Activities

```go
type PromoteInput struct {
    Entity string `json:"entity"` // Serialized from workflow
    Height uint64 `json:"height"`
}

func (ac *ActivityContext) PromoteData(ctx context.Context, in PromoteInput) error {
    // Validate entity from external input
    entity, err := entities.FromString(in.Entity)
    if err != nil {
        return fmt.Errorf("invalid entity: %w", err)
    }

    // Use entity safely in queries
    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
        entity.TableName(),
        entity.StagingTableName(),
    )

    return ac.db.Exec(ctx, query, in.Height)
}
```

### In Database Operations

```go
func PromoteEntity(ctx context.Context, db ChainStore, entity entities.Entity, height uint64) error {
    // Type-safe query construction
    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
        entity.TableName(),
        entity.StagingTableName(),
    )

    if err := db.Exec(ctx, query, height); err != nil {
        return fmt.Errorf("promote %s: %w", entity, err)
    }

    // Cleanup staging table
    cleanup := fmt.Sprintf(
        "ALTER TABLE %s DELETE WHERE height = ?",
        entity.StagingTableName(),
    )

    return db.Exec(ctx, cleanup, height)
}
```

### With Logging

```go
import "go.uber.org/zap"

func processEntity(entity entities.Entity, height uint64) {
    // Entity implements Stringer, works seamlessly with zap
    logger.Info("processing entity",
        zap.Stringer("entity", entity),
        zap.Uint64("height", height),
        zap.String("table", entity.TableName()),
    )
}
```

### Validation Examples

```go
// ✓ Compile-time safety with constants
entity := entities.Blocks                    // OK
table := entity.TableName()                  // "blocks"

// ✓ Runtime validation with FromString
entity, err := entities.FromString("blocks") // OK, returns Blocks
if err != nil {
    // Handle invalid entity
}

// ✓ Type-safe function parameters
func processEntity(entity entities.Entity) {
    // Only valid entities can be passed here
}
processEntity(entities.Blocks)               // OK

// ✗ These won't compile (type safety)
processEntity("blocks")                      // Compile error
entity := entities.Bloks                     // Compile error (undefined)

// ✗ These return errors (runtime validation)
entity, err := entities.FromString("bloks")  // Returns error
entity, err := entities.FromString("")       // Returns error
```

## Adding a New Entity

To add a new entity (e.g., `accounts`):

### 1. Add constant in `entities.go`

```go
const (
    Blocks          Entity = "blocks"
    Transactions    Entity = "transactions"
    TransactionsRaw Entity = "transactions_raw"
    BlockSummaries  Entity = "block_summaries"
    Accounts        Entity = "accounts"  // <-- Add here
)
```

### 2. Add to `allEntities` slice

```go
var allEntities = []Entity{
    Blocks,
    Transactions,
    TransactionsRaw,
    BlockSummaries,
    Accounts,  // <-- Add here
}
```

### 3. That's it!

Everything else works automatically:

- `Accounts.TableName()` → `"accounts"`
- `Accounts.StagingTableName()` → `"accounts_staging"`
- `Accounts.IsValid()` → `true`
- `All()` includes `Accounts`
- `FromString("accounts")` works
- Validation happens at startup via `init()`

## Design Decisions

### Why `type Entity string` instead of `const string`?

**Type safety**. With `type Entity string`, you get:

```go
func promote(entity Entity) { ... }

promote(entities.Blocks)    // ✓ OK
promote("blocks")           // ✗ Compile error - type mismatch
```

Plain string constants don't prevent typos:

```go
func promote(entity string) { ... }

promote("bloks")            // ✓ Compiles, ✗ Runtime error!
```

### Why not `iota` enums?

**Compatibility with external systems**. Entity names must be:

1. Serializable to JSON for Temporal workflows
2. Used in SQL queries as table names
3. Readable in logs and error messages

String-based approach works seamlessly:

```go
entity := entities.Blocks
json.Marshal(entity)        // "blocks" - works great
fmt.Sprintf("INSERT INTO %s ...", entity)  // Works in SQL
logger.Info("entity", zap.Stringer("entity", entity))  // Readable logs
```

With `iota`:

```go
const BlocksEnum = iota  // 0
json.Marshal(BlocksEnum) // 0 - not useful
fmt.Sprintf("INSERT INTO %s ...", BlocksEnum)  // "INSERT INTO 0" - broken
```

### Why validation in `init()`?

**Fail fast**. If an entity is misconfigured (empty name, contains `_staging`, etc.), the program panics at startup, not in production.

This catches developer errors immediately:

```go
const BadEntity Entity = ""  // Panics at startup with clear error
```

### Performance Characteristics

- **Constants**: Zero overhead, compile to strings
- **TableName()**: Simple type conversion, no allocation
- **StagingTableName()**: Single string concatenation
- **IsValid()**: O(1) map lookup
- **FromString()**: O(1) map lookup + error allocation

No reflection, no interface overhead, no runtime type checking.

## Testing

Run tests:

```bash
go test ./pkg/db/entities/...
```

Run benchmarks:

```bash
go test -bench=. ./pkg/db/entities/
```

View examples:

```bash
go test -run=Example ./pkg/db/entities/ -v
```

## Migration Guide

### Before (String Literals)

```go
// Workflow
entities := []string{"blocks", "txs", "accounts"}
for _, entity := range entities {
    PromoteData(entity, height)  // Typo risk
}

// Activity
func PromoteData(entity string, height uint64) error {
    // No validation!
    query := fmt.Sprintf("INSERT INTO %s ...", entity)
    // ...
}
```

### After (Type-Safe Entities)

```go
// Workflow
for _, entity := range entities.All() {
    PromoteData(entity.String(), height)  // Type-safe
}

// Activity
type PromoteInput struct {
    Entity string `json:"entity"`
    Height uint64 `json:"height"`
}

func PromoteData(ctx context.Context, in PromoteInput) error {
    // Validate at entry point
    entity, err := entities.FromString(in.Entity)
    if err != nil {
        return err  // Invalid entities rejected
    }

    // Use type-safe entity
    query := fmt.Sprintf("INSERT INTO %s ...", entity.TableName())
    // ...
}
```

### Migration Steps

1. **Add import**: `import "github.com/canopy-network/canopyx/pkg/db/entities"`

2. **Replace constants**:
   ```go
   // Before
   const EntityBlocks = "blocks"

   // After
   import "github.com/canopy-network/canopyx/pkg/db/entities"
   // Use entities.Blocks directly
   ```

3. **Update loops**:
   ```go
   // Before
   for _, entity := range []string{"blocks", "txs"} {
       // ...
   }

   // After
   for _, entity := range entities.All() {
       // Use entity.String() when passing to external systems
   }
   ```

4. **Add validation**:
   ```go
   // Before
   func process(entityName string) {
       // Hope it's valid!
   }

   // After
   func process(entityName string) error {
       entity, err := entities.FromString(entityName)
       if err != nil {
           return err
       }
       // ...
   }
   ```

5. **Update table names**:
   ```go
   // Before
   query := fmt.Sprintf("INSERT INTO %s_staging ...", entity)

   // After
   query := fmt.Sprintf("INSERT INTO %s ...", entity.StagingTableName())
   ```

## Thread Safety

All package functions and methods are safe for concurrent use. The internal state is initialized once at startup and never modified.

## Best Practices

1. **Use constants in code**: `entities.Blocks`, not `entities.FromString("blocks")`
2. **Validate external input**: Always use `FromString()` for user input, API requests, config files
3. **Use `.String()` for serialization**: When passing to JSON, Temporal, or external systems
4. **Don't hardcode table names**: Use `.TableName()` and `.StagingTableName()`
5. **Type-safe function parameters**: Accept `entities.Entity`, not `string`

## FAQ

**Q: Why not just use string constants?**

A: String constants don't provide type safety. Function parameters that accept `string` can receive any string, including typos. With `Entity` type, only valid entities compile.

**Q: What's the performance overhead?**

A: Zero for constants (compile away), minimal for validation (single map lookup).

**Q: Can I use entities in switch statements?**

A: Yes! Entities work in switch, map keys, and all standard Go constructs.

**Q: How do I add a new entity?**

A: Two-line change: add constant and add to `allEntities` slice. Everything else automatic.

**Q: What about backwards compatibility?**

A: Entities serialize to strings for JSON/Temporal, so they're compatible with existing data.

## License

Part of the CanopyX project.