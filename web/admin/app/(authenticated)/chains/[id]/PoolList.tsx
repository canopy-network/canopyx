'use client'

import { useState, useEffect } from 'react'

// Pool type definition
interface Pool {
  pool_id: number
  height: number
  chain_id: number
  amount: number
  total_points: number
  lp_count: number
  height_time: string
}

interface PoolsResponse {
  data: Pool[]
  limit: number
  next_cursor?: number
}

interface PoolListProps {
  chainId: string
}

// Format large numbers with commas
function formatNumber(n: number | undefined): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return n.toLocaleString()
}

// Format timestamp
function formatTime(time: string): string {
  try {
    const date = new Date(time)
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return time
  }
}

export function PoolList({ chainId }: PoolListProps) {
  const [pools, setPools] = useState<Pool[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [cursor, setCursor] = useState<number | undefined>(undefined)
  const [hasMore, setHasMore] = useState(false)

  const limit = 50

  // Fetch pools from API
  const fetchPools = async (resetCursor = false) => {
    setLoading(true)
    setError('')

    try {
      const params = new URLSearchParams()
      params.set('limit', limit.toString())
      if (!resetCursor && cursor !== undefined) {
        params.set('cursor', cursor.toString())
      }

      const response = await fetch(`/api/query/chains/${chainId}/pools?${params.toString()}`)

      if (!response.ok) {
        throw new Error(`Failed to fetch pools: ${response.statusText}`)
      }

      const result: PoolsResponse = await response.json()

      // Append to existing pools if loading more, otherwise replace
      if (resetCursor) {
        setPools(result.data || [])
      } else {
        setPools((prev) => [...prev, ...(result.data || [])])
      }

      setHasMore(result.next_cursor !== undefined && result.next_cursor !== null)
      setCursor(result.next_cursor)
    } catch (err: any) {
      console.error('Pools fetch error:', err)
      setError(err.message || 'Failed to load pools')
      if (resetCursor) {
        setPools([])
      }
      setHasMore(false)
    } finally {
      setLoading(false)
    }
  }

  // Fetch pools on mount
  useEffect(() => {
    fetchPools(true)
  }, [chainId])

  // Handle load more
  const handleLoadMore = () => {
    if (!loading && hasMore) {
      fetchPools(false)
    }
  }

  if (loading && pools.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Liquidity Pools</h3>
        </div>
        <div className="flex items-center justify-center py-12">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
            <p className="text-slate-400">Loading pools...</p>
          </div>
        </div>
      </div>
    )
  }

  if (error && pools.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Liquidity Pools</h3>
        </div>
        <div className="flex flex-col items-center justify-center py-12 text-rose-400">
          <svg className="h-16 w-16" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
          <p className="mt-4 text-sm">{error}</p>
          <button
            onClick={() => fetchPools(true)}
            className="mt-4 btn-secondary text-sm"
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="card">
      <div className="card-header">
        <div className="flex items-center justify-between w-full">
          <div className="flex items-center gap-3">
            <h3 className="card-title">Liquidity Pools</h3>
            <span className="text-xs text-slate-500">{pools.length} shown</span>
          </div>
        </div>
      </div>

      {pools.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-slate-500">
          <svg className="h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
            />
          </svg>
          <p className="mt-4 text-sm">No pools available</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Pool ID</th>
                  <th>Chain ID</th>
                  <th>Liquidity</th>
                  <th>LP Count</th>
                  <th>Total Points</th>
                  <th>Height</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {pools.map((pool) => (
                  <tr key={`${pool.pool_id}-${pool.height}`}>
                    <td className="font-mono text-sm font-semibold text-indigo-400">
                      #{pool.pool_id}
                    </td>
                    <td className="font-mono text-sm text-slate-300">
                      {pool.chain_id}
                    </td>
                    <td className="font-mono text-sm text-emerald-400">
                      {formatNumber(pool.amount)}
                    </td>
                    <td className="font-mono text-sm text-slate-300">
                      {formatNumber(pool.lp_count)}
                    </td>
                    <td className="font-mono text-sm text-purple-400">
                      {formatNumber(pool.total_points)}
                    </td>
                    <td className="font-mono text-sm text-slate-400">
                      #{pool.height.toLocaleString()}
                    </td>
                    <td className="text-xs text-slate-400">
                      {formatTime(pool.height_time)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {hasMore && (
            <div className="mt-4 flex justify-center border-t border-slate-800 pt-4">
              <button
                onClick={handleLoadMore}
                disabled={loading}
                className="btn-secondary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? (
                  <>
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-slate-500 border-t-slate-300"></div>
                    Loading...
                  </>
                ) : (
                  <>
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                    </svg>
                    Load More
                  </>
                )}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}