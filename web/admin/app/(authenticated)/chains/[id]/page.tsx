'use client'

import { useCallback, useEffect, useState } from 'react'
import { useParams, useRouter, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import * as Dialog from '@radix-ui/react-dialog'
import { apiFetch } from '../../../lib/api'
import { useToast } from '../../../components/ToastProvider'
import IndexingProgressChart from './IndexingProgressChart'
import { LiveSyncStatus } from '../../../components/LiveSyncStatus'
import { GapRangesDisplay } from '../../../components/GapRangesDisplay'
import { QueueHealthBadge } from '../../../components/QueueHealthBadge'
import { useBlockEvents } from '../../../hooks/useBlockEvents'
import { TransactionTypeBreakdown } from './TransactionTypeBreakdown'
import { TransactionList } from './TransactionList'
import { EventTypeBreakdown } from './EventTypeBreakdown'
import LPSnapshotManagement from './LPSnapshotManagement'

// Types
type QueueStatus = {
  pending_workflow: number
  pending_activity: number
  backlog_age_secs: number
  pollers: number
}

type HealthInfo = {
  status: string
  message: string
  updated_at: string
}

type ReindexEntry = {
  height: number
  status: string
  requested_by: string
  requested_at: string
  workflow_id: string
  run_id: string
}

type ChainConfig = {
  chain_id: string | number // Backend returns number, URL params are string
  chain_name: string
  rpc_endpoints: string[]
  paused: boolean
  deleted: boolean
  image: string
  min_replicas: number
  max_replicas: number
  reindex_min_replicas: number
  reindex_max_replicas: number
  reindex_scale_threshold: number
  notes?: string
  created_at: string
  updated_at: string
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
  ops_queue: QueueStatus
  indexer_queue: QueueStatus
  live_queue_depth: number // Live queue pending tasks
  live_queue_backlog_age: number // Live queue oldest task age in seconds
  historical_queue_depth: number // Historical queue pending tasks
  historical_queue_backlog_age: number // Historical queue oldest task age in seconds
  reindex_history?: ReindexEntry[]
  health: HealthInfo
  rpc_health: HealthInfo
  queue_health: HealthInfo
  deployment_health: HealthInfo
  // live sync and gap tracking
  missing_blocks_count?: number
  gap_ranges_count?: number
  largest_gap_start?: number
  largest_gap_end?: number
  is_live_sync?: boolean
  // event tracking
  event_counts_by_type?: { [key: string]: number }
}

type ReindexPayload = {
  heights?: number[]
  from?: number
  to?: number
}

type PaginatedResponse<T> = {
  data: T[]
  limit: number
  next_cursor: number | null
}

// Transaction and Block Summary types
type BlockSummary = {
  height: number
  height_time: string
  num_txs: number
  tx_counts_by_type: { [key: string]: number }
}

type Transaction = {
  height: number
  tx_hash: string
  message_type: string
  signer: string
  counterparty?: string | null
  amount?: number | null
  fee: number
  time: string
}

// Utility functions
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

function formatTimestamp(timestamp?: string): string {
  if (!timestamp) return 'Never'
  try {
    const date = new Date(timestamp)
    return date.toLocaleString()
  } catch {
    return 'Invalid date'
  }
}

// Parse live replicas from deployment health message
// Message formats: "2/2 replicas ready", "1/2 replicas ready (expected 2)", etc.
function parseLiveReplicas(deploymentHealthMessage?: string): { ready: number; total: number } | null {
  if (!deploymentHealthMessage) return null

  // Match patterns like "2/2 replicas ready" or "1/2 replicas ready"
  const match = deploymentHealthMessage.match(/(\d+)\/(\d+)\s+replicas/)
  if (match) {
    return {
      ready: parseInt(match[1], 10),
      total: parseInt(match[2], 10)
    }
  }

  return null
}

export default function ChainDetailPage() {
  const params = useParams()
  const router = useRouter()
  const searchParams = useSearchParams()
  const chainId = params.id as string
  const { notify } = useToast()

  // Get initial tab from URL, default to 'overview'
  const tabFromUrl = searchParams.get('tab') as 'overview' | 'queues' | 'settings' | null
  const [activeTab, setActiveTab] = useState<'overview' | 'queues' | 'settings'>(
    tabFromUrl && ['overview', 'queues', 'settings'].includes(tabFromUrl)
      ? tabFromUrl
      : 'overview'
  )
  const [config, setConfig] = useState<ChainConfig | null>(null)
  const [status, setStatus] = useState<ChainStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')

  // Subscribe to WebSocket block events for this chain
  const { lastHeight, isConnected: wsConnected, error: wsError } = useBlockEvents({
    chainId: chainId,
    enabled: !loading && !!config, // Only enable after initial load
  })

  // Update last_indexed and head when new block events arrive via WebSocket
  // If indexer indexed a block, that block exists - trust the indexer
  useEffect(() => {
    if (lastHeight && status && lastHeight > status.last_indexed) {
      setStatus({
        ...status,
        last_indexed: lastHeight,
        // If indexer is ahead of head scan, update head to match
        // The indexer wouldn't have indexed it if it didn't exist
        head: Math.max(lastHeight, status.head),
      })
    }
  }, [lastHeight, status])

  // Update URL when tab changes
  const handleTabChange = (tab: 'overview' | 'queues' | 'settings') => {
    setActiveTab(tab)
    const url = new URL(window.location.href)
    url.searchParams.set('tab', tab)
    router.push(url.pathname + url.search, { scroll: false })
  }

  // Load chain configuration and status
  const loadChainData = useCallback(async () => {
    try {
      const [configRes, statusRes] = await Promise.all([
        apiFetch(`/api/chains/${chainId}`),
        apiFetch(`/api/chains/status?ids=${chainId}`),
      ])

      if (!configRes.ok) {
        if (configRes.status === 404) {
          setError('Chain not found')
        } else {
          throw new Error('Failed to load chain configuration')
        }
        return
      }

      const configData: ChainConfig = await configRes.json()
      setConfig(configData)

      if (statusRes.ok) {
        const statusData: Record<string, ChainStatus> = await statusRes.json()
        setStatus(statusData[chainId] || null)
      }
    } catch (err) {
      console.error(err)
      setError('Failed to load chain data')
    } finally {
      setLoading(false)
    }
  }, [chainId])

  useEffect(() => {
    loadChainData()
    // Poll for status updates every 30 seconds
    const interval = setInterval(loadChainData, 30_000)
    return () => clearInterval(interval)
  }, [loadChainData])

  const handleTogglePause = async () => {
    if (!config) return
    const nextPaused = config.paused ? 0 : 1
    try {
      await apiFetch('/api/chains/status', {
        method: 'PATCH',
        body: JSON.stringify([{ chain_id: chainId, paused: nextPaused }]),
      })
      setConfig({ ...config, paused: !!nextPaused })
      notify(`Chain ${nextPaused ? 'paused' : 'resumed'}`)
    } catch (err) {
      notify('Failed to update pause state', 'error')
    }
  }

  const handleHeadScan = async () => {
    try {
      const res = await apiFetch(`/api/chains/${chainId}/headscan`, { method: 'POST' })
      if (!res.ok) throw new Error('Failed to trigger head scan')
      notify('Head scan triggered')
      await loadChainData()
    } catch (err) {
      notify('Failed to trigger head scan', 'error')
    }
  }

  const handleGapScan = async () => {
    try {
      const res = await apiFetch(`/api/chains/${chainId}/gapscan`, { method: 'POST' })
      if (!res.ok) throw new Error('Failed to trigger gap scan')
      notify('Gap scan triggered')
      await loadChainData()
    } catch (err) {
      notify('Failed to trigger gap scan', 'error')
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="flex items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
          <p className="text-slate-400">Loading chain details...</p>
        </div>
      </div>
    )
  }

  if (error || !config) {
    return (
      <div className="space-y-6">
        <div className="rounded-xl border border-rose-500/50 bg-rose-500/10 p-6 text-rose-200">
          {error || 'Chain not found'}
        </div>
        <Link href="/dashboard" className="btn-secondary">
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
          </svg>
          Back to Dashboard
        </Link>
      </div>
    )
  }

  const progress = status && status.head > 0 ? Math.min((status.last_indexed / status.head) * 100, 100) : 0
  const overallHealth = status?.health?.status || 'unknown'

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="flex items-center gap-2 text-sm text-slate-400">
        <Link href="/dashboard" className="hover:text-white">
          Dashboard
        </Link>
        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="text-white">{config.chain_name || config.chain_id}</span>
      </nav>

      {/* Deleted Chain Banner */}
      {config.deleted && (
        <div className="rounded-lg border-2 border-rose-500 bg-rose-500/10 p-4">
          <div className="flex items-start gap-4">
            <div className="flex-shrink-0">
              <svg className="h-6 w-6 text-rose-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <div className="flex-1">
              <h3 className="text-lg font-semibold text-rose-200">Chain Deleted</h3>
              <p className="mt-1 text-sm text-rose-300">
                This chain was soft-deleted on {formatTimestamp(config.updated_at)}. All indexing and monitoring has been stopped.
                The chain data is preserved and can be recovered if needed.
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-bold text-white">{config.chain_name || config.chain_id}</h1>
            {config.deleted ? (
              <span className="rounded-full bg-rose-500/20 px-3 py-1 text-sm font-semibold text-rose-300 border border-rose-500/50">
                DELETED
              </span>
            ) : config.paused ? (
              <span className="badge-warning">Paused</span>
            ) : (
              <span className={getHealthBadgeClass(overallHealth)}>
                <span className={getHealthStatusDotClass(overallHealth)}></span>
                {formatHealthStatus(overallHealth)}
              </span>
            )}
          </div>
          <p className="mt-2 text-sm text-slate-400">{config.chain_id}</p>

          {/* Index Progress Indicator */}
          {status && !config.deleted && (
            <div className="mt-4 flex items-center gap-6 text-sm">
              <div className="flex items-center gap-2">
                <span className="text-slate-500">Last Indexed:</span>
                <span className="font-mono font-semibold text-white">{formatNumber(status.last_indexed)}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-slate-500">Head:</span>
                <span className="font-mono font-semibold text-white">{formatNumber(status.head)}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-slate-500">Progress:</span>
                <span className="font-mono font-semibold text-indigo-400">{progress.toFixed(2)}%</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-slate-500">Allowed:</span>
                <span className="font-mono font-semibold text-slate-300">
                  {config.min_replicas}-{config.max_replicas}
                </span>
              </div>
              {(() => {
                const liveReplicas = parseLiveReplicas(status.deployment_health?.message)
                return liveReplicas ? (
                  <div className="flex items-center gap-2">
                    <span className="text-slate-500">Live:</span>
                    <span className="font-mono font-semibold text-emerald-400">
                      {liveReplicas.ready}/{liveReplicas.total}
                    </span>
                  </div>
                ) : null
              })()}
              {status.head > status.last_indexed && (
                <div className="flex items-center gap-2">
                  <span className="text-slate-500">Lag:</span>
                  <span className="font-mono font-semibold text-amber-400">
                    {formatNumber(status.head - status.last_indexed)} blocks
                  </span>
                </div>
              )}
              <div className="flex items-center gap-2">
                <span className="text-slate-500">WS:</span>
                {wsConnected ? (
                  <span className="flex items-center gap-1 font-semibold text-emerald-400">
                    <span className="h-2 w-2 rounded-full bg-emerald-400 animate-pulse"></span>
                    Live
                  </span>
                ) : (
                  <span className="flex items-center gap-1 font-semibold text-slate-500">
                    <span className="h-2 w-2 rounded-full bg-slate-500"></span>
                    Offline
                  </span>
                )}
              </div>
            </div>
          )}
          {/* Deleted Chain Status - Show static info only */}
          {status && config.deleted && (
            <div className="mt-4 flex items-center gap-6 text-sm opacity-60">
              <div className="flex items-center gap-2">
                <span className="text-slate-500">Last Indexed:</span>
                <span className="font-mono font-semibold text-slate-400">{formatNumber(status.last_indexed)}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-slate-500">Replicas:</span>
                <span className="font-mono font-semibold text-slate-400">0/0 (Stopped)</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-slate-500">Status:</span>
                <span className="font-mono font-semibold text-rose-400">Inactive</span>
              </div>
            </div>
          )}
        </div>

        {/* Quick Actions */}
        {!config.deleted && (
          <div className="flex flex-wrap gap-2">
            <button onClick={handleHeadScan} className="btn text-sm">
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
              Head Scan
            </button>
            <button onClick={handleGapScan} className="btn text-sm">
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              Gap Scan
            </button>
            <button onClick={handleTogglePause} className="btn-secondary text-sm">
              {config.paused ? (
                <>
                  <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M6.3 2.841A1.5 1.5 0 004 4.11V15.89a1.5 1.5 0 002.3 1.269l9.344-5.89a1.5 1.5 0 000-2.538L6.3 2.84z" />
                  </svg>
                  Resume
                </>
              ) : (
                <>
                  <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M5.75 3a.75.75 0 00-.75.75v12.5c0 .414.336.75.75.75h1.5a.75.75 0 00.75-.75V3.75A.75.75 0 007.25 3h-1.5zM12.75 3a.75.75 0 00-.75.75v12.5c0 .414.336.75.75.75h1.5a.75.75 0 00.75-.75V3.75a.75.75 0 00-.75-.75h-1.5z" />
                  </svg>
                  Pause
                </>
              )}
            </button>
            <button onClick={loadChainData} className="btn-secondary text-sm">
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              Refresh
            </button>
          </div>
        )}
      </div>

      {/* Tab Navigation */}
      <div className="border-b border-slate-800">
        <nav className="flex gap-6">
          {[
            { id: 'overview', label: 'Overview' },
            { id: 'settings', label: 'Settings' },
          ].map((tab) => (
            <button
              key={tab.id}
              onClick={() => handleTabChange(tab.id as any)}
              className={`border-b-2 px-1 py-3 text-sm font-medium transition-colors ${
                activeTab === tab.id
                  ? 'border-indigo-500 text-white'
                  : 'border-transparent text-slate-400 hover:text-white'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab Content */}
      {activeTab === 'overview' && (
        <OverviewTab
          config={config}
          status={status}
          progress={progress}
          onHeadScan={handleHeadScan}
          onGapScan={handleGapScan}
          onRefresh={loadChainData}
        />
      )}

      {activeTab === 'settings' && (
        <SettingsTab config={config} onRefresh={loadChainData} isDeleted={config.deleted} />
      )}
    </div>
  )
}

// Overview Tab Component
function OverviewTab({
  config,
  status,
  progress,
  onHeadScan,
  onGapScan,
  onRefresh,
}: {
  config: ChainConfig
  status: ChainStatus | null
  progress: number
  onHeadScan: () => void
  onGapScan: () => void
  onRefresh: () => void
}) {
  const { notify } = useToast()
  const [reindexDialogOpen, setReindexDialogOpen] = useState(false)

  // Transaction data state
  const [blockSummary, setBlockSummary] = useState<BlockSummary | null>(null)
  const [transactions, setTransactions] = useState<Transaction[]>([])
  const [loadingTxData, setLoadingTxData] = useState(false)

  // Fetch transaction data on mount and when chain status changes
  useEffect(() => {
    const fetchTransactionData = async () => {
      if (!status || status.last_indexed === 0) return

      setLoadingTxData(true)
      try {
        // Fetch latest block summary for tx_counts_by_type using generic entity endpoint
        const summaryRes = await apiFetch(`/api/chains/${config.chain_id}/entity/block_summaries?limit=1&sort=desc`)
        if (summaryRes.ok) {
          const summaryData: PaginatedResponse<BlockSummary> = await summaryRes.json()
          if (summaryData.data && summaryData.data.length > 0) {
            setBlockSummary(summaryData.data[0])
          }
        }

        // Fetch recent transactions using generic entity endpoint (entity name is "txs")
        const txRes = await apiFetch(`/api/chains/${config.chain_id}/entity/txs?limit=10&sort=desc`)
        if (txRes.ok) {
          const txData: PaginatedResponse<Transaction> = await txRes.json()
          setTransactions(txData.data || [])
        }
      } catch (err) {
        console.error('Failed to fetch transaction data:', err)
      } finally {
        setLoadingTxData(false)
      }
    }

    fetchTransactionData()
  }, [config.chain_id, status?.last_indexed])

  const handleReindex = async (payload: ReindexPayload) => {
    try {
      const res = await apiFetch(`/api/chains/${config.chain_id}/reindex`, {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const errorData = await res.json().catch(() => ({ error: 'Reindex failed' }))
        throw new Error(errorData.error || 'Reindex failed')
      }
      const data = await res.json().catch(() => ({ queued: 0 }))
      notify(`Queued ${data.queued ?? 0} blocks for reindexing`)
      setReindexDialogOpen(false)
      await onRefresh()
    } catch (err: any) {
      notify(err.message || 'Failed to queue reindex', 'error')
    }
  }

  return (
    <div className="space-y-6">
      {/* Live Sync Status - NEW */}
      {status && !config.deleted && (
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">Sync Status</h3>
          </div>
          <LiveSyncStatus
            last_indexed={status.last_indexed}
            head={status.head}
            missing_blocks_count={status.missing_blocks_count}
            is_live_sync={status.is_live_sync}
          />
        </div>
      )}

      {/* Gap Ranges Display - NEW */}
      {status && !config.deleted && status.gap_ranges_count !== undefined && (
        <GapRangesDisplay
          gap_ranges_count={status.gap_ranges_count}
          largest_gap_start={status.largest_gap_start}
          largest_gap_end={status.largest_gap_end}
          missing_blocks_count={status.missing_blocks_count}
        />
      )}

      {/* Health Status Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <HealthCard
          title="Overall Health"
          status={status?.health?.status}
          message={status?.health?.message}
          updatedAt={status?.health?.updated_at}
        />
        <HealthCard
          title="RPC Health"
          status={status?.rpc_health?.status}
          message={status?.rpc_health?.message}
          updatedAt={status?.rpc_health?.updated_at}
        />
        <HealthCard
          title="Queue Health"
          status={status?.queue_health?.status}
          message={status?.queue_health?.message}
          updatedAt={status?.queue_health?.updated_at}
        />
        <HealthCard
          title="Deployment Health"
          status={status?.deployment_health?.status}
          message={status?.deployment_health?.message}
          updatedAt={status?.deployment_health?.updated_at}
        />
      </div>

      {/* Queue Metrics */}
      {status && !config.deleted && (
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">Queue Metrics</h3>
          </div>
          <QueueHealthBadge
            liveDepth={status.live_queue_depth || 0}
            liveAge={status.live_queue_backlog_age || 0}
            historicalDepth={status.historical_queue_depth || 0}
            historicalAge={status.historical_queue_backlog_age || 0}
            opsQueue={{
              pending_workflow: status.ops_queue?.pending_workflow || 0,
              backlog_age_secs: status.ops_queue?.backlog_age_secs || 0,
            }}
            compact={false}
          />
        </div>
      )}

      {/* Configuration Card */}
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Configuration</h3>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div>
            <p className="text-xs text-slate-400">Container Image</p>
            <p className="mt-1 font-mono text-sm text-slate-200">{config.image}</p>
          </div>
          <div>
            <p className="text-xs text-slate-400">Replicas</p>
            <p className="mt-1 text-sm text-slate-200">
              Min: {config.min_replicas} / Max: {config.max_replicas}
            </p>
          </div>
          <div>
            <p className="text-xs text-slate-400">Created</p>
            <p className="mt-1 text-sm text-slate-200">{formatTimestamp(config.created_at)}</p>
          </div>
          <div>
            <p className="text-xs text-slate-400">Updated</p>
            <p className="mt-1 text-sm text-slate-200">{formatTimestamp(config.updated_at)}</p>
          </div>
          {config.rpc_endpoints && config.rpc_endpoints.length > 0 && (
            <div className="md:col-span-2">
              <p className="text-xs text-slate-400">RPC Endpoints</p>
              <div className="mt-2 space-y-1">
                {config.rpc_endpoints.map((endpoint, idx) => (
                  <div key={idx} className="flex items-center gap-2">
                    <svg className="h-3 w-3 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                    <p className="font-mono text-xs text-slate-300">{endpoint}</p>
                  </div>
                ))}
              </div>
            </div>
          )}
          {config.notes && (
            <div className="md:col-span-2">
              <p className="text-xs text-slate-400">Notes</p>
              <p className="mt-1 text-sm text-slate-200">{config.notes}</p>
            </div>
          )}
        </div>
      </div>

      {/* Indexing Progress Chart */}
      <IndexingProgressChart chainId={String(config.chain_id)} />

      {/* Transaction Type Breakdown */}
      {blockSummary && blockSummary.tx_counts_by_type && (
        <div className="grid gap-6 md:grid-cols-2">
          <TransactionTypeBreakdown txCounts={blockSummary.tx_counts_by_type} />
          <TransactionList transactions={transactions} loading={loadingTxData} />
        </div>
      )}

      {/* Event Type Breakdown */}
      {status?.event_counts_by_type && (
        <EventTypeBreakdown eventCounts={status.event_counts_by_type} />
      )}

      {/* Reindex History */}
      {status?.reindex_history && status.reindex_history.length > 0 && (
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">Reindex History</h3>
          </div>
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Height</th>
                  <th>Status</th>
                  <th>Requested By</th>
                  <th>Requested At</th>
                  <th>Workflow ID</th>
                  <th>Run ID</th>
                </tr>
              </thead>
              <tbody>
                {status.reindex_history.map((entry, idx) => (
                  <tr key={idx}>
                    <td className="font-mono">{entry.height}</td>
                    <td>
                      <span className={entry.status === 'completed' ? 'badge-success' : 'badge-warning'}>
                        {entry.status}
                      </span>
                    </td>
                    <td>{entry.requested_by}</td>
                    <td className="text-slate-400">{formatTimestamp(entry.requested_at)}</td>
                    <td className="font-mono text-xs text-slate-400">
                      {entry.workflow_id ? (
                        <span title={entry.workflow_id}>{entry.workflow_id.slice(0, 20)}...</span>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td className="font-mono text-xs text-slate-400">
                      {entry.run_id ? (
                        <span title={entry.run_id}>{entry.run_id.slice(0, 20)}...</span>
                      ) : (
                        '—'
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <ReindexDialog
        open={reindexDialogOpen}
        onOpenChange={setReindexDialogOpen}
        chainName={config.chain_name || String(config.chain_id)}
        onSubmit={handleReindex}
      />
    </div>
  )
}

// Health Card Component
function HealthCard({
  title,
  status,
  message,
  updatedAt,
}: {
  title: string
  status?: string
  message?: string
  updatedAt?: string
}) {
  return (
    <div className="card">
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <p className="text-xs font-medium text-slate-400">{title}</p>
          <div className="mt-2 flex items-center gap-2">
            <span className={getHealthStatusDotClass(status)}></span>
            <span className="text-lg font-semibold text-white">{formatHealthStatus(status)}</span>
          </div>
          {message && <p className="mt-2 text-xs text-slate-500">{message}</p>}
        </div>
        <span className={getHealthBadgeClass(status)}>{formatHealthStatus(status)}</span>
      </div>
      {updatedAt && (
        <p className="mt-3 text-xs text-slate-600">Updated {formatTimestamp(updatedAt)}</p>
      )}
    </div>
  )
}

// Settings Tab Component
function SettingsTab({
  config,
  onRefresh,
  isDeleted,
}: {
  config: ChainConfig
  onRefresh: () => void
  isDeleted: boolean
}) {
  const { notify } = useToast()
  const router = useRouter()

  const [image, setImage] = useState(config.image)
  const [minReplicas, setMinReplicas] = useState(config.min_replicas)
  const [maxReplicas, setMaxReplicas] = useState(config.max_replicas)
  const [reindexMinReplicas, setReindexMinReplicas] = useState(config.reindex_min_replicas)
  const [reindexMaxReplicas, setReindexMaxReplicas] = useState(config.reindex_max_replicas)
  const [reindexScaleThreshold, setReindexScaleThreshold] = useState(config.reindex_scale_threshold)
  const [notes, setNotes] = useState(config.notes || '')
  const [rpcEndpoints, setRpcEndpoints] = useState<string[]>(config.rpc_endpoints || [])
  const [newEndpoint, setNewEndpoint] = useState('')
  const [paused, setPaused] = useState(config.paused)
  const [saving, setSaving] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  const handleAddEndpoint = () => {
    if (!newEndpoint.trim()) return
    try {
      new URL(newEndpoint)
      setRpcEndpoints([...rpcEndpoints, newEndpoint.trim()])
      setNewEndpoint('')
    } catch {
      notify('Invalid URL format', 'error')
    }
  }

  const handleRemoveEndpoint = (index: number) => {
    setRpcEndpoints(rpcEndpoints.filter((_, i) => i !== index))
  }

  const handleSave = async () => {
    if (rpcEndpoints.length === 0) {
      notify('At least one RPC endpoint is required', 'error')
      return
    }

    setSaving(true)
    try {
      const res = await apiFetch(`/api/chains/${config.chain_id}`, {
        method: 'PATCH',
        body: JSON.stringify({
          image,
          min_replicas: minReplicas,
          max_replicas: maxReplicas,
          reindex_min_replicas: reindexMinReplicas,
          reindex_max_replicas: reindexMaxReplicas,
          reindex_scale_threshold: reindexScaleThreshold,
          notes: notes.trim() || undefined,
          rpc_endpoints: rpcEndpoints,
        }),
      })
      if (!res.ok) throw new Error('Failed to update chain')
      notify('Chain updated successfully')
      await onRefresh()
    } catch (err) {
      notify('Failed to update chain', 'error')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Chain Configuration</h3>
        </div>

        <div className="space-y-6">
          {/* Read-only fields */}
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">Chain Name</label>
              <input
                type="text"
                value={config.chain_name}
                className="input"
                disabled
              />
            </div>
            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">Chain ID</label>
              <input
                type="text"
                value={config.chain_id}
                className="input"
                disabled
              />
            </div>
          </div>

          {/* RPC Endpoints */}
          <div>
            <label className="mb-2 block text-sm font-medium text-slate-300">
              RPC Endpoints <span className="text-xs text-slate-500">({rpcEndpoints.length} configured)</span>
            </label>
            <div className="space-y-2">
              {rpcEndpoints.map((endpoint, index) => (
                <div key={index} className="flex items-center gap-2">
                  <input
                    type="text"
                    value={endpoint}
                    className="input flex-1"
                    disabled
                  />
                  <button
                    onClick={() => handleRemoveEndpoint(index)}
                    className="btn-danger px-3 py-2"
                    title="Remove endpoint"
                    disabled={isDeleted}
                  >
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>
              ))}

              <div className="flex items-center gap-2">
                <input
                  type="url"
                  value={newEndpoint}
                  onChange={(e) => setNewEndpoint(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleAddEndpoint()}
                  className="input flex-1"
                  placeholder="https://rpc.example.com"
                  disabled={isDeleted}
                />
                <button onClick={handleAddEndpoint} className="btn-secondary" disabled={isDeleted}>
                  <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  Add Endpoint
                </button>
              </div>
            </div>
          </div>

          {/* Container Image */}
          <div>
            <label className="mb-2 block text-sm font-medium text-slate-300">Container Image</label>
            <input
              type="text"
              value={image}
              onChange={(e) => setImage(e.target.value)}
              className="input"
              placeholder="ghcr.io/..."
              disabled={isDeleted}
            />
          </div>

          {/* Regular Indexing Replicas */}
          <div>
            <h3 className="mb-3 text-sm font-semibold text-slate-300">Regular Indexing</h3>
            <div className="grid gap-4 md:grid-cols-2">
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">Min Replicas</label>
                <input
                  type="number"
                  min={1}
                  value={minReplicas}
                  onChange={(e) => setMinReplicas(Number(e.target.value))}
                  className="input"
                  disabled={isDeleted}
                />
                <p className="mt-1 text-xs text-slate-500">Minimum number of worker replicas</p>
              </div>
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">Max Replicas</label>
                <input
                  type="number"
                  min={minReplicas}
                  value={maxReplicas}
                  onChange={(e) => setMaxReplicas(Number(e.target.value))}
                  className="input"
                  disabled={isDeleted}
                />
                <p className="mt-1 text-xs text-slate-500">Maximum number of worker replicas</p>
              </div>
            </div>
          </div>

          {/* Reindex Settings */}
          <div>
            <h3 className="mb-3 text-sm font-semibold text-slate-300">Reindex Settings</h3>
            <div className="grid gap-4 md:grid-cols-3">
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">Min Replicas</label>
                <input
                  type="number"
                  min={1}
                  value={reindexMinReplicas}
                  onChange={(e) => setReindexMinReplicas(Number(e.target.value))}
                  className="input"
                  disabled={isDeleted}
                />
                <p className="mt-1 text-xs text-slate-500">Min replicas during reindex</p>
              </div>
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">Max Replicas</label>
                <input
                  type="number"
                  min={reindexMinReplicas}
                  value={reindexMaxReplicas}
                  onChange={(e) => setReindexMaxReplicas(Number(e.target.value))}
                  className="input"
                  disabled={isDeleted}
                />
                <p className="mt-1 text-xs text-slate-500">Max replicas during reindex</p>
              </div>
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">Scale Threshold</label>
                <input
                  type="number"
                  min={1}
                  value={reindexScaleThreshold}
                  onChange={(e) => setReindexScaleThreshold(Number(e.target.value))}
                  className="input"
                  disabled={isDeleted}
                />
                <p className="mt-1 text-xs text-slate-500">Queue depth to trigger scaling</p>
              </div>
            </div>
          </div>

          {/* Notes */}
          <div>
            <label className="mb-2 block text-sm font-medium text-slate-300">Notes</label>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className="textarea"
              rows={4}
              placeholder="Optional notes about this chain..."
              disabled={isDeleted}
            />
          </div>

          {/* Paused Status */}
          <div className="flex items-center justify-between rounded-lg border border-slate-800 bg-slate-900/30 p-4">
            <div>
              <p className="font-medium text-white">Paused Status</p>
              <p className="mt-1 text-sm text-slate-400">
                {paused ? 'Indexing is currently paused' : 'Indexing is currently active'}
              </p>
            </div>
            <label className="relative inline-flex cursor-pointer items-center">
              <input
                type="checkbox"
                checked={paused}
                onChange={(e) => setPaused(e.target.checked)}
                className="peer sr-only"
                disabled={isDeleted}
              />
              <div className="peer h-6 w-11 rounded-full bg-slate-700 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all after:content-[''] peer-checked:bg-indigo-600 peer-checked:after:translate-x-full peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-indigo-500 peer-disabled:opacity-50 peer-disabled:cursor-not-allowed"></div>
            </label>
          </div>

          {/* Save Button */}
          {!isDeleted && (
            <button onClick={handleSave} className="btn w-full" disabled={saving}>
              {saving ? (
                <>
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></div>
                  Saving...
                </>
              ) : (
                <>
                  <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                  </svg>
                  Save Changes
                </>
              )}
            </button>
          )}
        </div>
      </div>

      {/* LP Snapshot Management */}
      <LPSnapshotManagement chainId={String(config.chain_id)} isDeleted={isDeleted} />

      {/* Danger Zone */}
      <div className="card border-rose-500/50 bg-rose-500/5">
        <div className="card-header">
          <h3 className="card-title text-rose-200">Danger Zone</h3>
        </div>
        <p className="text-sm text-rose-300">
          Deleting a chain will <strong>permanently delete</strong> all chain data from the database, including configuration, indexing progress, and blockchain data. This action cannot be undone.
        </p>
        <button
          onClick={() => setDeleteDialogOpen(true)}
          className="btn-danger mt-4"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
          Permanently Delete Chain
        </button>
      </div>

      <DeleteChainDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        chainId={String(config.chain_id)}
        chainName={config.chain_name}
        onSuccess={() => {
          notify('Chain permanently deleted')
          router.push('/dashboard')
        }}
      />
    </div>
  )
}

// Reindex Dialog Component
function ReindexDialog({
  open,
  onOpenChange,
  chainName,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  chainName: string
  onSubmit: (payload: ReindexPayload) => Promise<void>
}) {
  const [height, setHeight] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open) {
      setHeight('')
      setFrom('')
      setTo('')
      setError('')
    }
  }, [open])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    const payload: ReindexPayload = {}

    if (from && to) {
      const f = Number(from)
      const t = Number(to)
      if (Number.isNaN(f) || Number.isNaN(t) || t < f) {
        setError('Invalid range')
        return
      }
      payload.from = f
      payload.to = t
    } else if (height) {
      const h = Number(height)
      if (Number.isNaN(h)) {
        setError('Invalid height')
        return
      }
      payload.heights = [h]
    } else {
      setError('Enter a height or range')
      return
    }

    setSaving(true)
    try {
      await onSubmit(payload)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-2xl">
          <Dialog.Title className="text-xl font-bold text-white">Reindex Blocks</Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-slate-400">
            Queue reindex workflows for {chainName}
          </Dialog.Description>

          <form onSubmit={handleSubmit} className="mt-6 space-y-4">
            {error && (
              <div className="rounded-lg border border-rose-500/50 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
                {error}
              </div>
            )}

            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Single Height
              </label>
              <input
                type="text"
                value={height}
                onChange={(e) => setHeight(e.target.value)}
                className="input"
                placeholder="e.g., 12345"
              />
            </div>

            <div className="relative">
              <div className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-slate-700"></div>
              </div>
              <div className="relative flex justify-center text-xs">
                <span className="bg-slate-900 px-2 text-slate-500">OR</span>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">
                  Range From
                </label>
                <input
                  type="text"
                  value={from}
                  onChange={(e) => setFrom(e.target.value)}
                  className="input"
                  placeholder="start"
                />
              </div>
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">Range To</label>
                <input
                  type="text"
                  value={to}
                  onChange={(e) => setTo(e.target.value)}
                  className="input"
                  placeholder="end"
                />
              </div>
            </div>

            <p className="text-xs text-slate-500">
              Specify either a single height or a range (max 500 blocks per request).
            </p>

            <div className="flex justify-end gap-3 pt-4">
              <Dialog.Close asChild>
                <button type="button" className="btn-secondary" disabled={saving}>
                  Cancel
                </button>
              </Dialog.Close>
              <button type="submit" className="btn" disabled={saving}>
                {saving ? 'Queuing...' : 'Queue Reindex'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

// Delete Chain Dialog Component
function DeleteChainDialog({
  open,
  onOpenChange,
  chainId,
  chainName,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  chainId: string
  chainName: string
  onSuccess: () => void
}) {
  const { notify } = useToast()
  const [confirmText, setConfirmText] = useState('')
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    if (!open) {
      setConfirmText('')
    }
  }, [open])

  const handleDelete = async () => {
    if (confirmText !== String(chainId)) {
      notify('Chain ID does not match', 'error')
      return
    }

    setDeleting(true)
    try {
      const res = await apiFetch(`/api/chains/${chainId}?hard=true`, {
        method: 'DELETE',
      })
      if (!res.ok) throw new Error('Failed to delete chain')
      onSuccess()
      onOpenChange(false)
    } catch (err) {
      notify('Failed to delete chain', 'error')
    } finally {
      setDeleting(false)
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border border-rose-500/50 bg-slate-900 p-6 shadow-2xl">
          <Dialog.Title className="text-xl font-bold text-rose-200">
            Permanently Delete Chain
          </Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-slate-400">
            This action cannot be undone. This will permanently delete the chain configuration and all associated data.
          </Dialog.Description>

          <div className="mt-6 space-y-4">
            <div className="rounded-lg border border-rose-500/50 bg-rose-500/10 p-4 text-sm text-rose-200">
              <p className="font-semibold">The following will be permanently deleted:</p>
              <ul className="mt-2 list-inside list-disc space-y-1">
                <li>Chain configuration</li>
                <li>Indexing progress and metadata</li>
                <li>All indexed blockchain data</li>
                <li>Reindex history</li>
              </ul>
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Type <span className="font-mono text-rose-400">{chainId}</span> to confirm
              </label>
              <input
                type="text"
                value={confirmText}
                onChange={(e) => setConfirmText(e.target.value)}
                className="input border-rose-500/50 focus:border-rose-500 focus:ring-rose-500/20"
                placeholder="Enter chain ID"
              />
            </div>

            <div className="flex justify-end gap-3 pt-4">
              <Dialog.Close asChild>
                <button type="button" className="btn-secondary" disabled={deleting}>
                  Cancel
                </button>
              </Dialog.Close>
              <button
                onClick={handleDelete}
                className="btn-danger"
                disabled={deleting || confirmText !== String(chainId)}
              >
                {deleting ? (
                  <>
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></div>
                    Permanently Deleting...
                  </>
                ) : (
                  <>
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                    Permanently Delete
                  </>
                )}
              </button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
