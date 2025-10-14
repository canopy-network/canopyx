'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { apiFetch } from '../../lib/api'
import { queueHealth } from '../../lib/constants'

type ChainStatus = {
  chain_id: string
  chain_name: string
  image: string
  paused: boolean
  deleted: boolean
  min_replicas: number
  max_replicas: number
  last_indexed: number
  head: number
  queue: {
    pending_workflow: number
    pending_activity: number
    backlog_age_secs: number
    pollers: number
  }
}

type ChainStatusMap = Record<string, ChainStatus>

function formatNumber(n: number | undefined) {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return n.toLocaleString()
}

export default function DashboardPage() {
  const [status, setStatus] = useState<ChainStatusMap>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')

  const loadStatus = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await apiFetch('/api/chains/status')
      if (!res.ok) throw new Error('Failed to load status')
      const data: ChainStatusMap = await res.json()
      setStatus(data)
    } catch (err) {
      console.error(err)
      setError('Unable to load dashboard data')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadStatus()
    const interval = setInterval(loadStatus, 15_000)
    return () => clearInterval(interval)
  }, [loadStatus])

  const chains = Object.values(status)
  const totalChains = chains.length
  const activeChains = chains.filter((c) => !c.paused && !c.deleted).length
  const pausedChains = chains.filter((c) => c.paused).length

  const totalBacklog = chains.reduce(
    (sum, c) => sum + c.queue.pending_workflow + c.queue.pending_activity,
    0
  )

  const criticalChains = chains.filter((c) => {
    const backlog = c.queue.pending_workflow + c.queue.pending_activity
    return queueHealth(backlog).level === 'critical'
  }).length

  const warningChains = chains.filter((c) => {
    const backlog = c.queue.pending_workflow + c.queue.pending_activity
    return queueHealth(backlog).level === 'warning'
  }).length

  const avgIndexed = chains.length
    ? Math.floor(chains.reduce((sum, c) => sum + c.last_indexed, 0) / chains.length)
    : 0

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
        <button
          onClick={loadStatus}
          className="btn-secondary"
          disabled={loading}
        >
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

      {/* Stats Grid */}
      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-4">
        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-slate-400">Total Chains</p>
              <p className="mt-2 text-3xl font-bold text-white">{totalChains}</p>
              <p className="mt-1 text-xs text-slate-500">
                {activeChains} active • {pausedChains} paused
              </p>
            </div>
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-indigo-500/10">
              <svg
                className="h-6 w-6 text-indigo-400"
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
            </div>
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-slate-400">Queue Backlog</p>
              <p className="mt-2 text-3xl font-bold text-white">{formatNumber(totalBacklog)}</p>
              <p className="mt-1 text-xs text-slate-500">Pending tasks across all chains</p>
            </div>
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-amber-500/10">
              <svg
                className="h-6 w-6 text-amber-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-slate-400">Health Status</p>
              <p className="mt-2 text-3xl font-bold text-white">
                {chains.length - criticalChains - warningChains}
              </p>
              <p className="mt-1 text-xs text-slate-500">
                {criticalChains > 0 && (
                  <span className="text-rose-400">{criticalChains} critical • </span>
                )}
                {warningChains > 0 && (
                  <span className="text-amber-400">{warningChains} warning</span>
                )}
                {criticalChains === 0 && warningChains === 0 && (
                  <span className="text-emerald-400">All systems healthy</span>
                )}
              </p>
            </div>
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-emerald-500/10">
              <svg
                className="h-6 w-6 text-emerald-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-slate-400">Avg Indexed</p>
              <p className="mt-2 text-3xl font-bold text-white">{formatNumber(avgIndexed)}</p>
              <p className="mt-1 text-xs text-slate-500">Average block height</p>
            </div>
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-purple-500/10">
              <svg
                className="h-6 w-6 text-purple-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z"
                />
              </svg>
            </div>
          </div>
        </div>
      </div>

      {/* Chains Overview */}
      <div className="card">
        <div className="card-header">
          <h2 className="card-title">Chains Overview</h2>
          <Link href="/chains" className="link text-sm">
            View all chains →
          </Link>
        </div>

        {chains.length === 0 ? (
          <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-12 text-center">
            <svg
              className="mx-auto h-12 w-12 text-slate-600"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
              />
            </svg>
            <h3 className="mt-4 text-lg font-medium text-slate-300">No chains configured</h3>
            <p className="mt-2 text-sm text-slate-500">Get started by adding your first chain</p>
            <Link href="/chains" className="btn mt-6">
              Add Chain
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-slate-800">
            <table className="table">
              <thead>
                <tr>
                  <th>Chain</th>
                  <th>Status</th>
                  <th>Indexed</th>
                  <th>Head</th>
                  <th>Backlog</th>
                  <th>Replicas</th>
                </tr>
              </thead>
              <tbody>
                {chains.slice(0, 10).map((chain) => {
                  const backlog =
                    chain.queue.pending_workflow + chain.queue.pending_activity
                  const health = queueHealth(backlog)
                  return (
                    <tr key={chain.chain_id}>
                      <td>
                        <div>
                          <div className="font-medium text-white">
                            {chain.chain_name || chain.chain_id}
                          </div>
                          <div className="text-xs text-slate-500">{chain.chain_id}</div>
                        </div>
                      </td>
                      <td>
                        {chain.paused ? (
                          <span className="badge-warning">Paused</span>
                        ) : chain.deleted ? (
                          <span className="badge-danger">Deleted</span>
                        ) : (
                          <span
                            className={
                              health.level === 'critical'
                                ? 'badge-danger'
                                : health.level === 'warning'
                                ? 'badge-warning'
                                : 'badge-success'
                            }
                          >
                            <span
                              className={
                                health.level === 'critical'
                                  ? 'status-dot-danger'
                                  : health.level === 'warning'
                                  ? 'status-dot-warning'
                                  : 'status-dot-success'
                              }
                            ></span>
                            {health.label}
                          </span>
                        )}
                      </td>
                      <td className="font-mono text-sm">{formatNumber(chain.last_indexed)}</td>
                      <td className="font-mono text-sm">{formatNumber(chain.head)}</td>
                      <td className="font-mono text-sm">{formatNumber(backlog)}</td>
                      <td className="text-sm text-slate-400">
                        {chain.min_replicas} - {chain.max_replicas}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}