
[] - This code is repeated in multiple activities for the RPC H-1 pattern but I think we can run on goroutines issues do to spam
so for the best will be to refactor to use pond worker subpool from the current activity context.

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

[] - MOVE THIS INTO A SEPARATE SCHEDULED WORKFLOW (@every 20s + OPTIMIZE to avoid duplication) /v1/gov/poll 
[] - On every indexer for a chainID=X when we call `/v1/query/orders`, should be sent that `chainID` or 0? Or 0 if `chainID=1` (root) - Waiting Andrew answer.
[] - Parse `startPoll` from send transactions -> `CheckForPollTransaction` -> `checkMemoForStartPoll`
[] - Parse `votePoll` from send transactions -> `CheckForPollTransaction` -> `checkMemoForVotePoll`
[] - Parse `lockOrder` from send transactions -> `ParseLockOrder`
[] - Parse `closeOrder` from send transactions -> `ParseCloseOrder`
[] - Add `paymentPercents` map between address (val) and committee + percentage
[] - Add `TotalTransactions` from Block RPC to `BlockSummary` (is the lifetime number of transactions)
[] - Add `NetworkID uint64` to Block from Block RPC
[] - Add `H-1` (snapshots) to `dexprice` 
[] - Remove Genesis Activity/Models/etc - Not useful
[] - Join Validator and Committee data indexing handler in a single one, to avoid re-requests the other side to achieve the task below
[] - Add `Delegate` and `Compound` to CommitteeValidator
[] - Remove `Committees []uint64` from validator model.
[] - Review/Refactor `orders` fields since Canopy split Seller/Buyer in two fields - Check not at `pkg/db/models/indexer/order.go`
[] - Ensure Status for the `orders` are: `open`, `complete`, `canceled` - Make them been constants like we are doing for DexOrders.
[] - Update `activity/dex_batch.go` -> `indexer.DexWithdrawal` -> `PointsBurned` - Re check `../canopy` since Andrew already manage to add this (EventType will need to manage Points)
[] - Update `activity/dex_batch.go` -> `indexer.DexDeposit` -> `PointsReceived` - Re check `../canopy` since Andrew already manage to add this (EventType will need to manage Points)
[] - Add `H-1` (snapshots) to pools
[] - Run a critical analysis over the BlockSummaries and any "good to have" aggregated number.
[] - Add the upcoming params [PR #261](https://github.com/canopy-network/canopy/pull/261) - `MinimumStakeForValidators`, `MinimumStakeForDelegates` and `MaximumDelegatesPerCommittee`
[] - Review and update/fix/enhance `pkg/rpc/event_types.go` I leave a bunch of TODOs there - Many of them are related to other tasks on this `todo.md`

[x] - Separate dex-batch (aka current) from events maps, since once the event is triggered, the order will not be at dex-batch endpoint, needs to lookup into H-1
[x] - Create dex orders constant for the state which should be (pending, locked, complete)
[x] - /v1/query/double-signers

--- DO NOT HANDLE RIGHT NOW UNTIL ALL THESE ABOVE ARE DONE ---

[] Integration Tests (db layer)
[] Unit Tests (canopy rpc, admin api, workflows, activities)
