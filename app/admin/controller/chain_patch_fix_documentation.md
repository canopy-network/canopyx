# HandleChainPatch MinReplicas/MaxReplicas Fix

## Problem
The PATCH endpoint for updating chains (`HandleChainPatch` function) was NOT updating the `MinReplicas` or `MaxReplicas` fields. This caused the controller to scale deployments incorrectly when users updated these values via the UI.

## Root Cause
The `HandleChainPatch` function was missing the logic to handle updates to the `MinReplicas` and `MaxReplicas` fields. While other fields like `ChainName`, `Paused`, `Deleted`, and `RPCEndpoints` were being updated, the replica fields were being ignored completely.

## Solution
Added support for updating `MinReplicas` and `MaxReplicas` with proper validation:

### Implementation Details (lines 774-789 in chain.go)

```go
// Update MinReplicas if provided (non-zero value)
if in.MinReplicas > 0 {
    cur.MinReplicas = in.MinReplicas
}

// Update MaxReplicas if provided (non-zero value)
if in.MaxReplicas > 0 {
    cur.MaxReplicas = in.MaxReplicas
}

// Validate that MaxReplicas >= MinReplicas after updates
if cur.MaxReplicas < cur.MinReplicas {
    w.WriteHeader(http.StatusBadRequest)
    _ = json.NewEncoder(w).Encode(map[string]string{"error": "max_replicas must be greater than or equal to min_replicas"})
    return
}
```

### Key Features:
1. **Non-zero value check**: Only updates the fields if they are provided (non-zero), following PATCH semantics
2. **Validation**: Ensures `MaxReplicas >= MinReplicas` after the update
3. **Error handling**: Returns a 400 Bad Request with an error message if validation fails
4. **Consistency**: Follows the same pattern as the `HandleChainsUpsert` function

## Testing Scenarios

### Successful Updates:
- ✅ Update both MinReplicas and MaxReplicas together
- ✅ Update only MinReplicas (MaxReplicas unchanged)
- ✅ Update only MaxReplicas (MinReplicas unchanged)
- ✅ Allow MaxReplicas = MinReplicas (for fixed scaling)

### Validation Failures:
- ❌ Reject when MaxReplicas < MinReplicas after update
- ❌ Reject when updating MaxReplicas to a value less than current MinReplicas
- ❌ Reject when updating MinReplicas to a value greater than current MaxReplicas

### Edge Cases:
- Zero values are ignored (no update) - proper PATCH behavior
- Other fields remain unchanged when updating replicas

## API Usage Examples

### Update both min and max replicas:
```bash
curl -X PATCH http://localhost:8080/admin/chains/my-chain \
  -H "Content-Type: application/json" \
  -d '{"min_replicas": 2, "max_replicas": 5}'
```

### Update only max replicas:
```bash
curl -X PATCH http://localhost:8080/admin/chains/my-chain \
  -H "Content-Type: application/json" \
  -d '{"max_replicas": 10}'
```

### Invalid request (will return 400):
```bash
curl -X PATCH http://localhost:8080/admin/chains/my-chain \
  -H "Content-Type: application/json" \
  -d '{"min_replicas": 10, "max_replicas": 5}'
# Returns: {"error": "max_replicas must be greater than or equal to min_replicas"}
```

## Impact
This fix ensures that:
1. The UI can properly update replica settings for chains
2. The controller will scale deployments according to the updated values
3. Invalid configurations are prevented at the API level
4. The system maintains consistency between min and max replica values