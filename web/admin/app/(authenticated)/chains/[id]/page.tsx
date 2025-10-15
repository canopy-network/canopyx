'use client'

import { useCallback, useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import * as Dialog from '@radix-ui/react-dialog'
import { apiFetch } from '../../../lib/api'
import { useToast } from '../../../components/ToastProvider'

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
}

type ChainConfig = {
  chain_id: string
  chain_name: string
  rpc_endpoints: string[]
  paused: boolean
  deleted: boolean
  image: string
  min_replicas: number
  max_replicas: number
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
  reindex_history?: ReindexEntry[]
  health: HealthInfo
  rpc_health: HealthInfo
  queue_health: HealthInfo
  deployment_health: HealthInfo
}

type ReindexPayload = {
  heights?: number[]
  from?: number
  to?: number
}

// Explorer tab types
type ExplorerTable = 'blocks' | 'txs' | 'txs_raw'

type SchemaColumn = {
  name: string
  type: string
}

type SchemaResponse = {
  columns: SchemaColumn[]
}

type PaginatedResponse<T> = {
  data: T[]
  limit: number
  next_cursor: number | null
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

export default function ChainDetailPage() {
  const params = useParams()
  const router = useRouter()
  const chainId = params.id as string
  const { notify } = useToast()

  const [activeTab, setActiveTab] = useState<'overview' | 'queues' | 'explorer' | 'settings'>('overview')
  const [config, setConfig] = useState<ChainConfig | null>(null)
  const [status, setStatus] = useState<ChainStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')

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

  const progress = status && status.head > 0 ? (status.last_indexed / status.head) * 100 : 0
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
        <Link href="/chains" className="hover:text-white">
          Chains
        </Link>
        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="text-white">{config.chain_name || config.chain_id}</span>
      </nav>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-bold text-white">{config.chain_name || config.chain_id}</h1>
            {config.paused ? (
              <span className="badge-warning">Paused</span>
            ) : (
              <span className={getHealthBadgeClass(overallHealth)}>
                <span className={getHealthStatusDotClass(overallHealth)}></span>
                {formatHealthStatus(overallHealth)}
              </span>
            )}
          </div>
          <p className="mt-2 text-sm text-slate-400">{config.chain_id}</p>
        </div>

        {/* Quick Actions */}
        <div className="flex gap-2">
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
      </div>

      {/* Tab Navigation */}
      <div className="border-b border-slate-800">
        <nav className="flex gap-6">
          {[
            { id: 'overview', label: 'Overview' },
            { id: 'queues', label: 'Queues' },
            { id: 'explorer', label: 'Explorer' },
            { id: 'settings', label: 'Settings' },
          ].map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id as any)}
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

      {activeTab === 'queues' && <QueuesTab status={status} onRefresh={loadChainData} />}

      {activeTab === 'explorer' && <ExplorerTab chainId={chainId} />}

      {activeTab === 'settings' && (
        <SettingsTab config={config} onRefresh={loadChainData} />
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

      {/* Stats Grid */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="card">
          <p className="text-xs font-medium text-slate-400">Last Indexed</p>
          <p className="mt-2 font-mono text-2xl font-bold text-white">
            {formatNumber(status?.last_indexed ?? 0)}
          </p>
        </div>
        <div className="card">
          <p className="text-xs font-medium text-slate-400">Head</p>
          <p className="mt-2 font-mono text-2xl font-bold text-white">
            {formatNumber(status?.head ?? 0)}
          </p>
        </div>
        <div className="card">
          <p className="text-xs font-medium text-slate-400">Progress</p>
          <p className="mt-2 font-mono text-2xl font-bold text-indigo-400">
            {progress.toFixed(2)}%
          </p>
        </div>
        <div className="card">
          <p className="text-xs font-medium text-slate-400">Lag (blocks)</p>
          <p className="mt-2 font-mono text-2xl font-bold text-amber-400">
            {formatNumber((status?.head ?? 0) - (status?.last_indexed ?? 0))}
          </p>
        </div>
      </div>

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
          {config.notes && (
            <div className="md:col-span-2">
              <p className="text-xs text-slate-400">Notes</p>
              <p className="mt-1 text-sm text-slate-200">{config.notes}</p>
            </div>
          )}
        </div>
      </div>

      {/* Progress Visualization Placeholder */}
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Indexing Progress</h3>
        </div>
        <div className="flex items-center justify-center py-12 text-slate-500">
          <div className="text-center">
            <svg className="mx-auto h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
            </svg>
            <p className="mt-4 text-sm">Chart visualization coming soon</p>
            <p className="mt-1 text-xs text-slate-600">Historical progress tracking</p>
          </div>
        </div>
      </div>

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
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Action Buttons */}
      <div className="flex gap-3">
        <button onClick={onHeadScan} className="btn">
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
          Trigger Head Scan
        </button>
        <button onClick={onGapScan} className="btn">
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          Trigger Gap Scan
        </button>
        <button onClick={() => setReindexDialogOpen(true)} className="btn-secondary">
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Trigger Reindex
        </button>
      </div>

      <ReindexDialog
        open={reindexDialogOpen}
        onOpenChange={setReindexDialogOpen}
        chainName={config.chain_name || config.chain_id}
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

// Queues Tab Component
function QueuesTab({
  status,
  onRefresh,
}: {
  status: ChainStatus | null
  onRefresh: () => void
}) {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-white">Queue Metrics</h2>
        <button onClick={onRefresh} className="btn-secondary text-sm">
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Refresh
        </button>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Ops Queue */}
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">Ops Queue</h3>
            <span className="font-mono text-xs text-slate-500">admin:{status?.chain_id}</span>
          </div>

          <div className="space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <span className="text-sm text-slate-400">Pending Workflows</span>
              <span className="font-mono text-2xl font-bold text-amber-400">
                {formatNumber(status?.ops_queue?.pending_workflow ?? 0)}
              </span>
            </div>

            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <span className="text-sm text-slate-400">Pending Activities</span>
              <span className="font-mono text-2xl font-bold text-purple-400">
                {formatNumber(status?.ops_queue?.pending_activity ?? 0)}
              </span>
            </div>

            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <span className="text-sm text-slate-400">Backlog Age</span>
              <span className="font-mono text-xl font-semibold text-slate-300">
                {secondsToFriendly(status?.ops_queue?.backlog_age_secs ?? 0)}
              </span>
            </div>

            <div className="flex items-center justify-between">
              <span className="text-sm text-slate-400">Pollers</span>
              <span className="font-mono text-xl font-semibold text-slate-300">
                {status?.ops_queue?.pollers ?? 0}
              </span>
            </div>
          </div>

          <div className="mt-4 rounded-lg bg-slate-950/50 p-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-slate-400">Health Status</span>
              <span className={getHealthBadgeClass(status?.queue_health?.status)}>
                {formatHealthStatus(status?.queue_health?.status)}
              </span>
            </div>
            {status?.queue_health?.message && (
              <p className="mt-2 text-xs text-slate-500">{status.queue_health.message}</p>
            )}
          </div>
        </div>

        {/* Indexer Queue */}
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">Indexer Queue</h3>
            <span className="font-mono text-xs text-slate-500">index:{status?.chain_id}</span>
          </div>

          <div className="space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <span className="text-sm text-slate-400">Pending Workflows</span>
              <span className="font-mono text-2xl font-bold text-amber-400">
                {formatNumber(status?.indexer_queue?.pending_workflow ?? 0)}
              </span>
            </div>

            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <span className="text-sm text-slate-400">Pending Activities</span>
              <span className="font-mono text-2xl font-bold text-purple-400">
                {formatNumber(status?.indexer_queue?.pending_activity ?? 0)}
              </span>
            </div>

            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <span className="text-sm text-slate-400">Backlog Age</span>
              <span className="font-mono text-xl font-semibold text-slate-300">
                {secondsToFriendly(status?.indexer_queue?.backlog_age_secs ?? 0)}
              </span>
            </div>

            <div className="flex items-center justify-between">
              <span className="text-sm text-slate-400">Pollers</span>
              <span className="font-mono text-xl font-semibold text-slate-300">
                {status?.indexer_queue?.pollers ?? 0}
              </span>
            </div>
          </div>

          <div className="mt-4 rounded-lg bg-slate-950/50 p-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-slate-400">Health Status</span>
              <span className={getHealthBadgeClass(status?.queue_health?.status)}>
                {formatHealthStatus(status?.queue_health?.status)}
              </span>
            </div>
            {status?.queue_health?.message && (
              <p className="mt-2 text-xs text-slate-500">{status.queue_health.message}</p>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// Explorer Tab Component
function ExplorerTab({ chainId }: { chainId: string }) {
  const [selectedTable, setSelectedTable] = useState<ExplorerTable>('blocks')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>('')
  const [schema, setSchema] = useState<string[]>([])
  const [data, setData] = useState<any[]>([])
  const [nextCursor, setNextCursor] = useState<number | null>(null)
  const [cursors, setCursors] = useState<(number | null)[]>([null]) // Stack of cursors for navigation
  const [currentPageIndex, setCurrentPageIndex] = useState(0)

  const QUERY_SERVICE_URL = process.env.NEXT_PUBLIC_QUERY_SERVICE_URL || 'http://localhost:8082'
  const ITEMS_PER_PAGE = 50

  // Map UI table names to API table names
  const TABLE_NAME_MAP: Record<ExplorerTable, string> = {
    blocks: 'blocks',
    txs: 'transactions',
    txs_raw: 'transactions_raw',
  }

  // Fetch schema when table changes
  useEffect(() => {
    const fetchSchema = async () => {
      setLoading(true)
      setError('')
      try {
        const tableName = TABLE_NAME_MAP[selectedTable]
        const response = await fetch(
          `${QUERY_SERVICE_URL}/chains/${chainId}/schema?table=${tableName}`
        )

        if (!response.ok) {
          throw new Error(`Failed to fetch schema: ${response.statusText}`)
        }

        const schemaData: SchemaResponse = await response.json()
        const columnNames = schemaData.columns.map((col) => col.name)
        setSchema(columnNames)
      } catch (err: any) {
        console.error('Schema fetch error:', err)
        setError(err.message || 'Failed to load schema')
        setSchema([])
      } finally {
        setLoading(false)
      }
    }

    fetchSchema()
  }, [chainId, selectedTable, QUERY_SERVICE_URL])

  // Fetch data when table or page changes
  useEffect(() => {
    const fetchData = async () => {
      setLoading(true)
      setError('')
      try {
        const tableName = TABLE_NAME_MAP[selectedTable]
        const cursor = cursors[currentPageIndex]
        const cursorParam = cursor !== null ? `&cursor=${cursor}` : ''

        const response = await fetch(
          `${QUERY_SERVICE_URL}/chains/${chainId}/${tableName}?limit=${ITEMS_PER_PAGE}${cursorParam}`
        )

        if (!response.ok) {
          throw new Error(`Failed to fetch data: ${response.statusText}`)
        }

        const result: PaginatedResponse<any> = await response.json()
        setData(result.data || [])
        setNextCursor(result.next_cursor ?? null)
      } catch (err: any) {
        console.error('Data fetch error:', err)
        setError(err.message || 'Failed to load data')
        setData([])
        setNextCursor(null)
      } finally {
        setLoading(false)
      }
    }

    if (schema.length > 0) {
      fetchData()
    }
  }, [chainId, selectedTable, currentPageIndex, cursors, schema.length, QUERY_SERVICE_URL])

  const handleTableChange = (newTable: ExplorerTable) => {
    setSelectedTable(newTable)
    setCursors([null])
    setCurrentPageIndex(0)
    setData([])
    setNextCursor(null)
    setError('')
  }

  const handleNextPage = () => {
    if (nextCursor !== null) {
      const newCursors = [...cursors.slice(0, currentPageIndex + 1), nextCursor]
      setCursors(newCursors)
      setCurrentPageIndex(currentPageIndex + 1)
    }
  }

  const handlePreviousPage = () => {
    if (currentPageIndex > 0) {
      setCurrentPageIndex(currentPageIndex - 1)
    }
  }

  return (
    <div className="space-y-6">
      {/* Error Banner */}
      {error && (
        <div className="rounded-lg border border-rose-500/50 bg-rose-500/10 p-4 text-rose-200">
          <div className="flex items-start gap-3">
            <svg className="h-5 w-5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <div>
              <p className="font-semibold">Error Loading Explorer Data</p>
              <p className="mt-1 text-sm text-rose-300">{error}</p>
              <p className="mt-2 text-xs text-rose-300">
                Make sure the query service is running at {QUERY_SERVICE_URL}
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Table Selector */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <label className="text-sm font-medium text-slate-300">Select Table:</label>
          <select
            value={selectedTable}
            onChange={(e) => handleTableChange(e.target.value as ExplorerTable)}
            className="input w-auto"
            disabled={loading}
          >
            <option value="blocks">Blocks</option>
            <option value="txs">Transactions</option>
            <option value="txs_raw">Transactions Raw</option>
          </select>
        </div>

        {/* Pagination Controls */}
        <div className="flex items-center gap-2">
          <button
            onClick={handlePreviousPage}
            disabled={currentPageIndex === 0 || loading}
            className="btn-secondary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Previous
          </button>
          <span className="text-sm text-slate-400">
            Page {currentPageIndex + 1}
          </span>
          <button
            onClick={handleNextPage}
            disabled={nextCursor === null || loading}
            className="btn-secondary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Next
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
            </svg>
          </button>
        </div>
      </div>

      {/* Schema Display */}
      {schema.length > 0 && (
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">Schema: {selectedTable}</h3>
            <span className="text-xs text-slate-500">{schema.length} columns</span>
          </div>
          <div className="flex flex-wrap gap-2">
            {schema.map((col) => (
              <span key={col} className="badge-neutral">
                {col}
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Data Table */}
      <div className="card overflow-hidden p-0">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <div className="flex items-center gap-3">
              <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
              <p className="text-slate-400">Loading data...</p>
            </div>
          </div>
        ) : data.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-slate-500">
            <div className="text-center">
              <svg className="mx-auto h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
              </svg>
              <p className="mt-4 text-sm">No data available</p>
              <p className="mt-1 text-xs text-slate-600">This table may be empty or not yet indexed</p>
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  {schema.map((col) => (
                    <th key={col}>{col}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.map((row, idx) => (
                  <tr key={idx}>
                    {schema.map((col) => (
                      <td key={col} className="font-mono text-xs">
                        {row[col] !== null && row[col] !== undefined
                          ? String(row[col])
                          : '—'}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Results Info */}
      {data.length > 0 && (
        <div className="text-center text-xs text-slate-500">
          Showing {data.length} {data.length === 1 ? 'row' : 'rows'}
          {nextCursor !== null && ' • More results available'}
        </div>
      )}
    </div>
  )
}

// Settings Tab Component
function SettingsTab({
  config,
  onRefresh,
}: {
  config: ChainConfig
  onRefresh: () => void
}) {
  const { notify } = useToast()
  const router = useRouter()

  const [image, setImage] = useState(config.image)
  const [minReplicas, setMinReplicas] = useState(config.min_replicas)
  const [maxReplicas, setMaxReplicas] = useState(config.max_replicas)
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
                />
                <button onClick={handleAddEndpoint} className="btn-secondary">
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
            />
          </div>

          {/* Replicas */}
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">Min Replicas</label>
              <input
                type="number"
                min={1}
                value={minReplicas}
                onChange={(e) => setMinReplicas(Number(e.target.value))}
                className="input"
              />
            </div>
            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">Max Replicas</label>
              <input
                type="number"
                min={minReplicas}
                value={maxReplicas}
                onChange={(e) => setMaxReplicas(Number(e.target.value))}
                className="input"
              />
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
              />
              <div className="peer h-6 w-11 rounded-full bg-slate-700 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all after:content-[''] peer-checked:bg-indigo-600 peer-checked:after:translate-x-full peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-indigo-500"></div>
            </label>
          </div>

          {/* Save Button */}
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
        </div>
      </div>

      {/* Danger Zone */}
      <div className="card border-rose-500/50 bg-rose-500/5">
        <div className="card-header">
          <h3 className="card-title text-rose-200">Danger Zone</h3>
        </div>
        <p className="text-sm text-rose-300">
          Deleting a chain will permanently remove all configuration, indexing progress, and chain data.
          This action cannot be undone.
        </p>
        <button
          onClick={() => setDeleteDialogOpen(true)}
          className="btn-danger mt-4"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
          Delete Chain
        </button>
      </div>

      <DeleteChainDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        chainId={config.chain_id}
        chainName={config.chain_name}
        onSuccess={() => {
          notify('Chain deleted successfully')
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
    if (confirmText !== chainId) {
      notify('Chain ID does not match', 'error')
      return
    }

    setDeleting(true)
    try {
      const res = await apiFetch(`/api/chains/${chainId}`, {
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
          <Dialog.Title className="text-xl font-bold text-rose-200">Delete Chain</Dialog.Title>
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
                disabled={deleting || confirmText !== chainId}
              >
                {deleting ? (
                  <>
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></div>
                    Deleting...
                  </>
                ) : (
                  <>
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                    Delete Chain
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