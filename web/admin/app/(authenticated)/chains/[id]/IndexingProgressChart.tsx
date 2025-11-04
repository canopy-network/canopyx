'use client'

import { useEffect, useState } from 'react'
import { LineChart, Line, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import { apiFetch } from '../../../lib/api'

type ProgressDataPoint = {
  time: string
  height: number
  avg_latency: number
  avg_processing_time: number
  blocks_indexed: number
  velocity: number
}

type IndexingProgressChartProps = {
  chainId: string
}

export default function IndexingProgressChart({ chainId }: IndexingProgressChartProps) {
  const [data, setData] = useState<ProgressDataPoint[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')
  const [timeWindow, setTimeWindow] = useState<number>(24) // hours

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
    // Refresh every 30 seconds
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
    velocity: Math.round(point.velocity * 10) / 10, // blocks per minute
    latency: Math.round(point.avg_latency * 10) / 10, // seconds
    processingTime: Math.round(point.avg_processing_time), // milliseconds
    blocksIndexed: point.blocks_indexed,
  }))

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
        {/* Height Progress Chart */}
        <div>
          <h4 className="mb-2 text-sm font-medium text-slate-300">Block Height Over Time</h4>
          <ResponsiveContainer width="100%" height={200}>
            <AreaChart data={chartData}>
              <defs>
                <linearGradient id="colorHeight" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#6366f1" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#6366f1" stopOpacity={0} />
                </linearGradient>
              </defs>
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
                tickFormatter={(value) => value.toLocaleString()}
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
              <Area
                type="monotone"
                dataKey="height"
                stroke="#6366f1"
                strokeWidth={2}
                fillOpacity={1}
                fill="url(#colorHeight)"
              />
            </AreaChart>
          </ResponsiveContainer>
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
                dot={false}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>

        {/* Performance Metrics */}
        <div className="grid gap-4 md:grid-cols-2">
          <div>
            <h4 className="mb-2 text-sm font-medium text-slate-300">Avg Latency (seconds)</h4>
            <ResponsiveContainer width="100%" height={120}>
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
                  dataKey="latency"
                  stroke="#f59e0b"
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>

          <div>
            <h4 className="mb-2 text-sm font-medium text-slate-300">Avg Processing Time (ms)</h4>
            <ResponsiveContainer width="100%" height={120}>
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
                  dataKey="processingTime"
                  stroke="#8b5cf6"
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* Summary Stats */}
        <div className="grid gap-4 md:grid-cols-4 rounded-lg bg-slate-900/50 p-4">
          <div className="text-center">
            <p className="text-xs text-slate-400">Current Height</p>
            <p className="mt-1 font-mono text-lg font-bold text-indigo-400">
              {chartData[chartData.length - 1]?.height.toLocaleString() || '—'}
            </p>
          </div>
          <div className="text-center">
            <p className="text-xs text-slate-400">Avg Velocity</p>
            <p className="mt-1 font-mono text-lg font-bold text-green-400">
              {(
                chartData.reduce((sum, d) => sum + d.velocity, 0) / chartData.length || 0
              ).toFixed(1)}{' '}
              bl/min
            </p>
          </div>
          <div className="text-center">
            <p className="text-xs text-slate-400">Avg Latency</p>
            <p className="mt-1 font-mono text-lg font-bold text-amber-400">
              {(
                chartData.reduce((sum, d) => sum + d.latency, 0) / chartData.length || 0
              ).toFixed(1)}s
            </p>
          </div>
          <div className="text-center">
            <p className="text-xs text-slate-400">Avg Processing</p>
            <p className="mt-1 font-mono text-lg font-bold text-purple-400">
              {Math.round(
                chartData.reduce((sum, d) => sum + d.processingTime, 0) / chartData.length || 0
              )}ms
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}