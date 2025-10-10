# Indexer Ops Queue Migration Plan

## Tasks
- [x] Extend Temporal client helper methods for operations queues (e.g., `admin:<chainID>`).
- [x] Move HeadScan/GapScan workflows and related activities into the indexer package and share activity interfaces.
- [x] Update admin scheduling to publish head/gap workflows to the new per-chain operations queue.
- [x] Register a new worker (or multi-queue worker) inside the indexer binary to consume the operations queue.
- [x] Implement Temporal priority routing in headscan so new blocks > last 5k > older backfill.
- [x] Update/extend tests to cover the new queues and priority behaviour.
- [x] Refresh docs/AGENTS instructions once the migration stabilizes.

## Notes
- Admin should retain schedule creation; only execution moves to the indexer worker.
- Aim to keep duplication minimal by reusing existing activity interfaces introduced earlier.
- Priority windows currently assume "new" ≈ last 100 blocks, "recent" ≈ last 5k blocks. Tweak constants as production metrics dictate.
