# Admin UI Improvement Plan

**Date:** 2025-10-14
**Status:** Planning Phase - Updated with Single-Page Approach
**Priority:** High

## Executive Summary

This document outlines comprehensive improvements to the CanopyX Admin UI using a **single-page dashboard approach**. The design consolidates the dashboard and chain list into one unified view with top-level stats, a paginated expandable table, and drill-down detail pages for deep exploration.

## Current Issues

### 1. Chain Navigation from Dashboard (Critical)
**Problem:** Chains displayed on the dashboard are not clickable/navigable to chain detail view.

**Current State:**
- Dashboard shows chain overview table (lines 286-351 in `dashboard/page.tsx`)
- Chain rows display status, indexed height, head, backlog, and replicas
- No link or click handler to navigate to individual chain details

**Impact:** Users cannot drill down from dashboard overview to specific chain details, breaking expected UX flow.

---

### 2. Chain List Scalability (High Priority)
**Problem:** Chain list items are too large for managing hundreds of chains.

**Current State:**
- Chains page uses large card layout (2-column grid on desktop)
- Each card is ~400-500px tall with extensive details:
  - Health status breakdown (RPC, Queue, Deployment)
  - Queue metrics (Workflow, Activity, Pollers, Age)
  - Multiple action buttons
- Not feasible for viewing/managing 100+ chains

**Impact:** Poor scalability, excessive scrolling, difficulty scanning many chains at once.

---

### 3. Data Mixing on Polling (Critical Bug)
**Problem:** When poll query refreshes and data changes, UI shows mixed/incorrect data.

**Current State:**
- Two separate polling mechanisms:
  - `loadChains()` fetches chain configurations
  - `loadStatus()` fetches runtime status
- Status updates every 15 seconds independently
- React state updates may cause race conditions
- Lines 183-192 in `chains/page.tsx`:
  ```tsx
  useEffect(() => {
    if (!chains.length) return
    loadStatus(chains.map((c) => c.chain_id))
    const interval = setInterval(() => loadStatus(chains.map((c) => c.chain_id)), 15_000)
    return () => clearInterval(interval)
  }, [chains, loadStatus])
  ```

**Root Cause:** Status update depends on `chains` array, which can change independently, causing mismatched data rendering.

**Impact:** Users see incorrect metrics, health statuses, or data attributed to wrong chains.

---

### 4. Missing Delete Chain Action (High Priority)
**Problem:** No way to delete a chain from the UI.

**Current State:**
- Can create chains (CreateChainDialog)
- Can edit chains (EditChainDialog) - only image, replicas, notes
- Can pause/resume chains
- **Cannot delete chains**

**Impact:** Administrative gap - must use API/database directly to remove chains.

---

### 5. No RPC Endpoint Editing (High Priority)
**Problem:** Cannot edit RPC endpoints after chain creation.

**Current State:**
- RPC endpoints can be set during chain creation (CreateChainDialog)
- EditChainDialog only allows editing: image, min_replicas, max_replicas, notes
- RPC endpoints field (`rpc_endpoints: string[]`) is immutable post-creation

**Impact:** If RPC endpoints change or fail, must recreate entire chain configuration.

---

### 6. Incorrect Temporal Metrics (Critical Bug)
**Problem:** Workflow/activity metrics show incorrect values (0 or 2) instead of actual counts (100+).

**Current State:**
- Dashboard and chains page display queue metrics from `/api/chains/status` endpoint
- Queue metrics show:
  - `pending_workflow`
  - `pending_activity`
  - `backlog_age_secs`
  - `pollers`

**Suspected Issues:**
- Queue metrics may only show **one queue** (ops queue) instead of both (ops + indexer)
- Backend may be querying wrong queue names
- Temporal queue naming mismatch (queue name format: `canopy-{chainId}-ops` vs `canopy-{chainId}-indexer`)

**Impact:** Misleading operational metrics, cannot trust queue health indicators.

---

