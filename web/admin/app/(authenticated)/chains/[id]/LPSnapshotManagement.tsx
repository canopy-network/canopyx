'use client'

import { useCallback, useEffect, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { apiFetch } from '../../../lib/api'
import { useToast } from '../../../components/ToastProvider'

type LPScheduleStatus = {
  exists: boolean
  schedule_id: string
  chain_id: number
  paused: boolean
  pause_note?: string
  next_run_time?: string
  last_run_time?: string
  running_count: number
}

type LPSnapshotManagementProps = {
  chainId: string
  isDeleted: boolean
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

export default function LPSnapshotManagement({ chainId, isDeleted }: LPSnapshotManagementProps) {
  const { notify } = useToast()
  const [status, setStatus] = useState<LPScheduleStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)
  const [backfillDialogOpen, setBackfillDialogOpen] = useState(false)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)

  const loadStatus = useCallback(async () => {
    try {
      const res = await apiFetch(`/api/chains/${chainId}/lp-schedule`)
      if (res.ok) {
        const data: LPScheduleStatus = await res.json()
        setStatus(data)
      }
    } catch (err) {
      console.error('Failed to load LP schedule status:', err)
    } finally {
      setLoading(false)
    }
  }, [chainId])

  useEffect(() => {
    loadStatus()
    const interval = setInterval(loadStatus, 30_000)
    return () => clearInterval(interval)
  }, [loadStatus])

  const handleCreate = async (withBackfill: boolean, startDate?: string, endDate?: string) => {
    setActionLoading(true)
    try {
      const body: Record<string, unknown> = {}
      if (withBackfill) {
        body.backfill = startDate && endDate ? { start: startDate, end: endDate } : {}
      }
      const res = await apiFetch(`/api/chains/${chainId}/lp-schedule`, {
        method: 'POST',
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed to create schedule' }))
        throw new Error(err.error)
      }
      notify('LP snapshot schedule created')
      await loadStatus()
      setCreateDialogOpen(false)
    } catch (err: any) {
      notify(err.message || 'Failed to create schedule', 'error')
    } finally {
      setActionLoading(false)
    }
  }

  const handlePause = async () => {
    setActionLoading(true)
    try {
      const res = await apiFetch(`/api/chains/${chainId}/lp-schedule/pause`, { method: 'POST' })
      if (!res.ok) throw new Error('Failed to pause schedule')
      notify('LP snapshot schedule paused')
      await loadStatus()
    } catch (err: any) {
      notify(err.message || 'Failed to pause schedule', 'error')
    } finally {
      setActionLoading(false)
    }
  }

  const handleUnpause = async () => {
    setActionLoading(true)
    try {
      const res = await apiFetch(`/api/chains/${chainId}/lp-schedule/unpause`, { method: 'POST' })
      if (!res.ok) throw new Error('Failed to unpause schedule')
      notify('LP snapshot schedule resumed')
      await loadStatus()
    } catch (err: any) {
      notify(err.message || 'Failed to unpause schedule', 'error')
    } finally {
      setActionLoading(false)
    }
  }

  const handleDelete = async () => {
    if (!confirm('Are you sure you want to delete the LP snapshot schedule?')) return
    setActionLoading(true)
    try {
      const res = await apiFetch(`/api/chains/${chainId}/lp-schedule`, { method: 'DELETE' })
      if (!res.ok) throw new Error('Failed to delete schedule')
      notify('LP snapshot schedule deleted')
      await loadStatus()
    } catch (err: any) {
      notify(err.message || 'Failed to delete schedule', 'error')
    } finally {
      setActionLoading(false)
    }
  }

  const handleBackfill = async (startDate: string, endDate: string) => {
    setActionLoading(true)
    try {
      const res = await apiFetch(`/api/chains/${chainId}/lp-snapshots/backfill`, {
        method: 'POST',
        body: JSON.stringify({ start: startDate, end: endDate }),
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed to trigger backfill' }))
        throw new Error(err.error)
      }
      const data = await res.json()
      notify(`Backfill triggered: ${data.start_date} to ${data.end_date}`)
      setBackfillDialogOpen(false)
      await loadStatus()
    } catch (err: any) {
      notify(err.message || 'Failed to trigger backfill', 'error')
    } finally {
      setActionLoading(false)
    }
  }

  if (loading) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">LP Snapshot Schedule</h3>
        </div>
        <div className="flex items-center justify-center py-8">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-slate-700 border-t-indigo-500"></div>
        </div>
      </div>
    )
  }

  return (
    <div className="card">
      <div className="card-header flex items-center justify-between">
        <h3 className="card-title">LP Snapshot Schedule</h3>
        {status?.exists && (
          <span className={status.paused ? 'badge-warning' : 'badge-success'}>
            {status.paused ? 'Paused' : 'Active'}
          </span>
        )}
      </div>

      {!status?.exists ? (
        // Schedule not created
        <div className="space-y-4">
          <p className="text-sm text-slate-400">
            LP snapshot schedule is not configured for this chain. Create a schedule to automatically
            capture hourly LP position snapshots.
          </p>
          <button
            onClick={() => setCreateDialogOpen(true)}
            className="btn"
            disabled={isDeleted || actionLoading}
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
            Create Schedule
          </button>
        </div>
      ) : (
        // Schedule exists
        <div className="space-y-4">
          {/* Schedule Info */}
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            <div>
              <p className="text-xs text-slate-400">Schedule ID</p>
              <p className="mt-1 font-mono text-sm text-slate-200 truncate" title={status.schedule_id}>
                {status.schedule_id}
              </p>
            </div>
            <div>
              <p className="text-xs text-slate-400">Next Run</p>
              <p className="mt-1 text-sm text-slate-200">
                {status.paused ? 'Paused' : formatTimestamp(status.next_run_time)}
              </p>
            </div>
            <div>
              <p className="text-xs text-slate-400">Last Run</p>
              <p className="mt-1 text-sm text-slate-200">{formatTimestamp(status.last_run_time)}</p>
            </div>
            <div>
              <p className="text-xs text-slate-400">Running Workflows</p>
              <p className="mt-1 text-sm text-slate-200">{status.running_count}</p>
            </div>
          </div>

          {status.paused && status.pause_note && (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3">
              <p className="text-xs text-amber-200">
                <span className="font-semibold">Pause Note:</span> {status.pause_note}
              </p>
            </div>
          )}

          {/* Actions */}
          <div className="flex flex-wrap gap-2 border-t border-slate-800 pt-4">
            {status.paused ? (
              <button
                onClick={handleUnpause}
                className="btn text-sm"
                disabled={isDeleted || actionLoading}
              >
                <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M6.3 2.841A1.5 1.5 0 004 4.11V15.89a1.5 1.5 0 002.3 1.269l9.344-5.89a1.5 1.5 0 000-2.538L6.3 2.84z" />
                </svg>
                Resume
              </button>
            ) : (
              <button
                onClick={handlePause}
                className="btn-secondary text-sm"
                disabled={isDeleted || actionLoading}
              >
                <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M5.75 3a.75.75 0 00-.75.75v12.5c0 .414.336.75.75.75h1.5a.75.75 0 00.75-.75V3.75A.75.75 0 007.25 3h-1.5zM12.75 3a.75.75 0 00-.75.75v12.5c0 .414.336.75.75.75h1.5a.75.75 0 00.75-.75V3.75a.75.75 0 00-.75-.75h-1.5z" />
                </svg>
                Pause
              </button>
            )}
            <button
              onClick={() => setBackfillDialogOpen(true)}
              className="btn-secondary text-sm"
              disabled={isDeleted || actionLoading}
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              Backfill
            </button>
            <button
              onClick={handleDelete}
              className="btn-danger text-sm"
              disabled={isDeleted || actionLoading}
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
              Delete
            </button>
          </div>
        </div>
      )}

      {/* Create Schedule Dialog */}
      <CreateScheduleDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        onSubmit={handleCreate}
        loading={actionLoading}
      />

      {/* Backfill Dialog */}
      <BackfillDialog
        open={backfillDialogOpen}
        onOpenChange={setBackfillDialogOpen}
        onSubmit={handleBackfill}
        loading={actionLoading}
      />
    </div>
  )
}

