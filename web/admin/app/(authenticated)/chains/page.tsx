'use client'

import { useCallback, useEffect, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import * as Tooltip from '@radix-ui/react-tooltip'
import { useToast } from '../../components/ToastProvider'
import { apiFetch } from '../../lib/api'
import CreateChainDialog from '../../components/CreateChainDialog'

type ChainRow = {
  chain_id: string
  chain_name: string
  rpc_endpoints: string[]
  paused: number
  deleted?: number
  image: string
  min_replicas: number
  max_replicas: number
  notes?: string
}

type QueueStatus = {
  pending_workflow: number
  pending_activity: number
  backlog_age_secs: number
  pollers: number
}

type HealthStatus = {
  status: string
  message: string
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
  queue: QueueStatus
  health?: HealthStatus
  rpc_health?: HealthStatus
  queue_health?: HealthStatus
  deployment_health?: HealthStatus
  reindex_history?: ReindexEntry[]
}

type ChainStatusMap = Record<string, ChainStatus>

type ReindexPayload = {
  heights?: number[]
  from?: number
  to?: number
}

type ReindexEntry = {
  height: number
  status: string
  requested_by: string
  requested_at: string
}

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
    const now = new Date()
    const diffMs = now.getTime() - date.getTime()
    const diffSecs = Math.floor(diffMs / 1000)

    if (diffSecs < 60) return `${diffSecs}s ago`
    if (diffSecs < 3600) return `${Math.floor(diffSecs / 60)}m ago`
    if (diffSecs < 86400) return `${Math.floor(diffSecs / 3600)}h ago`

    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString()
  } catch {
    return 'Invalid date'
  }
}

