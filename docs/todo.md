[] Review:

Event Types from Canopy (9 total):
- Validator: EventReward, EventSlash, EventAutoPause, EventAutoBeginUnstaking,
  EventFinishUnstaking
- DEX/Trading: EventDexSwap, EventDexLiquidityDeposit, EventDexLiquidityWithdraw,
  EventOrderBookSwap

Activities that SHOULD use events but DON'T:
1. IndexAccounts - Should query: EventReward, EventSlash, EventFinishUnstaking
2. IndexValidators - Should query: All 5 validator events + EventReward, EventSlash
3. IndexOrders - Should query: EventOrderBookSwap
4. IndexPools - Should query: EventDexLiquidityDeposit, EventDexLiquidityWithdraw,
   EventDexSwap
---

This code is repeated in multiple activities for the RPC H-1 pattern but I think we can run on goroutines issues do to spam
so for the best will be to refactor to use pond workfer subpool from the current activity context.

```go
wg.Add(2)

// Worker 1: Fetch ALL current batches (locked orders across all committees)
go func() {
    defer wg.Done()
    currentBatches, currentErr = cli.AllDexBatchesByHeight(ctx, in.Height)
}()

// Worker 2: Fetch ALL next batches (future orders across all committees)
go func() {
    defer wg.Done()
    nextBatches, nextErr = cli.AllNextDexBatchesByHeight(ctx, in.Height)
}()

// Wait for both workers
wg.Wait()
```

```go
pool := ac.schedulerBatchPool(totalHeights)
group := pool.NewGroupContext(ctx)
groupCtx := group.Context()
group.Add(...)
```


---

[] - /v1/gov/poll does not support query by height, so how should be treated?
[] - on every indexer for a chain id = x when we call /v1/query/orders, should be sent that chainID or 0? Or 0 if chainID=1 (root)
[] - /v1/query/double-signers
[] - votePoll/startPoll/closeOrder/lockOrder
[] - activity/dex_batch.go -> indexer.DexWithdrawal -> PointsBurned should be calculated from pool state change
[] - activity/dex_batch.go -> indexer.DexDeposit -> PointsReceived should be calculated from pool state change

[] Integration Tests (db layer)
[] Unit Tests (canopy rpc, admin api, workflows, activities)
