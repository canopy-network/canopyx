# CanopyX — All‑in‑One README (Overview • Architecture • Dev • Tilt • CI)

> **What this is:** a single, self‑contained doc.  
> It covers how the system works, how to run it locally (with Tilt for infra), how to develop the web app, and how CI/Docker builds are wired.

---

## 1) What is CanopyX?

CanopyX indexes blockchain data per chain into ClickHouse, exposes APIs to operate and query it, and uses Temporal for reliable workflows. It’s split into focused binaries:

- **admin** — Admin API + admin UI (Next.js, built statically for prod; dev server in local mode).
- **indexer** — Per‑chain Temporal worker that pulls blocks/txs and writes to ClickHouse.
- **controller** — Spawns/manages one **indexer** deployment per chain (e.g., in Kubernetes).
- **reporter** — Periodic stats aggregation (hour/day/24h tx counts).
- **query** — Public/query API for charts and dashboards.

**Datastore:** ClickHouse (go‑clickhouse via `github.com/uptrace/go-clickhouse/ch`).  
**Workflow engine:** Temporal.

---

## 2) Repository Layout

```
cmd/
  admin/        # entrypoint for Admin service
  indexer/      # entrypoint for Indexer worker
  controller/   # entrypoint for Controller (K8s provider)
  query/        # entrypoint for Query service
  reporter/     # entrypoint for Reporter worker

app/
  admin/        # admin app composition (server, routes, workflows)
  indexer/      # indexer app composition
  controller/   # controller app composition
  query/        # query app composition
  reporter/     # reporter app composition

pkg/
  db/               # ClickHouse client + models + helpers (uptrace go-clickhouse)
  indexer/          # indexer activities/workflows/types
  reporter/         # reporter activities/workflows
  rpc/              # RPC client (HTTP), block/tx fetchers
  temporal/         # Temporal client + logging adapter
  logging/          # zap config
  utils/            # misc helpers

web/admin/          # Next.js admin (static build for prod; dev server for local)
```

---

## 3) Core Architecture

### 3.1 Temporal & Workers

- **Task queues**
  - `manager` — lightweight admin workflows (gap/head scans, etc.).
  - `index:<chainID>` — per‑chain index queue (isolates heavy catch‑ups so active chains aren’t starved).
  - `reporter` — scheduled/adhoc report jobs.

- **Workers**
  - **admin worker** (in `admin` binary) registers admin workflows/activities for housekeeping (head/gap scans, ensuring schedules exist, etc.).
  - **indexer worker** (in `indexer` binary) runs per‑chain index workflows / activities:
    - `IndexBlock` — fetch block + txs (single RPC roundtrip when possible) and write to ClickHouse in one shot.
  - **reporter worker** (in `reporter` binary) computes tx metrics (hourly/daily/24h).

### 3.2 Controller (Kubernetes Provider)

- Ensures there is **one Deployment per chain** (named `indexer-<chainID>`), with `TASK_QUEUE=index:<chainID>` and env for Temporal + ClickHouse.
- Supports **pause** (scale to zero) and **delete** (remove deployment + HPA).
- Uses a `Provider` interface; the `kubernetes` implementation uses official client‑go APIs.

---

## 4) Data Model (ClickHouse)

We use **model‑driven DDL** with `go-clickhouse` struct tags.

Databases:
1. Indexer: `canopyx_indexer` (default)
   1. `chains`
   2. `index_progress`
   3. `index_progress_agg` (materialized view)
2. Reports: `canopyx_reports` (default)
   1. @TBD 
3. Per-Chain: `<chainID>`
   1. `blocks`
   2. `txs`
   3. `txs_raw`

---

## 5) Local Development

### 5.1 Prereqs