export default function ChainsPage() {
  const [chains, setChains] = useState<ChainRow[]>([])
  const [status, setStatus] = useState<ChainStatusMap>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')
  const [selectedChain, setSelectedChain] = useState<ChainRow | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [reindexDialogOpen, setReindexDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const { notify } = useToast()

  const loadChains = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await apiFetch('/api/chains')
      if (!res.ok) throw new Error('Failed to load chains')
      const list: ChainRow[] = await res.json()
      setChains(list)
    } catch (err) {
      console.error(err)
      setError('Unable to load chains')
    } finally {
      setLoading(false)
    }
  }, [])

  const loadStatus = useCallback(async (chainIds: string[]) => {
    if (!chainIds.length) return
    try {
      const res = await apiFetch(`/api/chains/status?ids=${encodeURIComponent(chainIds.join(','))}`)
      if (!res.ok) throw new Error('status')
      const data: ChainStatusMap = await res.json()
      setStatus(data)
    } catch (err) {
      console.warn('status polling failed', err)
    }
  }, [])

  useEffect(() => {
    loadChains()
  }, [loadChains])

  useEffect(() => {
    if (!chains.length) return
    loadStatus(chains.map((c) => c.chain_id))
    const interval = setInterval(() => loadStatus(chains.map((c) => c.chain_id)), 15_000)
    return () => clearInterval(interval)
  }, [chains, loadStatus])

  const handleCreateChain = async (data: any) => {
    setSaving(true)
    try {
      const res = await apiFetch('/api/chains', {
        method: 'POST',
        body: JSON.stringify(data),
      })
      if (!res.ok) {
        const errorData = await res.json().catch(() => ({ error: 'Failed to create chain' }))
        throw new Error(errorData.error || 'Failed to create chain')
      }
      notify('Chain created successfully')
      setCreateDialogOpen(false)
      await loadChains()
    } catch (err: any) {
      notify(err.message || 'Failed to create chain', 'error')
      throw err
    } finally {
      setSaving(false)
    }
  }

  const handleEditChain = async (updates: Partial<ChainRow>) => {
    if (!selectedChain) return
    setSaving(true)
    try {
      const payload = { ...selectedChain, ...updates }
      const res = await apiFetch(`/api/chains/${selectedChain.chain_id}`, {
        method: 'PATCH',
        body: JSON.stringify(payload),
      })
      if (!res.ok) throw new Error('Failed to update chain')
      notify('Chain updated successfully')
      setEditDialogOpen(false)
      await loadChains()
    } catch (err) {
      notify('Failed to update chain', 'error')
    } finally {
      setSaving(false)
    }
  }

  const handleTogglePause = async (chain: ChainRow) => {
    const nextPaused = chain.paused ? 0 : 1
    try {
      await apiFetch('/api/chains/status', {
        method: 'PATCH',
        body: JSON.stringify([{ chain_id: chain.chain_id, paused: nextPaused }]),
      })
      setChains((list) =>
        list.map((c) => (c.chain_id === chain.chain_id ? { ...c, paused: nextPaused } : c))
      )
      notify(`Chain ${nextPaused ? 'paused' : 'resumed'}`)
    } catch (err) {
      notify('Failed to update pause state', 'error')
    }
  }

  const handleHeadScan = async (chain: ChainRow) => {
    try {
      const res = await apiFetch(`/api/chains/${chain.chain_id}/headscan`, { method: 'POST' })
      if (!res.ok) throw new Error('Failed to trigger head scan')
      notify('Head scan triggered')
      await loadStatus([chain.chain_id])
    } catch (err) {
      notify('Failed to trigger head scan', 'error')
    }
  }

  const handleGapScan = async (chain: ChainRow) => {
    try {
      const res = await apiFetch(`/api/chains/${chain.chain_id}/gapscan`, { method: 'POST' })
      if (!res.ok) throw new Error('Failed to trigger gap scan')
      notify('Gap scan triggered')
      await loadStatus([chain.chain_id])
    } catch (err) {
      notify('Failed to trigger gap scan', 'error')
    }
  }

  const handleReindex = async (payload: ReindexPayload) => {
    if (!selectedChain) return
    setSaving(true)
    try {
      const res = await apiFetch(`/api/chains/${selectedChain.chain_id}/reindex`, {
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
      await loadStatus([selectedChain.chain_id])
    } catch (err: any) {
      notify(err.message || 'Failed to queue reindex', 'error')
    } finally {
      setSaving(false)
    }
  }

  if (loading && chains.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="flex items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
          <p className="text-slate-400">Loading chains...</p>
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
    <Tooltip.Provider delayDuration={200}>
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-white">Chains</h1>
            <p className="mt-2 text-slate-400">Manage blockchain indexer configurations</p>
          </div>
          <div className="flex gap-3">
            <button onClick={() => loadChains()} className="btn-secondary" disabled={loading}>
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
            <button onClick={() => setCreateDialogOpen(true)} className="btn">
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
              </svg>
              Add Chain
            </button>
          </div>
        </div>

        {/* Chains Grid */}
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
              <button onClick={() => setCreateDialogOpen(true)} className="btn mt-6">
                <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                Add Chain
              </button>
            </div>
          </div>
        ) : (
          <div className="grid gap-6 lg:grid-cols-2">
            {chains.map((chain) => {
              const st = status[chain.chain_id]
              const backlog = st ? st.queue.pending_workflow + st.queue.pending_activity : 0
              const overallHealth = st?.health?.status || 'unknown'

              return (
                <div key={chain.chain_id} className="card group">
                  <div className="flex items-start justify-between">
                    <div className="flex-1">
                      <div className="flex items-center gap-3">
                        <h3 className="text-lg font-semibold text-white">
                          {chain.chain_name || chain.chain_id}
                        </h3>
                        {chain.paused ? (
                          <span className="badge-warning">Paused</span>
                        ) : (
                          <Tooltip.Root>
                            <Tooltip.Trigger asChild>
                              <span className={getHealthBadgeClass(overallHealth)}>
                                <span className={getHealthStatusDotClass(overallHealth)}></span>
                                {formatHealthStatus(overallHealth)}
                              </span>
                            </Tooltip.Trigger>
                            <Tooltip.Portal>
                              <Tooltip.Content
                                className="z-50 rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-xs text-slate-200 shadow-xl"
                                sideOffset={5}
                              >
                                <div className="space-y-2">
                                  <div className="font-semibold text-white">System Health</div>
                                  <div className="space-y-1.5">
                                    <div className="flex items-center justify-between gap-4">
                                      <span className="text-slate-400">RPC:</span>
                                      <span className={getHealthBadgeClass(st?.rpc_health?.status)}>
                                        {formatHealthStatus(st?.rpc_health?.status)}
                                      </span>
                                    </div>
                                    {st?.rpc_health?.message && (
                                      <div className="pl-2 text-[10px] text-slate-500">
                                        {st.rpc_health.message}
                                      </div>
                                    )}
                                    <div className="flex items-center justify-between gap-4">
                                      <span className="text-slate-400">Queue:</span>
                                      <span className={getHealthBadgeClass(st?.queue_health?.status)}>
                                        {formatHealthStatus(st?.queue_health?.status)}
                                      </span>
                                    </div>
                                    {st?.queue_health?.message && (
                                      <div className="pl-2 text-[10px] text-slate-500">
                                        {st.queue_health.message}
                                      </div>
                                    )}
                                    <div className="flex items-center justify-between gap-4">
                                      <span className="text-slate-400">Deployment:</span>
                                      <span className={getHealthBadgeClass(st?.deployment_health?.status)}>
                                        {formatHealthStatus(st?.deployment_health?.status)}
                                      </span>
                                    </div>
                                    {st?.deployment_health?.message && (
                                      <div className="pl-2 text-[10px] text-slate-500">
                                        {st.deployment_health.message}
                                      </div>
                                    )}
                                  </div>
                                  <div className="mt-2 border-t border-slate-700 pt-2 text-[10px] text-slate-500">
                                    Updated {formatTimestamp(st?.health?.updated_at)}
                                  </div>
                                </div>
                                <Tooltip.Arrow className="fill-slate-700" />
                              </Tooltip.Content>
                            </Tooltip.Portal>
                          </Tooltip.Root>
                        )}
                      </div>
                      <p className="mt-1 text-xs text-slate-500">{chain.chain_id}</p>
                      {chain.notes && <p className="mt-2 text-sm text-slate-400">{chain.notes}</p>}
                    </div>

                    <button
                      onClick={() => {
                        setSelectedChain(chain)
                        setEditDialogOpen(true)
                      }}
                      className="btn-ghost p-2 opacity-0 transition-opacity group-hover:opacity-100"
                    >
                      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                        />
                      </svg>
                    </button>
                  </div>

                  {/* Stats */}
                  <div className="mt-4 grid grid-cols-3 gap-4">
                    <div>
                      <p className="text-xs text-slate-500">Indexed</p>
                      <p className="mt-1 font-mono text-lg font-semibold text-white">
                        {formatNumber(st?.last_indexed ?? 0)}
                      </p>
                    </div>
                    <div>
                      <p className="text-xs text-slate-500">Head</p>
                      <p className="mt-1 font-mono text-lg font-semibold text-white">
                        {formatNumber(st?.head ?? 0)}
                      </p>
                    </div>
                    <div>
                      <p className="text-xs text-slate-500">Backlog</p>
                      <p className="mt-1 font-mono text-lg font-semibold text-white">
                        {formatNumber(backlog)}
                      </p>
                    </div>
                  </div>

                  {/* Health Status Breakdown */}
                  <div className="mt-4 rounded-lg border border-slate-800 bg-slate-900/30 p-3">
                    <div className="mb-2 text-xs font-semibold text-slate-400">Health Status</div>
                    <div className="space-y-2 text-xs">
                      <div className="flex items-center justify-between">
                        <span className="text-slate-500">RPC Endpoints</span>
                        <span className={getHealthBadgeClass(st?.rpc_health?.status)}>
                          {formatHealthStatus(st?.rpc_health?.status)}
                        </span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span className="text-slate-500">Queue System</span>
                        <span className={getHealthBadgeClass(st?.queue_health?.status)}>
                          {formatHealthStatus(st?.queue_health?.status)}
                        </span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span className="text-slate-500">Deployment</span>
                        <span className={getHealthBadgeClass(st?.deployment_health?.status)}>
                          {formatHealthStatus(st?.deployment_health?.status)}
                        </span>
                      </div>
                    </div>
                  </div>

                  {/* Queue details */}
                  <div className="mt-4 rounded-lg border border-slate-800 bg-slate-900/30 p-3">
                    <div className="mb-2 text-xs font-semibold text-slate-400">Queue Metrics</div>
                    <div className="grid grid-cols-2 gap-3 text-xs">
                      <div className="flex justify-between">
                        <span className="text-slate-500">Workflow</span>
                        <span className="font-mono text-slate-300">
                          {formatNumber(st?.queue.pending_workflow ?? 0)}
                        </span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-slate-500">Activity</span>
                        <span className="font-mono text-slate-300">
                          {formatNumber(st?.queue.pending_activity ?? 0)}
                        </span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-slate-500">Pollers</span>
                        <span className="font-mono text-slate-300">{st?.queue.pollers ?? 0}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-slate-500">Age</span>
                        <span className="font-mono text-slate-300">
                          {secondsToFriendly(st?.queue.backlog_age_secs ?? 0)}
                        </span>
                      </div>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="mt-4 flex flex-wrap gap-2">
                    <button
                      onClick={() => handleTogglePause(chain)}
                      className="btn-secondary text-xs"
                    >
                      {chain.paused ? (
                        <>
                          <svg className="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
                            <path d="M6.3 2.841A1.5 1.5 0 004 4.11V15.89a1.5 1.5 0 002.3 1.269l9.344-5.89a1.5 1.5 0 000-2.538L6.3 2.84z" />
                          </svg>
                          Resume
                        </>
                      ) : (
                        <>
                          <svg className="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
                            <path d="M5.75 3a.75.75 0 00-.75.75v12.5c0 .414.336.75.75.75h1.5a.75.75 0 00.75-.75V3.75A.75.75 0 007.25 3h-1.5zM12.75 3a.75.75 0 00-.75.75v12.5c0 .414.336.75.75.75h1.5a.75.75 0 00.75-.75V3.75a.75.75 0 00-.75-.75h-1.5z" />
                          </svg>
                          Pause
                        </>
                      )}
                    </button>
                    <button onClick={() => handleHeadScan(chain)} className="btn-secondary text-xs">
                      Head Scan
                    </button>
                    <button onClick={() => handleGapScan(chain)} className="btn-secondary text-xs">
                      Gap Scan
                    </button>
                    <button
                      onClick={() => {
                        setSelectedChain(chain)
                        setReindexDialogOpen(true)
                      }}
                      className="btn-secondary text-xs"
                    >
                      Reindex
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        )}

        {/* Dialogs */}
        <CreateChainDialog
          open={createDialogOpen}
          onOpenChange={setCreateDialogOpen}
          onSubmit={handleCreateChain}
          saving={saving}
        />

        <EditChainDialog
          open={editDialogOpen}
          onOpenChange={setEditDialogOpen}
          chain={selectedChain}
          onSave={handleEditChain}
          saving={saving}
        />

        <ReindexDialog
          open={reindexDialogOpen}
          onOpenChange={setReindexDialogOpen}
          chain={selectedChain}
          onSubmit={handleReindex}
          saving={saving}
        />
      </div>
    </Tooltip.Provider>
  )
}

// Edit Chain Dialog Component
function EditChainDialog({
  open,
  onOpenChange,
  chain,
  onSave,
  saving,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  chain: ChainRow | null
  onSave: (updates: Partial<ChainRow>) => Promise<void>
  saving: boolean
}) {
  const [image, setImage] = useState('')
  const [minReplicas, setMinReplicas] = useState(1)
  const [maxReplicas, setMaxReplicas] = useState(1)
  const [notes, setNotes] = useState('')

  useEffect(() => {
    if (chain) {
      setImage(chain.image || '')
      setMinReplicas(chain.min_replicas)
      setMaxReplicas(chain.max_replicas)
      setNotes(chain.notes || '')
    }
  }, [chain])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    await onSave({
      image,
      min_replicas: Number(minReplicas),
      max_replicas: Number(maxReplicas),
      notes,
    })
  }

  return (
    <Dialog.Root open={open && !!chain} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-2xl">
          <Dialog.Title className="text-xl font-bold text-white">Edit Chain</Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-slate-400">
            Update configuration for {chain?.chain_name || chain?.chain_id}
          </Dialog.Description>

          <form onSubmit={handleSubmit} className="mt-6 space-y-4">
            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">
                Container Image
              </label>
              <input
                type="text"
                value={image}
                onChange={(e) => setImage(e.target.value)}
                className="input"
                placeholder="ghcr.io/..."
              />
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">
                  Min Replicas
                </label>
                <input
                  type="number"
                  min={1}
                  value={minReplicas}
                  onChange={(e) => setMinReplicas(Number(e.target.value))}
                  className="input"
                />
              </div>
              <div>
                <label className="mb-2 block text-sm font-medium text-slate-300">
                  Max Replicas
                </label>
                <input
                  type="number"
                  min={minReplicas}
                  value={maxReplicas}
                  onChange={(e) => setMaxReplicas(Number(e.target.value))}
                  className="input"
                />
              </div>
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-slate-300">Notes</label>
              <textarea
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                className="textarea"
                rows={3}
              />
            </div>

            <div className="flex justify-end gap-3 pt-4">
              <Dialog.Close asChild>
                <button type="button" className="btn-secondary" disabled={saving}>
                  Cancel
                </button>
              </Dialog.Close>
              <button type="submit" className="btn" disabled={saving || !image.trim()}>
                {saving ? 'Saving...' : 'Save Changes'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

// Reindex Dialog Component
function ReindexDialog({
  open,
  onOpenChange,
  chain,
  onSubmit,
  saving,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  chain: ChainRow | null
  onSubmit: (payload: ReindexPayload) => Promise<void>
  saving: boolean
}) {
  const [height, setHeight] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [error, setError] = useState('')

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

    await onSubmit(payload)
  }

  return (
    <Dialog.Root open={open && !!chain} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-2xl">
          <Dialog.Title className="text-xl font-bold text-white">Reindex Blocks</Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-slate-400">
            Queue reindex workflows for {chain?.chain_name || chain?.chain_id}
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