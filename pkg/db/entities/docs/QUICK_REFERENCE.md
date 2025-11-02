
# Entity Package Quick Reference

One-page reference for the most common operations with type-safe entities.

## Import

```go
import "github.com/canopy-network/canopyx/pkg/db/entities"
```

## Available Entities

| Constant | Table Name | Staging Table |
|----------|------------|---------------|
| `entities.Blocks` | `blocks` | `blocks_staging` |
| `entities.Transactions` | `transactions` | `transactions_staging` |
| `entities.TransactionsRaw` | `transactions_raw` | `transactions_raw_staging` |
| `entities.BlockSummaries` | `block_summaries` | `block_summaries_staging` |

## Common Operations

### Get Entity Constant (Compile-Time Safe)

```go
entity := entities.Blocks
```

### Get Table Names

```go
prodTable := entity.TableName()        // "blocks"
stagingTable := entity.StagingTableName() // "blocks_staging"
entityName := entity.String()          // "blocks"
```

### Validate External Input (Runtime)

```go
// Returns error if invalid
entity, err := entities.FromString("blocks")
if err != nil {
    return fmt.Errorf("invalid entity: %w", err)
}

// Panics if invalid (use only for hardcoded values)
entity := entities.MustFromString("blocks")
```

### Iterate All Entities

```go
for _, entity := range entities.All() {
    fmt.Printf("Processing: %s\n", entity)
}

// Or as strings
for _, name := range entities.AllStrings() {
    fmt.Printf("Processing: %s\n", name)
}
```

### Check if Valid

```go
if entity.IsValid() {
    // Entity is recognized
}
```

## Usage Patterns

### In Workflows

```go
func (wc *Context) PromoteWorkflow(ctx workflow.Context, height uint64) error {
    for _, entity := range entities.All() {
        input := PromoteInput{
            Entity: entity.String(), // Serialize for Temporal
            Height: height,
        }
        err := workflow.ExecuteActivity(ctx, wc.PromoteData, input).Get(ctx, nil)
        if err != nil {
            return err
        }
    }
    return nil
}
```

### In Activities

```go
type PromoteInput struct {
    Entity string `json:"entity"`
    Height uint64 `json:"height"`
}

func (ac *ActivityContext) PromoteData(ctx context.Context, in PromoteInput) error {
    // Validate at entry point
    entity, err := entities.FromString(in.Entity)
    if err != nil {
        return fmt.Errorf("invalid entity: %w", err)
    }

    // Use type-safe table names
    query := fmt.Sprintf(
        "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
        entity.TableName(),
        entity.StagingTableName(),
    )

    return ac.db.Exec(ctx, query, in.Height)
}
```

### Database Queries

```go
// Promote data
promoteQuery := fmt.Sprintf(
    "INSERT INTO %s SELECT * FROM %s WHERE height = ?",
    entity.TableName(),
    entity.StagingTableName(),
)

// Clean staging
cleanQuery := fmt.Sprintf(
    "ALTER TABLE %s DELETE WHERE height = ?",
    entity.StagingTableName(),
)

// Select from production
selectQuery := fmt.Sprintf(
    "SELECT * FROM %s WHERE height > ? ORDER BY height DESC LIMIT 100",
    entity.TableName(),
)
```

### Structured Logging

```go
import "go.uber.org/zap"

logger.Info("promoting entity",
    zap.Stringer("entity", entity),        // Entity implements Stringer
    zap.Uint64("height", height),
    zap.String("table", entity.TableName()),
)
```

### Type-Safe Function Parameters

```go
// Internal function - type-safe parameter
func processEntity(ctx context.Context, entity entities.Entity, height uint64) error {
    // entity is guaranteed valid by type system
    return nil
}

// Public API - validate string input
func ProcessEntityByName(ctx context.Context, entityName string, height uint64) error {
    entity, err := entities.FromString(entityName)
    if err != nil {
        return err
    }
    return processEntity(ctx, entity, height)
}
```

## Common Mistakes

### ❌ Don't: Hardcode Table Names

```go
// BAD: Hardcoded, error-prone
query := fmt.Sprintf("INSERT INTO %s ...", "blocks")
stagingTable := "blocks" + "_staging"
```

### ✅ Do: Use Entity Methods