- Go 1.22+ (or the toolchain pinned by the repo).
- Node **22** + `pnpm` (the Makefile can bootstrap via `corepack`).
- Docker Desktop (or Docker Engine).
- **Tilt** (https://tilt.dev) for local infra (ClickHouse and Temporal).

### 5.2 Start Infra with Tilt

We use Tilt to bring up **ClickHouse** and **Temporal** quickly. If your repo already contains a `Tiltfile`, run:

```bash
tilt up
```

### 5.3 Build & Run apps

The **Makefile** exposes common tasks:

```bash
# one‑time
make tools         # installs pnpm, golangci-lint, goimports

# deps
make deps          # go mod tidy + pnpm install for web/admin

# code quality
make fmt           # go fmt + goimports
make lint          # golangci-lint run

# build
make build-web     # Next static export
make build-go      # all Go binaries
make build         # both

# dev web
make dev-web       # NEXT_PUBLIC_API_BASE=http://localhost:3000 PORT=3003 pnpm dev

# docker images
make docker-all    # builds admin/indexer/controller/query/reporter images (no push)
```

**Ports (convention):**  
- `3000` admin API + static UI in prod mode  
- `3001` query API  
- `3002` controller API (Kubernetes Only for now)
- `3003` Next.js dev server (admin UI in development)  
- `7233` Temporal (via Tilt)  
- `8080` Temporal Admin UI (via Tilt)  
- `8123/9000` ClickHouse HTTP/native (via Tilt)

### 5.4 Running the Admin UI (Dev vs. Prod)

- **Dev UX** (hot reload):
  1. Run admin backend (from repo root): `go run ./cmd/admin`
  2. Run Next dev server: `make dev-web`  
     This sets `NEXT_PUBLIC_API_BASE=http://localhost:3000` and serves UI at `http://localhost:3003`.

- **Prod/statically built** (UI served by admin backend):
  1. `make build-web`
  2. `go run ./cmd/admin` (serves the static export from `web/admin/out`).

> The UI fetches the API from `NEXT_PUBLIC_API_BASE`; in static prod, it’s a same‑origin (admin server). In dev, it points to `http://localhost:3000` while the UI runs on `http://localhost:3003`.

### 5.5 Minimal Env Vars

| Var | Example | Notes |
|-----|---------|-------|
| `ADMIN_TOKEN` | `devtoken` | For admin Bearer endpoints. |

@TODO: add more env vars.

---

## 6) API Highlights

- `POST /api/chains` — register/update a chain (idempotent); header `Authorization: Bearer $ADMIN_TOKEN`  
  ```json
  {"chain_id":"canopy-mainnet","chain_name":"Canopy Mainnet","rpc_endpoints":["https://rpc.node1.canopy.us.nodefleet.net"]}
  ```

- `GET /api/chains` — list chains (uses `FINAL` internally; shows `paused/deleted` flags).

- `PATCH /api/chains/status` — bulk pause/resume/delete (and can update RPC endpoints). Example body:
  ```json
  [
    {"chain_id":"local","paused":true},
    {"chain_id":"canopy-mainnet","deleted":true},
    {"chain_id":"dev","rpc_endpoints":["http://127.0.0.1:50002"]}
  ]
  ```

- `GET /api/chains/{id}/progress` — `{ "last": <uint> }` last indexed height (with aggregate fallback).

- `GET /api/chains/status?ids=a,b,c` — returns an object keyed by chain id with `last_indexed` and `head`:
  ```json
  {"local":{"last_indexed":2973,"head":2979}}
  ```

---

## 7) Docker Images

Five Dockerfiles exist: `Dockerfile.admin`, `Dockerfile.indexer`, `Dockerfile.controller`, `Dockerfile.query`, `Dockerfile.reporter`.

Build all (no push):

```bash
make docker-all              # builds canopyx/*:latest
# or tag:
make docker-admin TAG=v0.1.0
```

---

## 10) Roadmap Hints

- Add more entities to the indexer.
- Add the remaining services to Tilt as `local_resource` with file deps for hot reload.
- Wire the **controller** to your cluster (namespace, image/tag, env), and scale per chain.

---

## 11) Handy Commands

```bash
# Spin up infra
tilt up

# Run admin (backend, serves static in prod)
go run ./cmd/admin

# Run indexer to handle `canopy-mainnet` task queue
CHAIN_ID=canopy-mainnet go run ./cmd/indexer

# Run Next dev (hot reload, API proxied to :3000)
make dev-web

# Register a chain via Makefile helper
make register-chain ADMIN_TOKEN=devtoken API=http://localhost:3000
```
