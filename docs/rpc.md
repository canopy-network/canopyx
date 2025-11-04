# RPC Integration

Guide to Canopy RPC client code organization.

## Package Structure

```
pkg/rpc/
├── interfaces.go      # Client interface definition
├── client.go          # HTTP client implementation
├── types.go           # RPC request/response types
├── block.go           # Block fetching methods
├── accounts.go        # Account fetching methods
├── validators.go      # Validator fetching methods
├── committees.go      # Committee fetching methods
├── pools.go           # Pool fetching methods
├── orders.go          # Order fetching methods
└── params.go          # Parameter fetching methods
```

## Core Pattern: RPC(H) vs RPC(H-1)

**Rule**: Always fetch the current state from RPC at height H and previous state at height H-1.

**Never** query database for previous state in activities.

**Why**: Enables stateless execution, gap filling, and re-indexing.

**See implementation**: Any activity in `app/indexer/activity/` (all follow this pattern)

## Client Interface

**Location**: `pkg/rpc/interfaces.go`

**Methods**: All methods accept `height uint64` parameter for historical queries.

**Pattern**:
```go
Block(ctx, height uint64) (*RpcBlock, error)
Validators(ctx, height uint64) ([]*RpcValidator, error)
AccountsByHeight(ctx, height uint64) ([]*RpcAccount, error)
// ... see interfaces.go for complete list
```

## RPC Types

**Location**: `pkg/rpc/types.go`

**Important**: All types match Canopy protobuf definitions exactly.

**Structure**:
- `RpcBlock` - Block header and transactions
- `RpcTransaction` - Transaction data and result
- `RpcAccount` - Account balance
- `RpcValidator` - Validator state (10 fields from Canopy)
- `RpcNonSigner` - Missed block counter
- `RpcCommittee` - Committee metadata
- `RpcPool` - Pool balance
- `RpcOrder` - DEX order state
- `RpcAllParams` - Chain parameters

**Key point**: Types are for RPC unmarshaling only. Business logic goes in models (`pkg/db/models/indexer/`).

**Derived fields**: If a field doesn't exist in Canopy RPC (like Validator.Status), compute it in the model layer, not RPC layer.

## Usage in Activities

**Pattern**: Parallel fetch H and H-1, then compare.

**Example structure** (see `app/indexer/activity/validators.go`):
```go
// 1. Parallel RPC fetch
go func() { current = cli.Validators(ctx, height) }()
go func() { previous = cli.Validators(ctx, height-1) }()

// 2. Build prevMap for O(1) lookup
prevMap := make(map[string]*RpcValidator)
for _, v := range previous {
    prevMap[v.Address] = v
}

// 3. Compare and detect changes
for _, curr := range current {
    prev := prevMap[curr.Address]
    if prev == nil || fieldsChanged(curr, prev) {
        // Insert to staging
    }
}
```

**See**: Any file in `app/indexer/activity/` for real implementation.

## HTTP Client Implementation

**Location**: `pkg/rpc/client.go`

**Endpoints**: All Canopy RPC endpoints via HTTP POST.

**Request format**: `{"height": 123}` in request body.

**Error handling**: Some endpoints are optional (like NonSigners) - handle gracefully.

## Genesis Handling

Height 1 (genesis) has no previous state:

```go
if height == 1 {
    previous = make([]*RpcValidator, 0)
} else {
    previous = cli.Validators(ctx, height-1)
}
```

**See**: Any activity file for genesis handling pattern.

## Key Files to Read

| What                  | File                                 | Why                               |
|-----------------------|--------------------------------------|-----------------------------------|
| Interface definition  | `pkg/rpc/interfaces.go`              | All available methods             |
| RPC types             | `pkg/rpc/types.go`                   | Request/response structures       |
| Client implementation | `pkg/rpc/client.go`                  | HTTP client setup                 |
| Validator methods     | `pkg/rpc/validators.go`              | Example of height parameter usage |
| Activity usage        | `app/indexer/activity/validators.go` | How activities use RPC client     |

## Important Rules

**DO**:
- Always pass height parameter to RPC methods
- Fetch H and H-1 from RPC (never database)
- Match Canopy protobuf types exactly in `types.go`
- Handle optional endpoints gracefully
- Use parallel goroutines for independent fetches

**DON'T**:
- Query database for previous state in activities
- Skip height parameter (breaks historical queries and gap filling)
- Add fields to RPC types that don't exist in Canopy
- Put business logic in RPC types (use models instead)
- Compute derived fields in RPC layer (do in model/activity layer)

## Further Reading

- Canopy RPC endpoints: See Canopy repository for endpoint definitions
- Activity pattern: Read any file in `app/indexer/activity/`
- Model layer: `docs/models.md` (explains where business logic goes)