```go
// GOOD: Type-safe, consistent
query := fmt.Sprintf("INSERT INTO %s ...", entity.TableName())
stagingTable := entity.StagingTableName()
```

### ❌ Don't: Skip Validation

```go
// BAD: No validation, runtime errors
func PromoteData(entityName string) error {
    query := fmt.Sprintf("INSERT INTO %s ...", entityName) // Could be anything!
}
```

### ✅ Do: Validate External Input

```go
// GOOD: Validate at entry point
func PromoteData(entityName string) error {
    entity, err := entities.FromString(entityName)
    if err != nil {
        return err
    }
    query := fmt.Sprintf("INSERT INTO %s ...", entity.TableName())
}
```

### ❌ Don't: Use MustFromString with User Input

```go
// BAD: Panics on invalid input
entity := entities.MustFromString(userInput) // DANGER!
```

### ✅ Do: Use FromString for Runtime Values

```go
// GOOD: Returns error for invalid input
entity, err := entities.FromString(userInput)
if err != nil {
    return fmt.Errorf("invalid entity: %w", err)
}
```

## Testing

### Test with All Entities

```go
func TestProcessingAllEntities(t *testing.T) {
    for _, entity := range entities.All() {
        t.Run(entity.String(), func(t *testing.T) {
            err := processEntity(entity)
            assert.NoError(t, err)
        })
    }
}
```

### Test Validation

```go
func TestInvalidEntityRejected(t *testing.T) {
    _, err := entities.FromString("invalid")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid entity")
}
```

### Test Table Names

```go
func TestTableNamingConvention(t *testing.T) {
    for _, entity := range entities.All() {
        expected := entity.TableName() + "_staging"
        actual := entity.StagingTableName()
        assert.Equal(t, expected, actual)
    }
}
```

## Adding a New Entity

1. **Add constant** in `entities.go`:
   ```go
   const Accounts Entity = "accounts"
   ```

2. **Add to slice** in `entities.go`:
   ```go
   var allEntities = []Entity{
       Blocks,
       Transactions,
       TransactionsRaw,
       BlockSummaries,
       Accounts, // <-- Add here
   }
   ```

3. **Done!** Everything else works automatically:
   - `Accounts.TableName()` → `"accounts"`
   - `Accounts.StagingTableName()` → `"accounts_staging"`
   - `entities.All()` includes `Accounts`
   - `entities.FromString("accounts")` works

## Performance

All operations have essentially zero overhead:

- **Validation**: ~6 nanoseconds (O(1) map lookup)
- **TableName()**: ~0.2 nanoseconds (optimized type conversion)
- **StagingTableName()**: ~10 nanoseconds (single string concatenation)

## API Summary

| Function/Method | Returns | Use Case |
|----------------|---------|----------|
| `entity.String()` | `string` | Get entity name |
| `entity.TableName()` | `string` | Get production table name |
| `entity.StagingTableName()` | `string` | Get staging table name |
| `entity.IsValid()` | `bool` | Check if recognized entity |
| `FromString(s)` | `(Entity, error)` | Validate external input |
| `MustFromString(s)` | `Entity` | Convert hardcoded string (panics if invalid) |
| `All()` | `[]Entity` | Get all entities |
| `AllStrings()` | `[]string` | Get all entity names |
| `Count()` | `int` | Number of entities |

## Error Handling

```go
entity, err := entities.FromString(input)
if err != nil {
    // Error format: "unknown entity "foo", valid entities: blocks, transactions, ..."
    return err // Error message is helpful for debugging
}
```

## Best Practices

1. **Use constants in code**: `entities.Blocks` (not `entities.FromString("blocks")`)
2. **Validate external input**: Use `FromString()` for API/config/user input
3. **Use `.String()` for serialization**: JSON, Temporal, external systems
4. **Type-safe parameters**: Accept `entities.Entity`, not `string` (when possible)
5. **Iterate with `All()`**: Never hardcode entity lists

## More Information

- **Full Documentation**: `pkg/db/entities/README.md`
- **Migration Guide**: `pkg/db/entities/MIGRATION_GUIDE.md`
- **Examples**: `pkg/db/entities/examples_test.go`
- **Integration Examples**: `pkg/db/entities/integration_example.go`

---

**Remember**: The goal is compile-time safety. Use entity constants in code, validate at boundaries.