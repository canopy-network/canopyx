# Repository Guidelines

## System Overview
CanopyX is a high-throughput blockchain indexer that processes blocks/transactions into ClickHouse, orchestrated by Temporal workflows. The architecture is optimized for **parallel multi-chain indexing** with per-chain isolation.

### Core Services
- **admin** (`cmd/admin`, `app/admin`) - Admin API + Next.js UI; manages chain registration, schedules, triggers head/gap scans
- **indexer** (`cmd/indexer`, `app/indexer`) - Per-chain Temporal worker (2 workers per chain: index queue + ops queue); fetches blocks/txs and writes to ClickHouse
- **controller** (`cmd/controller`, `app/controller`) - K8s orchestrator; spawns one `indexer` Deployment per chain, scales replicas based on Temporal queue depth
- **query** (`cmd/query`) - Public query API for dashboards
- **reporter** (`cmd/reporter`) - Periodic stats aggregation (hourly/daily/24h tx counts)

### Architecture Deep Dive

**Per-Chain Isolation Model:**
- Each chain gets **TWO dedicated Temporal task queues** (same namespace):
  - `index:<chainID>` - IndexBlock workflows (high-throughput, 5000 concurrent activities)
  - `admin:<chainID>` - Ops workflows (HeadScan, GapScan, Scheduler - lightweight, 5 pollers)
- Each chain gets **ONE Kubernetes Deployment** (`indexer-<chainID>`):
  - Runs 2 workers polling both queues
  - Controller auto-scales replicas (min/max) based on `index:<chainID>` backlog depth
  - Env: `CHAIN_ID=<chainID>`, Temporal/ClickHouse creds
- Each chain gets **ONE ClickHouse database** (`<chainID>`):
  - Tables: `blocks`, `txs`, `txs_raw`
  - Created on-demand by indexer at startup

**Temporal Workflow Patterns:**
1. **IndexBlock workflow** (index queue, per-block):
   - PrepareIndexBlock (local) → check if indexed
   - FetchBlockFromRPC (local, fast retry) → RPC call
   - SaveBlock (regular) → ClickHouse write
   - IndexTransactions (regular) → fetch txs, write to ClickHouse
   - SaveBlockSummary (regular) → aggregate stats
   - RecordIndexed (regular) → update `index_progress`
   - Unlimited retries, 2min timeout per activity

2. **HeadScan workflow** (ops queue):
   - Queries chain head vs last indexed
   - If <1000 blocks: schedules directly to index queue (ultra-high priority)
   - If ≥1000 blocks: triggers SchedulerWorkflow child (catch-up mode)

3. **GapScan workflow** (ops queue):
   - Queries DB for missing heights
   - If <1000 total: schedules directly (high priority)
   - If ≥1000 total: triggers SchedulerWorkflow child

4. **SchedulerWorkflow** (ops queue, long-running):
   - Processes large ranges (e.g., 700k blocks) sequentially in 500-block batches
   - ContinueAsNew every 10k blocks to avoid history bloat
   - Rate-limited: 500 workflows/sec (1s delay per batch)
   - Dynamic priority based on block age (ultra-high → ultra-low)

**Controller Scaling Logic:**
- Every 15s reconciliation tick
- Fetches queue stats from Temporal API (cached 30s)
- Computes desired replicas: `min + ceil(ratio * (max - min))`
  - `backlog < 10`: scale to min
  - `backlog > 1000`: scale to max
  - In between: linear interpolation
- 60s cooldown on scale changes
- Updates ClickHouse with queue/deployment health status

**Data Flow:**
- RPC → IndexBlock workflow → ClickHouse per-chain DB → Query API
- Very little data in ClickHouse currently (blocks/txs only, no deep entity indexing yet)

## Project Structure & Module Organization
- Services: `cmd/<service>` entries with orchestration in `app/<service>`; update those pairs together.
- Shared Go libraries live in `pkg/` (database, Temporal, RPC, logging, utils); avoid duplicating helpers elsewhere.
- Web admin code sits in `web/admin/` with static exports under `web/admin/out`. Infra assets stay in the root `Makefile`, Dockerfiles, `Tiltfile`, and `deploy/`.
- Indexer binaries poll per-chain queues: `index:<chainID>` for block workflows and `admin:<chainID>` for head/gap maintenance tasks.

## Admin UI Workflows
- Chain metadata now includes `image`, `min_replicas`, `max_replicas`, and optional `notes`; the `/api/chains` create/update endpoints expect these fields and the UI form persists optimistic updates.
- Shared toast notifications live in `web/admin/app/components/ToastProvider.tsx`; use the `useToast` hook for success/error feedback on admin actions (edits, reindex, head/gap scans).
- Queue health badges (green/yellow/red) derive from controller thresholds (`low=10`, `high=1000`) via `web/admin/app/lib/constants.ts`; adjust both sides together if scaling logic changes.
- Chain detail surfaces Temporal queue metrics (pollers, backlog age) alongside quick actions for head scans, gap scans, and targeted reindex requests.

## Build, Test, and Development Commands
- `make tools` – install pnpm, golangci-lint, goimports.
- `make deps` – tidy Go modules and install `web/admin` deps.
- `make build-go`, `make build-web`, `make build` – build binaries, the admin UI, or both.
- `tilt up` – boot Temporal + ClickHouse locally; watch via Tilt HUD.
- `go run ./cmd/admin` + `make dev-web` – backend plus hot-reload UI. `make docker-all TAG=vX.Y.Z` – tagged images.
- `make test TEST_PKG=./pkg/indexer/activity RUN=TestIndexBlockWorkflow` – run targeted Go tests; omit vars to test `./...`.

## Coding Style & Naming Conventions
- Enforce gofmt/goimports with `make fmt`; packages stay lower_snake and exported Go identifiers CamelCase.
- Run `make lint` (golangci-lint) before reviews; keep `pkg/` packages narrow in scope.
- Frontend: use functional components, colocate files under `web/admin/app/<feature>`, and keep routes kebab-case.
- Run `pnpm lint` in `web/admin/` for ESLint + Next checks.

## Testing Guidelines
- `make test` (`go test ./...` by default) must pass pre-push; scope runs with `TEST_PKG`/`RUN` when iterating on a suite.
- Mock Temporal and ClickHouse via interfaces or fakes; note any temporary manual checks in PRs until UI automation lands.

## Commit & Pull Request Guidelines
- Commit subjects stay imperative and ≤72 chars (e.g., `Add indexer chain validation`); add bodies only for context or rollback notes.
- Scope PRs per service/module and record schema or workflow changes in the description.
- PRs should list verification (`make test`, Tilt run), attach UI screenshots when relevant, and flag infra impacts (Tilt, Docker, deploy).

## Security & Configuration Tips
- Store secrets outside git; rely on `.env`, Tilt overrides, or cluster secrets. Document required env vars (`ADMIN_TOKEN`, Temporal/ClickHouse credentials).
- When testing remote chains, keep credentials in `deploy/` manifests or isolated kube contexts to avoid leaking production endpoints.

## Agent Interaction & Token Discipline
- Keep prompts, responses, and logs terse; cite paths or line numbers instead of pasting large blocks.
- Prefer summaries or diffs over full command output; link to existing docs rather than repeating background.
- Batch related questions and reuse context variables so agents do not resend redundant data.
