--- DONE ---

[x] - Refactor RPC H-1 pattern to use pond worker subpool from activity context (3f43c6f)
[x] - Add `/v1/query/supply` to indexing life cycle with snapshot-on-change (ae4d7e0, c48e0a3)
[x] - Move `poll` into a scheduled workflow that runs every 20s (f509cfe)
[x] - Add `paymentPercents` map - CommitteePayment entity (0536fb5, 3225df6)
[x] - Add `TotalTransactions` from Block RPC to `BlockSummary` (01b4805)
[x] - Add `NetworkID uint64` to Block from Block RPC (01b4805)
[x] - Add `H-1` (snapshots) to `dexprice` - PriceDelta, LocalPoolDelta, RemotePoolDelta (1538660)
[x] - Add `H-1` (snapshots) to pools - AmountDelta, TotalPointsDelta, LPCountDelta (1538660)
[x] - Remove Genesis Activity/Models/etc - Confirmed dead code, safely removed (01b4805)
[x] - Add `Delegate` and `Compound` to CommitteeValidator (4dffaba)
[x] - Remove `Committees []uint64` from validator model (4dffaba)
[x] - Review/Refactor `orders` fields since Canopy split Seller/Buyer - aligned with SellOrder protobuf (f3b44d4)
[x] - Ensure Status for the `orders` are constants: `open`, `complete`, `canceled` (01b4805)
[x] - Update `activity/dex_batch.go` -> `indexer.DexWithdrawal` -> `PointsBurned` (f103678)
[x] - Update `activity/dex_batch.go` -> `indexer.DexDeposit` -> `PointsReceived` (f103678)
[x] - Run critical analysis over BlockSummaries - added supply, order status, DEX state, pool, account, validator counters (c48e0a3, 3467a64, 67566bf, 2a7dac7)
[x] - Add params PR #261 - MinimumStakeForValidators, MinimumStakeForDelegates, MaximumDelegatesPerCommittee (01b4805)
[x] - Review and clean up `pkg/rpc/event_types.go` TODOs (5e4f19b)
[x] - Refactor crosschain.NewStore to accept database name as parameter (01b4805)
[x] - Separate dex-batch from events maps, lookup H-1 for completed orders (c48e0a3)
[x] - Create dex orders constant for state: pending, locked, complete (01b4805)
[x] - /v1/query/double-signers integration (67566bf)
[x] - Parse `startPoll` from send transactions via memo detection (poll_hash extracted)
[x] - Parse `votePoll` from send transactions via memo detection (poll_hash extracted)
[x] - Parse `lockOrder` from send transactions via memo detection (buyer_receive_address, buyer_send_address, buyer_chain_deadline extracted)
[x] - Parse `closeOrder` from send transactions via memo detection (order_id extracted)
[x] - Add BlockSummary counters for memo-based tx types (num_txs_start_poll, num_txs_vote_poll, num_txs_lock_order, num_txs_close_order)

--- PENDING ---

[] - Rework/Review reindex feature - current implementation does not properly handle snapshot-on-change entities
(initial state not captured when indexer starts at height > 1, need to ensure first-seen records are inserted)
[] - Replace current RPC client types with Canopy RPC client

--- DO NOT HANDLE RIGHT NOW UNTIL ALL ABOVE PENDING ARE DONE ---

[] Integration Tests (db layer)
[] Unit Tests (canopy rpc, admin api, workflows, activities)
