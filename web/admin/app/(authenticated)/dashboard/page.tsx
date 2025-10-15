'use client'

import { useCallback, useEffect, useState, useMemo } from 'react'
import Link from 'next/link'
import { apiFetch } from '../../lib/api'
import { useToast } from '../../components/ToastProvider'

// Updated types to match new API response
type QueueStatus = {
  pending_workflow: number
  pending_activity: number
  backlog_age_secs: number
  pollers: number
}

type HealthInfo = {
  status: string // unknown, healthy, degraded, warning, critical, unreachable, failed
  message: string
  updated_at: string
}

type ReindexEntry = {
  height: number
  status: string
  requested_by: string
  requested_at: string
}

type ChainStatus = {
  chain_id: string
  chain_name: string
  image: string
  notes?: string
  paused: boolean
  deleted: boolean
  min_replicas: number
  max_replicas: number
  last_indexed: number
  head: number
  queue: QueueStatus // Deprecated
  ops_queue: QueueStatus // NEW: Ops queue
  indexer_queue: QueueStatus // NEW: Indexer queue
  reindex_history?: ReindexEntry[]
  health: HealthInfo
  rpc_health: HealthInfo
  queue_health: HealthInfo
  deployment_health: HealthInfo
}

type ChainStatusMap = Record<string, ChainStatus>

type SortField = 'name' | 'last_indexed' | 'ops_queue' | 'indexer_queue' | 'health'
type SortDirection = 'asc' | 'desc'
type FilterMode = 'all' | 'active' | 'paused' | 'issues'

function formatNumber(n: number | undefined) {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return n.toLocaleString()
}

function secondsToFriendly(sec: number) {
  if (!sec || sec <= 0) return '—'
  if (sec < 60) return `${Math.round(sec)}s`
  if (sec < 3600) return `${Math.round(sec / 60)}m`
  return `${Math.round(sec / 3600)}h`
}

function getHealthBadgeClass(status?: string): string {
  if (!status) return 'badge-secondary'
  switch (status.toLowerCase()) {
    case 'healthy':
      return 'badge-success'
    case 'warning':
    case 'degraded':
      return 'badge-warning'
    case 'critical':
    case 'unreachable':
    case 'failed':
      return 'badge-danger'
    case 'unknown':
    default:
      return 'badge-secondary'
  }
}

function getHealthStatusDotClass(status?: string): string {
  if (!status) return 'status-dot-secondary'
  switch (status.toLowerCase()) {
    case 'healthy':
      return 'status-dot-success'
    case 'warning':
    case 'degraded':
      return 'status-dot-warning'
    case 'critical':
    case 'unreachable':
    case 'failed':
      return 'status-dot-danger'
    case 'unknown':
    default:
      return 'status-dot-secondary'
  }
}

function formatHealthStatus(status?: string): string {
  if (!status) return 'Unknown'
  return status.charAt(0).toUpperCase() + status.slice(1).toLowerCase()
}

function isChainHealthy(chain: ChainStatus): boolean {
  const statuses = [
    chain.health?.status,
    chain.rpc_health?.status,
    chain.queue_health?.status,
    chain.deployment_health?.status,
  ]
  return statuses.every((s) => s === 'healthy' || s === 'unknown')
}

