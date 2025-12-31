## 1. Consumption

* This data is **only** for UI charting of LP performance.
* Charts are **calendar-day based (UTC)**.
* Supported UI ranges: **1d, 7d, 30d, 1y**, derived from daily snapshots.
* UI shows:

  * **per-day deltas**
  * optional **cumulative balance**
* No intraday or per-block charts are required.

## 2. LP Position Identity

An **LP position** is uniquely defined as:

```
lp_position_id = (address, pool_id)
```

Rules:

* Adding liquidity **does not create a new position**.
* Partial withdrawals **do not create a new position**.
* A position exists as long as `address.Points > 0`.
* If `address.Points` reaches zero, the position is **closed**.
* If the same address later re-enters the pool, this is treated as a **new position** with a new lifecycle.

This guarantees:

* One active position per `(address, pool)` at any time.
* Clear start and end boundaries for rewards.

## 3. Source of Truth & Determinism

* All balances are **computed from on-chain state at a deterministic block height**.
* The indexer is **not a source of truth**, only a deterministic executor.
* Given the same `(chain_state, block_height)`, **any indexer must compute identical results**.

No synthetic reward events are emitted by the chain.

## 4. Day Boundary Rule

* A “day” is defined as **00:00:00 → 23:59:59 UTC**.
* The snapshot height for a day is:

> the **highest block with `block_time ≤ 23:59:59 UTC`** for that calendar day

If no block exists for that day:

* The snapshot height is the **most recent prior block**.
* This is recorded explicitly and treated as valid.

This rule is fixed and deterministic.

## 5. Snapshot Trigger Rule (no gaps, no write dependency)

* A **dedicated scheduled indexer worker** runs independently of DEX activity.
* Behavior:

  * Periodically recomputes **today’s snapshot** (e.g. hourly).
  * Finalizes **yesterday** once the UTC boundary passes.
* Snapshot creation **never depends on trades, writes, or state changes**.

This guarantees:

* Zero-activity days still produce records.
* No gaps due to missing writes.

## 6. Snapshot Data Model

For each `(lp_position_id, snapshot_date)`:

```
snapshot_balance =
  (address.Points / totalPoints) * pool_liquidity
```

Stored fields:

* `address`
* `pool_id`
* `snapshot_date` (UTC)
* `snapshot_height`
* `snapshot_balance`
* `last_updated_at`

Only **one row per LP per day** exists.

## 7. Initial Balance & Position Start Rule

* The **initial snapshot** for a position is:

  * The first `snapshot_date` **on or after the block where `address.Points > 0`**.
* There is **no assumed zero snapshot**.
* Rewards are computed only between existing snapshots:

```
daily_reward =
  snapshot_balance(day N) - snapshot_balance(day N-1)
```

If `day N-1` does not exist:

* No reward is shown for that day.

## 8. Partial Withdrawals & Balance Decreases

* Partial withdrawals:

  * Reduce `address.Points`
  * Affect `snapshot_balance` naturally
* **Negative daily rewards are valid** and expected:

  * Pool liquidity can decrease
  * Impermanent loss is real

UI rules:

* Negative values are displayed as-is.
* No clamping or correction is applied.

This keeps accounting honest and chain-derived.

## 9. Position Closure Rule

* If `address.Points == 0` at snapshot time:

  * The position is marked **closed**
  * No further snapshots are created
* Final reward is computed normally from the last two snapshots.

If the address later re-enters:

* A **new position lifecycle** begins.
* Previous history remains immutable.

## 10. Today Update Rule

* “Today” may be recomputed multiple times.
* All updates:

  * Overwrite the same `(address, pool_id, snapshot_date)` row.
* `last_updated_at` reflects freshness for UI display.

## 11. Missing Data & Backfill Rule

* If a `snapshot_date` is missing:

  * The indexer recomputes using the same height-selection rule.
  * Writes idempotently.
* Backfill requires historical state at `snapshot_height`.

If historical state is unavailable:

* The day is marked `unrecoverable = true`
* This is **not terminal**:

  * If state becomes available later, the snapshot may be recomputed.

## 12. Performance & Cost Guarantees

* Snapshot computation is:

  * **O(active LP positions)** per run
  * Independent of block sync speed
* No per-block recomputation.
* Backfill is manual or bounded, not automatic during fast sync.

This avoids the “expensive during high sync” issue.

## 13. Filtering Usage
* By souce_chain_id (chain read)
* By chain_id where the LP staked
* By address

## 14. Non-goals

* No per-block LP accounting.
* No reward events emitted by the chain.
* No dependency on DEX write events.
* No real-time reward guarantees.