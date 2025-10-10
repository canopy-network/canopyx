# Controller & Admin UI Validation

1. **Controller telemetry**
   - Run the controller service (e.g. `tilt up` or `go run ./cmd/controller`).
   - Watch logs for `queue depth metrics`, `replica decision`, and `replica change applied`.
2. **Scaling cooldown**
   - Simulate backlog spikes (enqueue dummy Temporal tasks).
   - Confirm replicas hold for 60s before changing and update after cooldown expires.
3. **Query API smoke**
   - `GET /api/query/{chain}/blocks` and `/api/query/{chain}/txs` for an indexed chain.
   - Verify pagination cursors (`next_cursor`) and record limits behave.
4. **Admin actions + toasts**
   - Run `go run ./cmd/admin` plus `make dev-web`.
   - Exercise pause/resume, head scan, gap scan, and reindex; ensure each shows success/error toast.
5. **Queue health badges**
   - Observe the Chains overview with backlog <10, between 10–999, and ≥1000.
   - Confirm badge colours (green/yellow/red) and tooltip thresholds align with controller constants.
6. **Docs review**
   - Read `AGENTS.md` and `docs/chain-scaling-ui-plan.md` to verify the new workflow notes are accurate.
