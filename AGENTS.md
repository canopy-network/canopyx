# Repository Guidelines

## Project Structure & Module Organization
- Services: `cmd/<service>` entries with orchestration in `app/<service>`; update those pairs together.
- Shared Go libraries live in `pkg/` (database, Temporal, RPC, logging, utils); avoid duplicating helpers elsewhere.
- Web admin code sits in `web/admin/` with static exports under `web/admin/out`. Infra assets stay in the root `Makefile`, Dockerfiles, `Tiltfile`, and `deploy/`.
- Indexer binaries now poll per-chain queues: `index:<chainID>` for block workflows and `admin:<chainID>` for head/gap maintenance tasks.

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