// Create Schedule Dialog
function CreateScheduleDialog({
  open,
  onOpenChange,
  onSubmit,
  loading,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (withBackfill: boolean, startDate?: string, endDate?: string) => Promise<void>
  loading: boolean
}) {
  const [withBackfill, setWithBackfill] = useState(false)
  const [useCustomDates, setUseCustomDates] = useState(false)
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')

  useEffect(() => {
    if (!open) {
      setWithBackfill(false)
      setUseCustomDates(false)
      setStartDate('')
      setEndDate('')
    }
  }, [open])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (withBackfill && useCustomDates) {
      onSubmit(true, startDate, endDate)
    } else if (withBackfill) {
      onSubmit(true)
    } else {
      onSubmit(false)
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-2xl">
          <Dialog.Title className="text-xl font-bold text-white">Create LP Snapshot Schedule</Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-slate-400">
            Create an hourly schedule to capture LP position snapshots automatically.
          </Dialog.Description>

          <form onSubmit={handleSubmit} className="mt-6 space-y-4">
            {/* Backfill Option */}
            <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4">
              <label className="flex cursor-pointer items-start gap-3">
                <input
                  type="checkbox"
                  checked={withBackfill}
                  onChange={(e) => setWithBackfill(e.target.checked)}
                  className="mt-0.5 h-4 w-4 cursor-pointer rounded border-slate-600 bg-slate-800 text-indigo-500 focus:ring-2 focus:ring-indigo-500/20"
                />
                <div className="flex-1">
                  <div className="text-sm font-semibold text-slate-200">Include Historical Backfill</div>
                  <div className="mt-1 text-xs text-slate-400">
                    Generate snapshots for historical data in addition to future hourly runs.
                  </div>
                </div>
              </label>
            </div>

            {withBackfill && (
              <div className="space-y-4 rounded-lg border border-slate-800 bg-slate-900/50 p-4">
                <label className="flex cursor-pointer items-start gap-3">
                  <input
                    type="checkbox"
                    checked={useCustomDates}
                    onChange={(e) => setUseCustomDates(e.target.checked)}
                    className="mt-0.5 h-4 w-4 cursor-pointer rounded border-slate-600 bg-slate-800 text-indigo-500 focus:ring-2 focus:ring-indigo-500/20"
                  />
                  <div className="flex-1">
                    <div className="text-sm font-semibold text-slate-200">Custom Date Range</div>
                    <div className="mt-1 text-xs text-slate-400">
                      By default, backfill covers from block 1 to current indexed height.
                    </div>
                  </div>
                </label>

                {useCustomDates && (
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="mb-2 block text-sm font-medium text-slate-300">Start Date</label>
                      <input
                        type="date"
                        value={startDate}
                        onChange={(e) => setStartDate(e.target.value)}
                        className="input"
                        required
                      />
                    </div>
                    <div>
                      <label className="mb-2 block text-sm font-medium text-slate-300">End Date</label>
                      <input
                        type="date"
                        value={endDate}
                        onChange={(e) => setEndDate(e.target.value)}
                        className="input"
                        required
                      />
                    </div>
                  </div>
                )}
              </div>
            )}

            <div className="flex justify-end gap-3 pt-4">
              <Dialog.Close asChild>
                <button type="button" className="btn-secondary" disabled={loading}>
                  Cancel
                </button>
              </Dialog.Close>
              <button type="submit" className="btn" disabled={loading}>
                {loading ? 'Creating...' : 'Create Schedule'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

// Backfill Dialog
function BackfillDialog({
  open,
  onOpenChange,
  onSubmit,
  loading,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (startDate: string, endDate: string) => Promise<void>
  loading: boolean
}) {
  const [useCustomDates, setUseCustomDates] = useState(false)
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')

  useEffect(() => {
    if (!open) {
      setUseCustomDates(false)
      setStartDate('')
      setEndDate('')
    }
  }, [open])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (useCustomDates && startDate && endDate) {
      onSubmit(startDate, endDate)
    } else {
      // Empty dates = auto-calculate from block heights
      onSubmit('', '')
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-2xl">
          <Dialog.Title className="text-xl font-bold text-white">Trigger LP Snapshot Backfill</Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-slate-400">
            Generate LP position snapshots for historical data. This will queue snapshot workflows for each hour in the specified range.
          </Dialog.Description>

          <form onSubmit={handleSubmit} className="mt-6 space-y-4">
            <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4">
              <label className="flex cursor-pointer items-start gap-3">
                <input
                  type="checkbox"
                  checked={useCustomDates}
                  onChange={(e) => setUseCustomDates(e.target.checked)}
                  className="mt-0.5 h-4 w-4 cursor-pointer rounded border-slate-600 bg-slate-800 text-indigo-500 focus:ring-2 focus:ring-indigo-500/20"
                />
                <div className="flex-1">
                  <div className="text-sm font-semibold text-slate-200">Custom Date Range</div>
                  <div className="mt-1 text-xs text-slate-400">
                    By default, backfill covers from block 1 to current indexed height.
                  </div>
                </div>
              </label>
            </div>

            {useCustomDates && (
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="mb-2 block text-sm font-medium text-slate-300">Start Date</label>
                  <input
                    type="date"
                    value={startDate}
                    onChange={(e) => setStartDate(e.target.value)}
                    className="input"
                    required
                  />
                </div>
                <div>
                  <label className="mb-2 block text-sm font-medium text-slate-300">End Date</label>
                  <input
                    type="date"
                    value={endDate}
                    onChange={(e) => setEndDate(e.target.value)}
                    className="input"
                    required
                  />
                </div>
              </div>
            )}

            <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3">
              <p className="text-xs text-amber-200">
                <span className="font-semibold">Note:</span> Backfill can generate many workflows. For large date ranges, this may take significant time to complete.
              </p>
            </div>

            <div className="flex justify-end gap-3 pt-4">
              <Dialog.Close asChild>
                <button type="button" className="btn-secondary" disabled={loading}>
                  Cancel
                </button>
              </Dialog.Close>
              <button type="submit" className="btn" disabled={loading}>
                {loading ? 'Triggering...' : 'Trigger Backfill'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}