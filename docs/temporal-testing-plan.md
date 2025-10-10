# Temporal Testing & DB Abstraction Plan

## Goals
- Enable unit-style tests for Temporal workflows and activities without hitting Temporal Cloud or ClickHouse.
- Introduce interfaces around database access so activity contexts accept mocks in tests.
- Provide a repeatable test harness developers can expand as new workflows land.

## Architecture Adjustments
- Define lightweight interfaces in `pkg/indexer/activity` (or a new `pkg/indexer/storage`) for the admin DB (`GetChain`, `RecordIndexed`, `ListChain`, `EnsureChainsDbs`) and per-chain DB operations required by activities.
- Refactor activity contexts to depend on those interfaces instead of concrete `*db.AdminDB` / `*db.ChainDB` structs.
- Provide default implementations in `pkg/db` that satisfy the interfaces; expose constructor helpers for production wiring.
- Mirror the pattern for reporter activities (`ListChain`, per-chain `ExecContext` calls) so both services share the abstractions.

## Testing Strategy
- Use `go.temporal.io/sdk/workflowtest` to stand up deterministic workflow environments. Register real activities with mocks for downstream dependencies.
- Cover the indexer workflow by asserting activity scheduling order, retry policy propagation, and state passed between activities via mocked return values.
- Test individual activities with `workflowtest.NewTestActivityEnvironment`; inject mock DB and RPC clients to simulate success, transient failures, and error paths.
- Build table-driven tests that verify serialization of errors (e.g., Temporal application errors when the chain DB is unavailable) and correct metrics (number of transactions, recorded heights).
- For reporter workflows, simulate chains with different pause states, validating that only eligible chains trigger downstream work.

## Implementation Steps
1. Sketch interface contracts and gather consensus on method names/signatures (indexer admin store, chain store, reports store, RPC client abstraction if needed).
2. Refactor `activity.Context` structs to accept the interfaces; add constructors that wire real `db` clients and memoized chain DB map.
3. Update activity functions to use interface methods, adding any helper wrappers (e.g., `ChainWriter.InsertBlock`, `ChainWriter.InsertTransactions`) to reduce direct `*ch.DB` usage.
4. Add Temporal workflow tests under `pkg/indexer/workflow` and `pkg/reporter/workflow`, using mocks/stubs (`testify/mock` or handcrafted fakes) that satisfy the new interfaces.
5. Add activity unit tests under `pkg/indexer/activity` and `pkg/reporter/activity`, covering success, retryable errors, and fatal errors.
6. Document the test harness in `AGENTS.md` (or CONTRIBUTING) so contributors run `go test ./...` with new packages.

## Decisions
- **DB interfaces**: centralize database actions under `pkg/db`, exposing interfaces that services embed. This keeps all ClickHouse touches in one package while giving tests a single mocking surface.
- **RPC client**: wrap `rpc.NewHTTPWithOpts` behind an interface (e.g., `BlockFetcher`, `TxFetcher`) so activities can substitute fakes in tests.
