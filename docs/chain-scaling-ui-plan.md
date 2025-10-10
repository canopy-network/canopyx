# Chain Scaling & Admin UI Plan

## Schema & API
- [x] Extend chain model (`pkg/db/models/admin/chain.go`, admin API DTOs) with `image`, `min_replicas`, `max_replicas`, and optional `notes` (no migration script needed; adjust table creation defaults).
- [x] Surface new fields through admin REST endpoints (create/update/list) and ensure temporal schedules reuse updated metadata.
- [x] Backfill ClickHouse table / migration script to add columns with sensible defaults. *(Not required pre-launch; handled via updated table definition.)*

## Controller & Scaling Logic
- [x] Update controller config loader to consume chain-specific `image`/replica bounds instead of env overrides.
- [x] Implement Temporal queue depth polling (`DescribeTaskQueue`) per `index:<chainID>` and derive desired replica counts within `[min,max]`.
- [x] Add cooldown and hysteresis to scaling decisions to avoid flapping; integrate with existing deployment reconciler.
- [x] Emit metrics/logs for queue depth, scaling actions, and failures.

## Web Admin Enhancements
- [x] Fix authentication/session persistence (Next.js middleware, refresh token handling, server-side cookie storage).
- [x] Redesign chain detail page to show min/max replicas, image, status flags, queue depths, latest head/indexed heights, and gap scan status using React + Radix UI primitives.
- [x] Add editable forms for chain properties with validation and optimistic updates.
- [x] Display Temporal queue metrics (pollers, backlog) and gap scan history; use server actions or API proxy layer.
- [x] Provide actions to trigger head/gap scans or request reindex of single height / height ranges.

## Reindex & Activity Updates
- [x] Extend indexer workflows to accept a reindex flag and add a local activity guard that skips blocks when already up to date unless reindexing.
- [x] Ensure reindex requests set distinct workflow IDs or pass metadata to avoid collision with existing runs.
- [x] Add audit logging for manual reindex operations.

## UX & Feedback
- [x] Add toast/notification system for admin actions (updates, reindex trigger, schedule commands).
- [x] Include queue health indicators (green/yellow/red) based on thresholds aligned with scaling logic.

## Testing & Documentation
- [x] Update Temporal/unit tests for new activity logic and controller scaling decisions.
- [x] Document new fields and admin workflows in `AGENTS.md` / README.
