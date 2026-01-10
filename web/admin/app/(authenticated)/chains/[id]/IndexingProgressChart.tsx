'use client'

import { useEffect, useState } from 'react'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import { apiFetch } from '../../../lib/api'

// Timing metric options for the selector - Total is first (default)
const TIMING_METRICS = [
  { key: 'avg_total_time_ms', label: 'Total Processing Time', color: '#8b5cf6' },
  { key: 'avg_fetch_block_ms', label: 'Fetch Block (RPC)', color: '#f59e0b' },
  { key: 'avg_prepare_index_ms', label: 'Prepare Index', color: '#8b5cf6' },
  { key: 'avg_index_accounts_ms', label: 'Index Accounts', color: '#10b981' },
  { key: 'avg_index_committees_ms', label: 'Index Committees', color: '#3b82f6' },
  { key: 'avg_index_dex_batch_ms', label: 'Index DEX Batch', color: '#ec4899' },
  { key: 'avg_index_dex_prices_ms', label: 'Index DEX Prices', color: '#14b8a6' },
  { key: 'avg_index_events_ms', label: 'Index Events', color: '#f97316' },
  { key: 'avg_index_orders_ms', label: 'Index Orders', color: '#a855f7' },
  { key: 'avg_index_params_ms', label: 'Index Params', color: '#06b6d4' },
  { key: 'avg_index_pools_ms', label: 'Index Pools', color: '#84cc16' },
  { key: 'avg_index_supply_ms', label: 'Index Supply', color: '#eab308' },
  { key: 'avg_index_transactions_ms', label: 'Index Transactions', color: '#ef4444' },
  { key: 'avg_index_validators_ms', label: 'Index Validators', color: '#22c55e' },
  { key: 'avg_save_block_ms', label: 'Save Block', color: '#6366f1' },
  { key: 'avg_save_block_summary_ms', label: 'Save Block Summary', color: '#d946ef' },
] as const

type TimingMetricKey = typeof TIMING_METRICS[number]['key']

type ProgressDataPoint = {
  time: string
  height: number
  avg_total_time_ms: number
  blocks_indexed: number
  velocity: number
  avg_fetch_block_ms: number
  avg_prepare_index_ms: number
  avg_index_accounts_ms: number
  avg_index_committees_ms: number
  avg_index_dex_batch_ms: number
  avg_index_dex_prices_ms: number
  avg_index_events_ms: number
  avg_index_orders_ms: number
  avg_index_params_ms: number
  avg_index_pools_ms: number
  avg_index_supply_ms: number
  avg_index_transactions_ms: number
  avg_index_validators_ms: number
  avg_save_block_ms: number
  avg_save_block_summary_ms: number
}

type IndexingProgressChartProps = {
  chainId: string
}