export default function DashboardPage() {
  const [status, setStatus] = useState<ChainStatusMap>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')
  const [expandedRow, setExpandedRow] = useState<string | null>(null)
  const [sortField, setSortField] = useState<SortField>('name')
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc')
  const [filterMode, setFilterMode] = useState<FilterMode>('all')
  const [currentPage, setCurrentPage] = useState(1)
  const { notify } = useToast()

  const ITEMS_PER_PAGE = 50

  const loadStatus = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await apiFetch('/api/chains/status')
      if (!res.ok) throw new Error('Failed to load status')
      const data: ChainStatusMap = await res.json()
      // Use functional update to preserve stability and prevent mixing
      setStatus((prev) => {
        // Merge new data with previous, preserving chain order
        const merged = { ...prev }
        Object.keys(data).forEach((chainId) => {
          merged[chainId] = data[chainId]
        })
        return merged
      })
    } catch (err) {
      console.error(err)
      setError('Unable to load dashboard data')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadStatus()
    const interval = setInterval(loadStatus, 30_000) // 30 seconds
    return () => clearInterval(interval)
  }, [loadStatus])

  // Compute metrics
  const chains = useMemo(() => Object.values(status), [status])
  const totalChains = chains.length
  const activeChains = chains.filter((c) => !c.paused && !c.deleted).length
  const pausedChains = chains.filter((c) => c.paused).length

  const avgProgress = useMemo(() => {
    const activeNonDeleted = chains.filter((c) => !c.deleted && !c.paused)
    if (activeNonDeleted.length === 0) return 0
    const totalProgress = activeNonDeleted.reduce((sum, c) => {
      if (!c.head || c.head === 0) return sum
      return sum + (c.last_indexed / c.head) * 100
    }, 0)
    return Math.round(totalProgress / activeNonDeleted.length)
  }, [chains])

  const chainsWithIssues = chains.filter((c) => !isChainHealthy(c)).length

  const totalOpsQueue = chains.reduce((sum, c) => sum + (c.ops_queue?.pending_workflow || 0), 0)
  const totalIndexerQueue = chains.reduce(
    (sum, c) => sum + (c.indexer_queue?.pending_workflow || 0),
    0
  )

  // Filter chains
  const filteredChains = useMemo(() => {
    let filtered = chains
    switch (filterMode) {
      case 'active':
        filtered = chains.filter((c) => !c.paused && !c.deleted)
        break
      case 'paused':
        filtered = chains.filter((c) => c.paused)
        break
      case 'issues':
        filtered = chains.filter((c) => !isChainHealthy(c))
        break
      default:
        filtered = chains
    }
    return filtered
  }, [chains, filterMode])

  // Sort chains
  const sortedChains = useMemo(() => {
    const sorted = [...filteredChains]
    sorted.sort((a, b) => {
      let aVal: any, bVal: any
      switch (sortField) {
        case 'name':
          aVal = a.chain_name || a.chain_id
          bVal = b.chain_name || b.chain_id
          break
        case 'last_indexed':
          aVal = a.last_indexed
          bVal = b.last_indexed
          break
        case 'ops_queue':
          aVal = a.ops_queue?.pending_workflow || 0
          bVal = b.ops_queue?.pending_workflow || 0
          break
        case 'indexer_queue':
          aVal = a.indexer_queue?.pending_workflow || 0
          bVal = b.indexer_queue?.pending_workflow || 0
          break
        case 'health':
          // Sort by worst health status
          const healthPriority = (status: string) => {
            switch (status?.toLowerCase()) {
              case 'failed':
              case 'critical':
                return 5
              case 'unreachable':
                return 4
              case 'warning':
              case 'degraded':
                return 3
              case 'unknown':
                return 2
              case 'healthy':
                return 1
              default:
                return 0
            }
          }
          aVal = healthPriority(a.health?.status)
          bVal = healthPriority(b.health?.status)
          break
        default:
          aVal = a.chain_name
          bVal = b.chain_name
      }

      if (typeof aVal === 'string' && typeof bVal === 'string') {
        return sortDirection === 'asc'
          ? aVal.localeCompare(bVal)
          : bVal.localeCompare(aVal)
      }
      return sortDirection === 'asc' ? aVal - bVal : bVal - aVal
    })
    return sorted
  }, [filteredChains, sortField, sortDirection])

  // Paginate
  const totalPages = Math.ceil(sortedChains.length / ITEMS_PER_PAGE)
  const paginatedChains = useMemo(() => {
    const start = (currentPage - 1) * ITEMS_PER_PAGE
    return sortedChains.slice(start, start + ITEMS_PER_PAGE)
  }, [sortedChains, currentPage])

  // Reset to page 1 when filter/sort changes
  useEffect(() => {
    setCurrentPage(1)
  }, [filterMode, sortField, sortDirection])

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDirection('asc')
    }
  }

  const handleHeadScan = async (chain: ChainStatus, e: React.MouseEvent) => {
    e.stopPropagation()
    try {
      const res = await apiFetch(`/api/chains/${chain.chain_id}/headscan`, { method: 'POST' })
      if (!res.ok) throw new Error('Failed to trigger head scan')
      notify('Head scan triggered')
      await loadStatus()
    } catch (err) {
      notify('Failed to trigger head scan', 'error')
    }
  }

  const handleGapScan = async (chain: ChainStatus, e: React.MouseEvent) => {
    e.stopPropagation()
    try {
      const res = await apiFetch(`/api/chains/${chain.chain_id}/gapscan`, { method: 'POST' })
      if (!res.ok) throw new Error('Failed to trigger gap scan')
      notify('Gap scan triggered')
      await loadStatus()
    } catch (err) {
      notify('Failed to trigger gap scan', 'error')
    }
  }

  const handleTogglePause = async (chain: ChainStatus, e: React.MouseEvent) => {
    e.stopPropagation()
    const nextPaused = chain.paused ? 0 : 1
    try {
      await apiFetch('/api/chains/status', {
        method: 'PATCH',
        body: JSON.stringify([{ chain_id: chain.chain_id, paused: nextPaused }]),
      })
      notify(`Chain ${nextPaused ? 'paused' : 'resumed'}`)
      await loadStatus()
    } catch (err) {
      notify('Failed to update pause state', 'error')
    }
  }

  if (loading && chains.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="flex items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
          <p className="text-slate-400">Loading dashboard...</p>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-xl border border-rose-500/50 bg-rose-500/10 p-6 text-rose-200">
        {error}
      </div>
    )
  }

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Dashboard</h1>
          <p className="mt-2 text-slate-400">
            Monitor your blockchain indexer infrastructure
          </p>
        </div>
        <button onClick={loadStatus} className="btn-secondary" disabled={loading}>
          <svg
            className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
            />
          </svg>
          Refresh
        </button>
      </div>

      {/* Stats Grid - 6 cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <div className="card">
          <p className="text-xs font-medium text-slate-400">Total Chains</p>
          <p className="mt-2 text-2xl font-bold text-white">{totalChains}</p>
        </div>

        <div className="card">
          <p className="text-xs font-medium text-slate-400">Active Chains</p>
          <p className="mt-2 text-2xl font-bold text-emerald-400">{activeChains}</p>
        </div>

        <div className="card">
          <p className="text-xs font-medium text-slate-400">Avg Progress</p>
          <p className="mt-2 text-2xl font-bold text-indigo-400">{avgProgress}%</p>
        </div>

        <div className="card">
          <p className="text-xs font-medium text-slate-400">With Issues</p>
          <p className="mt-2 text-2xl font-bold text-rose-400">{chainsWithIssues}</p>
        </div>

        <div className="card">
          <p className="text-xs font-medium text-slate-400">Ops Queue</p>
          <p className="mt-2 text-2xl font-bold text-amber-400">{formatNumber(totalOpsQueue)}</p>
        </div>

        <div className="card">
          <p className="text-xs font-medium text-slate-400">Indexer Queue</p>
          <p className="mt-2 text-2xl font-bold text-purple-400">
            {formatNumber(totalIndexerQueue)}
          </p>
        </div>
      </div>

      {/* Filters and Sort */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex gap-2">
          <button
            onClick={() => setFilterMode('all')}
            className={filterMode === 'all' ? 'btn text-xs' : 'btn-secondary text-xs'}
          >
            All ({chains.length})
          </button>
          <button
            onClick={() => setFilterMode('active')}
            className={filterMode === 'active' ? 'btn text-xs' : 'btn-secondary text-xs'}
          >
            Active ({chains.filter((c) => !c.paused && !c.deleted).length})
          </button>
          <button
            onClick={() => setFilterMode('paused')}
            className={filterMode === 'paused' ? 'btn text-xs' : 'btn-secondary text-xs'}
          >
            Paused ({pausedChains})
          </button>
          <button
            onClick={() => setFilterMode('issues')}
            className={filterMode === 'issues' ? 'btn text-xs' : 'btn-secondary text-xs'}
          >
            Has Issues ({chainsWithIssues})
          </button>
        </div>

        <div className="text-sm text-slate-400">
          Showing {paginatedChains.length} of {filteredChains.length} chains
        </div>
      </div>

      {/* Chain Table */}
      {chains.length === 0 ? (
        <div className="card">
          <div className="flex flex-col items-center justify-center py-12">
            <svg
              className="h-16 w-16 text-slate-600"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"
              />
            </svg>
            <h3 className="mt-4 text-lg font-semibold text-slate-300">No chains configured</h3>
            <p className="mt-2 text-sm text-slate-500">Get started by adding your first chain</p>
            <Link href="/chains" className="btn mt-6">
              Add Chain
            </Link>
          </div>
        </div>
      ) : paginatedChains.length === 0 ? (
        <div className="card">
          <div className="flex flex-col items-center justify-center py-12">
            <p className="text-slate-400">No chains match the current filter</p>
          </div>
        </div>
      ) : (
        <div className="card overflow-hidden p-0">
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th className="w-12"></th>
                  <th>
                    <button
                      onClick={() => handleSort('health')}
                      className="flex items-center gap-1 hover:text-white"
                    >
                      Status
                      {sortField === 'health' && (
                        <svg
                          className={`h-3 w-3 transition-transform ${
                            sortDirection === 'desc' ? 'rotate-180' : ''
                          }`}
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M5 15l7-7 7 7"
                          />
                        </svg>
                      )}
                    </button>
                  </th>
                  <th>
                    <button
                      onClick={() => handleSort('name')}
                      className="flex items-center gap-1 hover:text-white"
                    >
                      Chain Name
                      {sortField === 'name' && (
                        <svg
                          className={`h-3 w-3 transition-transform ${
                            sortDirection === 'desc' ? 'rotate-180' : ''
                          }`}
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M5 15l7-7 7 7"
                          />
                        </svg>
                      )}
                    </button>
                  </th>
                  <th>
                    <button
                      onClick={() => handleSort('last_indexed')}
                      className="flex items-center gap-1 hover:text-white"
                    >
                      Progress
                      {sortField === 'last_indexed' && (
                        <svg
                          className={`h-3 w-3 transition-transform ${
                            sortDirection === 'desc' ? 'rotate-180' : ''
                          }`}
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M5 15l7-7 7 7"
                          />
                        </svg>
                      )}
                    </button>
                  </th>
                  <th>
                    <button
                      onClick={() => handleSort('ops_queue')}
                      className="flex items-center gap-1 hover:text-white"
                    >
                      Ops Queue
                      {sortField === 'ops_queue' && (
                        <svg
                          className={`h-3 w-3 transition-transform ${
                            sortDirection === 'desc' ? 'rotate-180' : ''
                          }`}
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M5 15l7-7 7 7"
                          />
                        </svg>
                      )}
                    </button>
                  </th>
                  <th>
                    <button
                      onClick={() => handleSort('indexer_queue')}
                      className="flex items-center gap-1 hover:text-white"
                    >
                      Indexer Queue
                      {sortField === 'indexer_queue' && (
                        <svg
                          className={`h-3 w-3 transition-transform ${
                            sortDirection === 'desc' ? 'rotate-180' : ''
                          }`}
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M5 15l7-7 7 7"
                          />
                        </svg>
                      )}
                    </button>
                  </th>
                  <th>RPC Health</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {paginatedChains.map((chain) => {
                  const isExpanded = expandedRow === chain.chain_id
                  const progress = chain.head > 0 ? (chain.last_indexed / chain.head) * 100 : 0

                  return (
                    <>
                      <tr
                        key={chain.chain_id}
                        onClick={() =>
                          setExpandedRow(isExpanded ? null : chain.chain_id)
                        }
                        className="cursor-pointer"
                      >
                        <td>
                          <svg
                            className={`h-4 w-4 text-slate-400 transition-transform ${
                              isExpanded ? 'rotate-90' : ''
                            }`}
                            fill="none"
                            viewBox="0 0 24 24"
                            stroke="currentColor"
                          >
                            <path
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              strokeWidth={2}
                              d="M9 5l7 7-7 7"
                            />
                          </svg>
                        </td>
                        <td>
                          <span className={getHealthStatusDotClass(chain.health?.status)}></span>
                        </td>
                        <td>
                          <div>
                            <div className="font-medium text-white">
                              {chain.chain_name || chain.chain_id}
                            </div>
                            <div className="text-xs text-slate-500">{chain.chain_id}</div>
                          </div>
                        </td>
                        <td>
                          <div className="flex items-center gap-2">
                            <div className="h-2 w-24 overflow-hidden rounded-full bg-slate-800">
                              <div
                                className={`h-full ${
                                  progress >= 99
                                    ? 'bg-emerald-500'
                                    : progress >= 90
                                    ? 'bg-indigo-500'
                                    : progress >= 50
                                    ? 'bg-amber-500'
                                    : 'bg-rose-500'
                                }`}
                                style={{ width: `${Math.min(progress, 100)}%` }}
                              ></div>
                            </div>
                            <span className="font-mono text-xs text-slate-400">
                              {formatNumber(chain.last_indexed)} / {formatNumber(chain.head)}
                            </span>
                          </div>
                        </td>
                        <td className="font-mono text-sm">
                          {formatNumber(chain.ops_queue?.pending_workflow || 0)}
                        </td>
                        <td className="font-mono text-sm">
                          {formatNumber(chain.indexer_queue?.pending_workflow || 0)}
                        </td>
                        <td>
                          <span className={getHealthBadgeClass(chain.rpc_health?.status)}>
                            {formatHealthStatus(chain.rpc_health?.status)}
                          </span>
                        </td>
                        <td onClick={(e) => e.stopPropagation()}>
                          <div className="flex items-center justify-end gap-1">
                            <button
                              onClick={(e) => handleHeadScan(chain, e)}
                              className="btn-ghost p-1.5 text-xs"
                              title="Head Scan"
                            >
                              <svg
                                className="h-4 w-4"
                                fill="none"
                                viewBox="0 0 24 24"
                                stroke="currentColor"
                              >
                                <path
                                  strokeLinecap="round"
                                  strokeLinejoin="round"
                                  strokeWidth={2}
                                  d="M13 10V3L4 14h7v7l9-11h-7z"
                                />
                              </svg>
                            </button>
                            <button
                              onClick={(e) => handleGapScan(chain, e)}
                              className="btn-ghost p-1.5 text-xs"
                              title="Gap Scan"
                            >
                              <svg
                                className="h-4 w-4"
                                fill="none"
                                viewBox="0 0 24 24"
                                stroke="currentColor"
                              >
                                <path
                                  strokeLinecap="round"
                                  strokeLinejoin="round"
                                  strokeWidth={2}
                                  d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
                                />
                              </svg>
                            </button>
                            <button
                              onClick={(e) => handleTogglePause(chain, e)}
                              className="btn-ghost p-1.5 text-xs"
                              title={chain.paused ? 'Resume' : 'Pause'}
                            >
                              {chain.paused ? (
                                <svg
                                  className="h-4 w-4"
                                  fill="currentColor"
                                  viewBox="0 0 20 20"
                                >
                                  <path d="M6.3 2.841A1.5 1.5 0 004 4.11V15.89a1.5 1.5 0 002.3 1.269l9.344-5.89a1.5 1.5 0 000-2.538L6.3 2.84z" />
                                </svg>
                              ) : (
                                <svg
                                  className="h-4 w-4"
                                  fill="currentColor"
                                  viewBox="0 0 20 20"
                                >
                                  <path d="M5.75 3a.75.75 0 00-.75.75v12.5c0 .414.336.75.75.75h1.5a.75.75 0 00.75-.75V3.75A.75.75 0 007.25 3h-1.5zM12.75 3a.75.75 0 00-.75.75v12.5c0 .414.336.75.75.75h1.5a.75.75 0 00.75-.75V3.75a.75.75 0 00-.75-.75h-1.5z" />
                                </svg>
                              )}
                            </button>
                            <Link
                              href={`/chains/${chain.chain_id}`}
                              className="btn-ghost p-1.5 text-xs"
                              title="View Details"
                            >
                              <svg
                                className="h-4 w-4"
                                fill="none"
                                viewBox="0 0 24 24"
                                stroke="currentColor"
                              >
                                <path
                                  strokeLinecap="round"
                                  strokeLinejoin="round"
                                  strokeWidth={2}
                                  d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
                                />
                              </svg>
                            </Link>
                          </div>
                        </td>
                      </tr>
                      {isExpanded && (
                        <tr className="bg-slate-900/50">
                          <td colSpan={8}>
                            <div className="grid gap-4 p-4 md:grid-cols-2 lg:grid-cols-3">
                              {/* Health Status Breakdown */}
                              <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-3">
                                <div className="mb-3 text-sm font-semibold text-white">
                                  Health Status
                                </div>
                                <div className="space-y-2 text-xs">
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Overall</span>
                                    <span className={getHealthBadgeClass(chain.health?.status)}>
                                      {formatHealthStatus(chain.health?.status)}
                                    </span>
                                  </div>
                                  {chain.health?.message && (
                                    <p className="text-slate-500">{chain.health.message}</p>
                                  )}
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">RPC</span>
                                    <span className={getHealthBadgeClass(chain.rpc_health?.status)}>
                                      {formatHealthStatus(chain.rpc_health?.status)}
                                    </span>
                                  </div>
                                  {chain.rpc_health?.message && (
                                    <p className="text-slate-500">{chain.rpc_health.message}</p>
                                  )}
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Queue</span>
                                    <span
                                      className={getHealthBadgeClass(chain.queue_health?.status)}
                                    >
                                      {formatHealthStatus(chain.queue_health?.status)}
                                    </span>
                                  </div>
                                  {chain.queue_health?.message && (
                                    <p className="text-slate-500">{chain.queue_health.message}</p>
                                  )}
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Deployment</span>
                                    <span
                                      className={getHealthBadgeClass(
                                        chain.deployment_health?.status
                                      )}
                                    >
                                      {formatHealthStatus(chain.deployment_health?.status)}
                                    </span>
                                  </div>
                                  {chain.deployment_health?.message && (
                                    <p className="text-slate-500">
                                      {chain.deployment_health.message}
                                    </p>
                                  )}
                                </div>
                              </div>

                              {/* Ops Queue Details */}
                              <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-3">
                                <div className="mb-3 text-sm font-semibold text-white">
                                  Ops Queue
                                </div>
                                <div className="space-y-2 text-xs">
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Pending Workflows</span>
                                    <span className="font-mono text-slate-300">
                                      {formatNumber(chain.ops_queue?.pending_workflow || 0)}
                                    </span>
                                  </div>
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Pending Activities</span>
                                    <span className="font-mono text-slate-300">
                                      {formatNumber(chain.ops_queue?.pending_activity || 0)}
                                    </span>
                                  </div>
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Backlog Age</span>
                                    <span className="font-mono text-slate-300">
                                      {secondsToFriendly(chain.ops_queue?.backlog_age_secs || 0)}
                                    </span>
                                  </div>
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Pollers</span>
                                    <span className="font-mono text-slate-300">
                                      {chain.ops_queue?.pollers || 0}
                                    </span>
                                  </div>
                                </div>
                              </div>

                              {/* Indexer Queue Details */}
                              <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-3">
                                <div className="mb-3 text-sm font-semibold text-white">
                                  Indexer Queue
                                </div>
                                <div className="space-y-2 text-xs">
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Pending Workflows</span>
                                    <span className="font-mono text-slate-300">
                                      {formatNumber(chain.indexer_queue?.pending_workflow || 0)}
                                    </span>
                                  </div>
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Pending Activities</span>
                                    <span className="font-mono text-slate-300">
                                      {formatNumber(chain.indexer_queue?.pending_activity || 0)}
                                    </span>
                                  </div>
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Backlog Age</span>
                                    <span className="font-mono text-slate-300">
                                      {secondsToFriendly(
                                        chain.indexer_queue?.backlog_age_secs || 0
                                      )}
                                    </span>
                                  </div>
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Pollers</span>
                                    <span className="font-mono text-slate-300">
                                      {chain.indexer_queue?.pollers || 0}
                                    </span>
                                  </div>
                                </div>
                              </div>

                              {/* Chain Info */}
                              <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-3">
                                <div className="mb-3 text-sm font-semibold text-white">
                                  Chain Info
                                </div>
                                <div className="space-y-2 text-xs">
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Min Replicas</span>
                                    <span className="font-mono text-slate-300">
                                      {chain.min_replicas}
                                    </span>
                                  </div>
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Max Replicas</span>
                                    <span className="font-mono text-slate-300">
                                      {chain.max_replicas}
                                    </span>
                                  </div>
                                  <div className="flex items-center justify-between">
                                    <span className="text-slate-400">Status</span>
                                    <span className="text-slate-300">
                                      {chain.paused ? 'Paused' : chain.deleted ? 'Deleted' : 'Active'}
                                    </span>
                                  </div>
                                  {chain.image && (
                                    <div>
                                      <span className="text-slate-400">Image:</span>
                                      <p className="mt-1 break-all font-mono text-[10px] text-slate-500">
                                        {chain.image}
                                      </p>
                                    </div>
                                  )}
                                  {chain.notes && (
                                    <div>
                                      <span className="text-slate-400">Notes:</span>
                                      <p className="mt-1 text-slate-300">{chain.notes}</p>
                                    </div>
                                  )}
                                </div>
                              </div>
                            </div>
                          </td>
                        </tr>
                      )}
                    </>
                  )
                })}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between border-t border-slate-800 px-6 py-4">
              <div className="text-sm text-slate-400">
                Page {currentPage} of {totalPages}
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                  disabled={currentPage === 1}
                  className="btn-secondary text-xs"
                >
                  Previous
                </button>
                <button
                  onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                  disabled={currentPage === totalPages}
                  className="btn-secondary text-xs"
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}