### 7. Missing Chain LIST vs DETAIL Separation (High Priority)
**Problem:** Current chains page tries to be both list and detail view, causing UX confusion.

**Current Requirements:**
- **Chain LIST view:** Compact table/grid showing many chains with key metrics
- **Chain DETAIL view:** Dedicated page for single chain with comprehensive information:
  - Both queue types (ops and indexer) with separate metrics
  - Full health status breakdown
  - Chain database explorer (see Issue #8)
  - Complete configuration
  - Action panel (pause, scan, reindex, delete, edit)

**Current State:**
- Single "chains" page shows all chains in large cards
- Tries to show detailed info in each card
- No dedicated detail view route

**Impact:** Cannot scale to hundreds of chains, cannot show comprehensive detail for one chain.

---

### 8. Chain DB Explorer (New Feature)
**Problem:** No way to explore indexed blockchain data within the admin UI.

**Proposed Feature:**
- Add "Explorer" tab/section to chain detail page
- Show all tables in the chain's ClickHouse database:
  - `blocks` table
  - `txs` table
  - `txs_raw` table
- For each table:
  - Display schema/columns
  - Paginated data browsing (cursor-based)
  - Basic filtering/search
  - Record count
- Use ClickHouse HTTP interface directly or new Go API endpoints

**Technical Considerations:**
- ClickHouse HTTP API supports direct queries
- Security: must validate/sanitize queries, limit to read-only
- Performance: implement pagination limits, query timeouts
- Optional: Build reusable query engine (inside of query app) in Go for better control

**Impact:** Provides valuable operational insight into indexed data without external tools.

---

## Proposed Architecture (UPDATED - Single Page Approach)

### Page Structure

```
/dashboard                               - UNIFIED view: Stats + Chain List + Actions
  ├─ Top Stats (4-6 metric cards)
  ├─ Paginated Chain Table (expandable rows)
  │  └─ Row actions: Head Scan, Gap Scan, Pause/Resume, Detail
  └─> Click detail → /chains/[chain_id]

/chains/[chain_id]                       - DETAIL view for deep exploration
  ├─ Overview tab       - Health, status, config, charts
  ├─ Queues tab         - Separate metrics for BOTH ops + indexer queues
  ├─ Explorer tab       - Browse chain database tables (blocks, txs, txs_raw)
  └─ Settings tab       - Edit ALL config (including RPC endpoints), delete chain

(Remove /chains route entirely - dashboard serves as list)
```

### Visual Layout

```
┌─────────────────────────────────────────────────────────────┐
│  DASHBOARD                                                  │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │  Total   │ │  Health  │ │ Chains   │ │  Total   │      │
│  │  Chains  │ │  Status  │ │  Behind  │ │   Gaps   │      │
│  │   125    │ │  🟢 100  │ │    15    │ │   342    │      │
│  │ Active:  │ │  🟡 20   │ │          │ │          │      │
│  │   120    │ │  🔴 5    │ │          │ │          │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
├─────────────────────────────────────────────────────────────┤
│  CHAIN TABLE                            [Refresh] [+ Add]  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │▶ Chain Name    │Status│Indexed/Head│Behind│Actions│   │
│  ├─────────────────────────────────────────────────────┤   │
│  │▶ Pokt Mainnet  │ 🟢  │ 1.2M/1.5M │ 300K │⚡🔍⏸📊│   │
│  │  chain-001     │     │           │      │       │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │▼ Eth Mainnet   │ 🟡  │  18M/18M  │   0  │⚡🔍⏸📊│   │
│  │  chain-002     │     │           │      │       │   │
│  │  └─ Expanded Details ─────────────────────────┐    │   │
│  │     Health: RPC 🟢  Queue 🟡  Deploy 🟢      │    │   │
│  │     Ops Queue: 150 wf, 200 act, 4 pollers    │    │   │
│  │     Indexer Queue: 0 wf, 0 act, 2 pollers    │    │   │
│  │     Replicas: 2/5 │ Notes: Production chain   │    │   │
│  │  ─────────────────────────────────────────────┘    │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │▶ Solana Devnet │ 🔴  │ 500K/600K │ 100K │⚡🔍▶📊│   │
│  │  chain-003     │     │           │      │       │   │
│  └─────────────────────────────────────────────────────┘   │
│  [← Prev]  [1] [2] [3] ... [10]  [Next →]                │
└─────────────────────────────────────────────────────────────┘
```

### Component Breakdown

#### 1. Dashboard (`/dashboard`)
- **Keep:** Summary stats (total chains, backlog, health, avg indexed)
- **Fix:** Make chain rows clickable → navigate to `/chains/[chain_id]`
- **Improve:** Add "View Details" link column

#### 2. Chains List (`/chains`)
- **Replace:** Large cards → Compact table or dense grid
- **Columns:**
  - Chain Name + ID
  - Status badge (paused, healthy, warning, critical)
  - Indexed / Head (compact format)
  - Backlog (combined ops + indexer)
  - Quick Actions (play/pause, refresh)
  - Details link → `/chains/[chain_id]`
- **Features:**
  - Sortable columns
  - Search/filter by name or ID
  - Pagination (50-100 per page)
  - Bulk actions (future: pause multiple, delete multiple)

#### 3. Chain Detail (`/chains/[chain_id]`)

**Layout:** Tabs or side navigation

**Tab 1: Overview**
- Chain name, ID, status, paused state
- Health status breakdown (RPC, Queue, Deployment) with details
- Indexing progress: indexed height, head, gap count
- Configuration summary: image, replicas, RPC endpoints
- Recent reindex history

**Tab 2: Queues**
- **Two separate queue sections:**
  - **Ops Queue (`canopy-{chainId}-ops`)**
    - Pending workflows
    - Pending activities
    - Pollers count
    - Backlog age
    - Task queue health
  - **Indexer Queue (`canopy-{chainId}-indexer`)**
    - Pending workflows
    - Pending activities
    - Pollers count
    - Backlog age
    - Task queue health
- Temporal workflow/activity charts (optional future enhancement)

**Tab 3: Explorer** *(New Feature)*
- Table selector: blocks | txs | txs_raw
- Schema display (column names, types)
- Paginated table view with cursors
- Record count
- Basic filters (height range, hash search)
- Export options (CSV, JSON - future)

**Tab 4: Settings**
- Edit form:
  - Container image
  - RPC endpoints (array input, add/remove)
  - Min/max replicas
  - Notes
- Action buttons:
  - Save changes
  - Pause/Resume
  - Head Scan
  - Gap Scan
  - Reindex (opens dialog)
  - **Delete Chain** (with confirmation)

---

## Implementation Plan

### Phase 1: Critical Bug Fixes (1-2 days)
**Goal:** Fix data integrity and navigation issues

**Tasks:**
1. **Fix chain navigation from dashboard**
   - Add Link wrapper or onClick handler to chain rows in dashboard
   - Create `/chains/[chain_id]` route structure
   - Test navigation flow

2. **Fix data mixing bug**
   - Refactor polling logic to use stable chain IDs
   - Use `useCallback` with proper dependencies
   - Consider using React Query or SWR for better cache management
   - Add loading states during status updates
   - Test rapid chain addition/removal scenarios

3. **Fix Temporal metrics display**
   - Audit backend `/api/chains/status` endpoint
   - Verify both ops and indexer queues are queried
   - Check queue naming conventions match Temporal configuration
   - Update frontend to display both queue types separately
   - Add debug logging for queue metric fetching

**Deliverables:**
- Dashboard chains are clickable
- No data mixing during polling
- Accurate queue metrics from both queues

---

### Phase 2: UI Restructuring (2-3 days)
**Goal:** Implement LIST vs DETAIL separation

**Tasks:**
1. **Create Chain List View** (`/chains`)
   - Replace card grid with compact table
   - Implement sortable columns
   - Add search/filter functionality
   - Implement pagination (server-side or client-side based on chain count)
   - Add quick action buttons (pause, view details)

2. **Create Chain Detail View** (`/chains/[chain_id]`)
   - Set up dynamic route
   - Create tab navigation structure (Overview, Queues, Explorer, Settings)
   - Implement Overview tab (reuse existing card content)
   - Implement Queues tab with dual queue display
   - Implement Settings tab (move edit functionality)

3. **Update Dashboard**
   - Keep current overview stats
   - Update chain table to match new compact list style
   - Add "View Details" links

**Deliverables:**
- Compact, scalable chain list
- Dedicated chain detail pages with tabs
- Consistent navigation experience

---

### Phase 3: Missing Features (2-3 days)
**Goal:** Add delete chain and RPC endpoint editing

**Tasks:**
1. **Add Delete Chain Functionality**
   - Add "Delete Chain" button to Settings tab
   - Create confirmation dialog (require typing chain ID)
   - Implement DELETE `/api/chains/:id` call
   - Handle soft delete vs hard delete (based on backend implementation)
   - Update UI after successful deletion
   - Add error handling for chains with active workflows

2. **Add RPC Endpoint Editing**
   - Update EditChainDialog or Settings tab form
   - Implement array input component (add/remove endpoints)
   - Validate URL formats
   - Update backend PATCH endpoint if needed
   - Test RPC health refresh after update

**Deliverables:**
- Functional delete chain with confirmation
- Editable RPC endpoints with validation

---

### Phase 4: Chain DB Explorer (3-4 days)
**Goal:** Implement blockchain data browsing feature

**Tasks:**

**Backend Work:**
1. **Create Query Endpoints** (optional - can use ClickHouse HTTP directly)
   - `GET /api/chains/:id/db/tables` - List tables
   - `GET /api/chains/:id/db/tables/:table/schema` - Get table schema
   - `GET /api/chains/:id/db/tables/:table/data?cursor=X&limit=50` - Paginated data
   - Implement read-only query execution with:
     - Query validation/sanitization
     - Timeout limits
     - Row limits (max 1000 per request)
     - Cursor-based pagination

2. **Alternative: Direct ClickHouse HTTP**
   - Use ClickHouse HTTP interface from frontend
   - Implement query builder utilities
   - Add authentication/authorization checks
   - More flexible but requires careful security

**Frontend Work:**
1. **Create Explorer Tab**
   - Table selector dropdown (blocks, txs, txs_raw)
   - Schema display component
   - Paginated data table
   - Cursor navigation (Previous, Next)
   - Loading states and error handling

2. **Optional Enhancements:**
   - Column sorting
   - Basic filters (height range, hash search)
   - Row detail expansion
   - Export buttons
   - Query history

**Deliverables:**
- Functional chain database explorer
- Paginated table browsing
- Schema information display

---

### Phase 5: Polish & Optimization (1-2 days)
**Goal:** Improve UX and performance

**Tasks:**
1. **Performance Optimization**
   - Implement query debouncing
   - Add optimistic UI updates
   - Consider virtualization for large chain lists
   - Optimize polling intervals (adaptive based on activity)

2. **UX Improvements**
   - Add keyboard shortcuts (refresh, navigate)
   - Improve loading states and skeletons
   - Add tooltips for metrics
   - Implement toast notifications consistently
   - Add error boundaries

3. **Accessibility**
   - Ensure keyboard navigation works
   - Add ARIA labels
   - Test screen reader compatibility
   - Improve color contrast

4. **Documentation**
   - Update README with UI feature documentation
   - Add inline help/tooltips
   - Create user guide

**Deliverables:**
- Performant, accessible UI
- Comprehensive documentation

---

## Technical Specifications

### API Changes Required

#### New Endpoints
```
GET    /api/chains/:id/db/tables                    - List chain DB tables
GET    /api/chains/:id/db/tables/:table/schema      - Get table schema
GET    /api/chains/:id/db/tables/:table/data        - Get paginated table data
  Query params: cursor, limit, orderBy, filters

DELETE /api/chains/:id                              - Delete chain
  Body: { confirm: chain_id }
```

#### Modified Endpoints
```
PATCH  /api/chains/:id                              - Update chain config
  Add support for: rpc_endpoints array

GET    /api/chains/status                           - Chain status
  Fix: Return BOTH ops and indexer queue metrics
  Response format:
  {
    "chain_id": {
      "ops_queue": { pending_workflow, pending_activity, ... },
      "indexer_queue": { pending_workflow, pending_activity, ... },
      ...
    }
  }
```

### Frontend State Management

**Consider migrating to React Query or SWR for:**
- Automatic refetching
- Cache management
- Optimistic updates
- Request deduplication
- Better error handling

**Example:**
```tsx
const { data: chains } = useQuery('chains', fetchChains)
const { data: status } = useQuery(
  ['chainStatus', chainIds],
  () => fetchStatus(chainIds),
  { refetchInterval: 15000 }
)
```

### Database Explorer Query Security

**Must Implement:**
1. Read-only query execution (no INSERT, UPDATE, DELETE, DROP)
2. Query timeout (5-10 seconds max)
3. Row limit enforcement (max 1000 rows)
4. Allowed table whitelist (blocks, txs, txs_raw only)
5. No arbitrary SQL injection (use parameterized queries)
6. Rate limiting (max 10 queries/minute per user)

---

## Testing Strategy

### Unit Tests
- Component rendering tests (Jest + React Testing Library)
- State management logic tests
- Query builder utilities
- Pagination logic

### Integration Tests
- API endpoint tests
- Query security validation
- Error handling flows

### E2E Tests (Playwright/Cypress)
- Complete user flows:
  - Create chain → View list → Navigate to detail → Edit → Delete
  - Dashboard navigation
  - Explorer data browsing
  - Multi-queue monitoring
- Polling/refresh behavior
- Error states and recovery

### Manual Testing Scenarios
- 100+ chains scalability test
- Rapid polling with changing data
- Delete chain with active workflows
- Invalid RPC endpoint handling
- ClickHouse connection failures

---

## Success Metrics

1. **Navigation:** 100% of chain rows clickable from dashboard
2. **Data Integrity:** 0% data mixing incidents during polling
3. **Queue Metrics:** Both ops and indexer queues display accurate counts
4. **Scalability:** Can view/manage 500+ chains without performance degradation
5. **Feature Completeness:** All CRUD operations available (Create, Read, Update, Delete)
6. **Response Time:** Chain list loads in <2s, detail view in <1s
7. **Explorer Usability:** Can browse 10K+ blocks with smooth pagination

---

## Rollout Plan

### Development Branch Strategy
```
main
  └─ feature/ui-improvements
       ├─ phase-1-bug-fixes
       ├─ phase-2-restructure
       ├─ phase-3-features
       ├─ phase-4-explorer
       └─ phase-5-polish
```

### Deployment Phases
1. **Alpha:** Phase 1-2 (bug fixes + restructure) → Internal testing
2. **Beta:** Phase 1-3 (+ missing features) → Limited rollout
3. **RC:** Phase 1-4 (+ explorer) → Staging environment
4. **Production:** Phase 1-5 (complete) → Full rollout

### Rollback Plan
- Feature flags for new UI sections
- Maintain old `/chains` route as fallback during migration
- Database migrations must be reversible
- API versioning for breaking changes

---

## Open Questions

1. **Chain DB Explorer:**
   - Direct ClickHouse HTTP or Go API endpoints?
   - Should we support custom SQL queries or just table browsing?
   - Export functionality scope?

2. **Delete Chain:**
   - Soft delete (mark deleted) or hard delete (remove from DB)?
   - What happens to indexed data (chain DB)?
   - Should we archive chain configuration?

3. **Performance:**
   - At what scale should we implement backend pagination for chain list?
   - Should explorer use cursor pagination or offset pagination?

4. **Queue Metrics:**
   - Should we add historical queue depth charts?
   - Real-time updates via WebSocket instead of polling?

5. **UI Framework:**
   - Continue with current component library or migrate to shadcn/ui?
   - Implement design system for consistency?

---

## Dependencies

### External Libraries to Consider
- **React Query** or **SWR** - Better data fetching
- **TanStack Table** - Advanced table features
- **Recharts** or **Chart.js** - Queue metrics visualization
- **React Virtual** - Virtualization for large lists
- **Zod** - Runtime validation for forms

### Backend Dependencies
- ClickHouse HTTP interface documentation
- Temporal queue naming conventions
- Chain lifecycle hooks (delete, update)

---

## Timeline Estimate

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1: Bug Fixes | 2 days | Backend queue endpoint fix |
| Phase 2: Restructure | 3 days | None |
| Phase 3: Features | 3 days | Backend delete endpoint |
| Phase 4: Explorer | 4 days | Backend query endpoints or ClickHouse access |
| Phase 5: Polish | 2 days | All phases complete |
| **Total** | **~14 days** | **~2-3 sprint cycles** |

---

## Next Steps

1. **Review & Approval:** Team review of this plan
2. **Backend Coordination:** Confirm API changes with backend team
3. **Design Mockups:** Create UI mockups for new LIST/DETAIL views (optional)
4. **Create Tasks:** Break down phases into individual Jira/GitHub issues
5. **Start Phase 1:** Begin with critical bug fixes

---

## Appendix

### Current File Structure
```
web/admin/app/
├── (authenticated)/
│   ├── dashboard/page.tsx      # Dashboard overview
│   ├── chains/page.tsx         # Current chains list
│   └── layout.tsx              # Authenticated layout
├── components/
│   ├── CreateChainDialog.tsx   # Create chain modal
│   ├── Sidebar.tsx             # Navigation
│   └── ToastProvider.tsx       # Notifications
├── lib/
│   ├── api.ts                  # API fetch wrapper
│   ├── auth-context.tsx        # Auth state
│   └── constants.ts            # Utility constants
└── login/page.tsx              # Login page
```

### Proposed New Structure
```
web/admin/app/
├── (authenticated)/
│   ├── dashboard/page.tsx           # Dashboard overview [UPDATED]
│   ├── chains/
│   │   ├── page.tsx                 # Compact chain LIST [NEW]
│   │   └── [chainId]/
│   │       ├── page.tsx             # Chain detail router [NEW]
│   │       ├── overview/page.tsx    # Overview tab [NEW]
│   │       ├── queues/page.tsx      # Queues tab [NEW]
│   │       ├── explorer/page.tsx    # DB explorer tab [NEW]
│   │       └── settings/page.tsx    # Settings tab [NEW]
│   └── layout.tsx
├── components/
│   ├── chains/
│   │   ├── ChainListTable.tsx       # Compact list component [NEW]
│   │   ├── ChainCard.tsx            # Reusable chain card [NEW]
│   │   ├── QueueMetrics.tsx         # Queue display component [NEW]
│   │   ├── HealthStatusBadge.tsx    # Health badge component [NEW]
│   │   └── DeleteChainDialog.tsx    # Delete confirmation [NEW]
│   ├── explorer/
│   │   ├── TableSelector.tsx        # DB table selector [NEW]
│   │   ├── SchemaDisplay.tsx        # Show table schema [NEW]
│   │   ├── DataTable.tsx            # Paginated data view [NEW]
│   │   └── QueryBuilder.tsx         # Query interface [NEW]
│   ├── CreateChainDialog.tsx
│   ├── EditChainDialog.tsx          # [UPDATED: add RPC endpoints]
│   ├── Sidebar.tsx
│   └── ToastProvider.tsx
└── lib/
    ├── api.ts
    ├── auth-context.tsx
    ├── constants.ts
    └── queries/                     # React Query hooks [NEW]
        ├── useChains.ts
        ├── useChainStatus.ts
        └── useChainExplorer.ts
```

---

**Document Version:** 1.0
**Last Updated:** 2025-10-14
**Author:** Claude Code
**Status:** Awaiting Review