export default function IndexingProgressChart({ chainId }: IndexingProgressChartProps) {
  const [data, setData] = useState<ProgressDataPoint[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')
  const [timeWindow, setTimeWindow] = useState<number>(24) // hours
  const [selectedTiming, setSelectedTiming] = useState<TimingMetricKey>('avg_total_time_ms') // Default to Total

  useEffect(() => {
    const fetchData = async () => {
      setLoading(true)
      setError('')
      try {
        const res = await apiFetch(`/api/chains/${chainId}/progress-history?hours=${timeWindow}`)
        if (!res.ok) {
          throw new Error('Failed to fetch progress history')
        }
        const points: ProgressDataPoint[] = await res.json()
        setData(points)
      } catch (err: any) {
        console.error('Failed to fetch progress history:', err)
        setError(err.message || 'Failed to load data')
      } finally {
        setLoading(false)
      }
    }

    fetchData()
    const interval = setInterval(fetchData, 30_000)
    return () => clearInterval(interval)
  }, [chainId, timeWindow])

  // Format data for display
  const chartData = data.map((point) => ({
    time: new Date(point.time).toLocaleTimeString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }),
    height: point.height,
    velocity: Math.round(point.velocity * 10) / 10,
    blocksIndexed: point.blocks_indexed,
    // All timing values for the selector
    selectedTiming: Math.round((point[selectedTiming] || 0) * 10) / 10,
    // Keep total time separately for summary
    totalTime: Math.round(point.avg_total_time_ms * 10) / 10,
  }))

  // Get the selected timing metric info
  const selectedMetric = TIMING_METRICS.find(m => m.key === selectedTiming)

  // Calculate summary stats (fixed, don't depend on selector)
  const totalBlocksIndexed = data.reduce((sum, d) => sum + d.blocks_indexed, 0)
  const avgVelocity = chartData.length > 0
    ? chartData.reduce((sum, d) => sum + d.velocity, 0) / chartData.length
    : 0
  const avgTotalTime = chartData.length > 0
    ? chartData.reduce((sum, d) => sum + d.totalTime, 0) / chartData.length
    : 0
  const currentHeight = chartData.length > 0 ? chartData[chartData.length - 1]?.height : 0

  if (loading && data.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Indexing Progress</h3>
        </div>
        <div className="flex items-center justify-center py-12">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
            <p className="text-slate-400">Loading chart data...</p>
          </div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Indexing Progress</h3>
        </div>
        <div className="rounded-lg border border-rose-500/50 bg-rose-500/10 p-4 text-rose-200">
          <p className="font-semibold">Error Loading Chart Data</p>
          <p className="mt-1 text-sm">{error}</p>
        </div>
      </div>
    )
  }

  if (data.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Indexing Progress</h3>
        </div>
        <div className="flex items-center justify-center py-12 text-slate-500">
          <div className="text-center">
            <svg className="mx-auto h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
            </svg>
            <p className="mt-4 text-sm">No indexing data available</p>
            <p className="mt-1 text-xs text-slate-600">Data will appear once indexing begins</p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="card">
      <div className="card-header flex items-center justify-between">
        <h3 className="card-title">Indexing Progress</h3>
        <div className="flex items-center gap-2">
          <label className="text-xs text-slate-400">Time Window:</label>
          <select
            value={timeWindow}
            onChange={(e) => setTimeWindow(Number(e.target.value))}
            className="input w-auto px-2 py-1 text-xs"
          >
            <option value={1}>1 Hour</option>
            <option value={6}>6 Hours</option>
            <option value={24}>24 Hours</option>
            <option value={72}>3 Days</option>
            <option value={168}>7 Days</option>
          </select>
        </div>
      </div>

      <div className="space-y-6 p-4">
        {/* Summary Stats - At the top */}
        <div className="grid gap-4 md:grid-cols-4 rounded-lg bg-slate-900/50 p-4">
          <div className="text-center">
            <p className="text-xs text-slate-400">Current Height</p>
            <p className="mt-1 font-mono text-lg font-bold text-indigo-400">
              {currentHeight.toLocaleString() || '—'}
            </p>
          </div>
          <div className="text-center">
            <p className="text-xs text-slate-400">Blocks Indexed</p>
            <p className="mt-1 font-mono text-lg font-bold text-blue-400">
              {totalBlocksIndexed.toLocaleString()}
            </p>
          </div>
          <div className="text-center">
            <p className="text-xs text-slate-400">Avg Velocity</p>
            <p className="mt-1 font-mono text-lg font-bold text-green-400">
              {avgVelocity.toFixed(1)} bl/min
            </p>
          </div>
          <div className="text-center">
            <p className="text-xs text-slate-400">Avg Processing Time</p>
            <p className="mt-1 font-mono text-lg font-bold text-purple-400">
              {avgTotalTime.toFixed(1)}ms
            </p>
          </div>
        </div>

        {/* Velocity Chart */}
        <div>
          <h4 className="mb-2 text-sm font-medium text-slate-300">Indexing Velocity (blocks/min)</h4>
          <ResponsiveContainer width="100%" height={150}>
            <LineChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
              <XAxis
                dataKey="time"
                stroke="#64748b"
                style={{ fontSize: '11px' }}
                tick={{ fill: '#94a3b8' }}
              />
              <YAxis
                stroke="#64748b"
                style={{ fontSize: '11px' }}
                tick={{ fill: '#94a3b8' }}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: '#1e293b',
                  border: '1px solid #475569',
                  borderRadius: '8px',
                  color: '#e2e8f0',
                }}
                labelStyle={{ color: '#94a3b8' }}
              />
              <Line
                type="monotone"
                dataKey="velocity"
                stroke="#10b981"
                strokeWidth={2}
                dot={chartData.length < 50}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>

        {/* Combined Timing Chart with Selector (Total + Individual metrics) */}
        <div>
          <div className="mb-2 flex items-center justify-between">
            <h4 className="text-sm font-medium text-slate-300">Processing Time (ms)</h4>
            <select
              value={selectedTiming}
              onChange={(e) => setSelectedTiming(e.target.value as TimingMetricKey)}
              className="input w-auto px-2 py-1 text-xs"
            >
              {TIMING_METRICS.map((metric) => (
                <option key={metric.key} value={metric.key}>
                  {metric.label}
                </option>
              ))}
            </select>
          </div>
          <ResponsiveContainer width="100%" height={150}>
            <LineChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
              <XAxis
                dataKey="time"
                stroke="#64748b"
                style={{ fontSize: '11px' }}
                tick={{ fill: '#94a3b8' }}
              />
              <YAxis
                stroke="#64748b"
                style={{ fontSize: '11px' }}
                tick={{ fill: '#94a3b8' }}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: '#1e293b',
                  border: '1px solid #475569',
                  borderRadius: '8px',
                  color: '#e2e8f0',
                }}
                labelStyle={{ color: '#94a3b8' }}
                formatter={(value: number) => [`${value} ms`, selectedMetric?.label || 'Time']}
              />
              <Line
                type="monotone"
                dataKey="selectedTiming"
                stroke={selectedMetric?.color || '#8b5cf6'}
                strokeWidth={2}
                dot={chartData.length < 50}
                name={selectedMetric?.label}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  )
}