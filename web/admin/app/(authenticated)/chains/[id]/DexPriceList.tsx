'use client'

import { useState, useEffect } from 'react'

// DexPrice type definition
interface DexPrice {
  local_chain_id: number
  remote_chain_id: number
  height: number
  local_pool: number
  remote_pool: number
  price_e6: number
  height_time: string
}

interface DexPricesResponse {
  data: DexPrice[]
  limit: number
  next_cursor?: number
}

interface DexPriceListProps {
  chainId: string
}

// Format large numbers with commas
function formatNumber(n: number | undefined): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(2)}K`
  return n.toLocaleString()
}

// Format price (scaled by 1M)
function formatPrice(priceE6: number): string {
  const price = priceE6 / 1_000_000
  return price.toFixed(6)
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

export function DexPriceList({ chainId }: DexPriceListProps) {
  const [prices, setPrices] = useState<DexPrice[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [cursor, setCursor] = useState<number | undefined>(undefined)
  const [hasMore, setHasMore] = useState(false)
  const [localChainFilter, setLocalChainFilter] = useState('')
  const [remoteChainFilter, setRemoteChainFilter] = useState('')

  const limit = 50

  // Fetch prices from API
  const fetchPrices = async (resetCursor = false) => {
    setLoading(true)
    setError('')

    try {
      const params = new URLSearchParams()
      params.set('limit', limit.toString())
      if (!resetCursor && cursor !== undefined) {
        params.set('cursor', cursor.toString())
      }
      if (localChainFilter) {
        params.set('local', localChainFilter)
      }
      if (remoteChainFilter) {
        params.set('remote', remoteChainFilter)
      }

      const response = await fetch(`/api/query/chains/${chainId}/dex-prices?${params.toString()}`)

      if (!response.ok) {
        throw new Error(`Failed to fetch DEX prices: ${response.statusText}`)
      }

      const result: DexPricesResponse = await response.json()

      // Append to existing prices if loading more, otherwise replace
      if (resetCursor) {
        setPrices(result.data || [])
      } else {
        setPrices((prev) => [...prev, ...(result.data || [])])
      }

      setHasMore(result.next_cursor !== undefined && result.next_cursor !== null)
      setCursor(result.next_cursor)
    } catch (err: any) {
      console.error('DEX prices fetch error:', err)
      setError(err.message || 'Failed to load DEX prices')
      if (resetCursor) {
        setPrices([])
      }
      setHasMore(false)
    } finally {
      setLoading(false)
    }
  }

  // Fetch prices on mount and when filters change
  useEffect(() => {
    fetchPrices(true)
  }, [chainId, localChainFilter, remoteChainFilter])

  // Handle load more
  const handleLoadMore = () => {
    if (!loading && hasMore) {
      fetchPrices(false)
    }
  }

  // Handle filter application
  const handleApplyFilters = () => {
    setCursor(undefined)
    fetchPrices(true)
  }

  // Handle filter clear
  const handleClearFilters = () => {
    setLocalChainFilter('')
    setRemoteChainFilter('')
    setCursor(undefined)
  }

  if (loading && prices.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">DEX Prices</h3>
        </div>
        <div className="flex items-center justify-center py-12">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
            <p className="text-slate-400">Loading prices...</p>
          </div>
        </div>
      </div>
    )
  }

  if (error && prices.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">DEX Prices</h3>
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
            onClick={() => fetchPrices(true)}
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
            <h3 className="card-title">DEX Prices</h3>
            <span className="text-xs text-slate-500">{prices.length} shown</span>
          </div>
        </div>
      </div>

      {/* Chain Pair Filters */}
      <div className="border-b border-slate-800 px-4 py-3">
        <div className="flex flex-wrap items-end gap-3">
          <div className="flex-1 min-w-[150px]">
            <label className="block text-xs font-medium text-slate-400 mb-1">
              Local Chain ID
            </label>
            <input
              type="number"
              value={localChainFilter}
              onChange={(e) => setLocalChainFilter(e.target.value)}
              placeholder="e.g., 1"
              className="input w-full text-sm"
              disabled={loading}
            />
          </div>
          <div className="flex-1 min-w-[150px]">
            <label className="block text-xs font-medium text-slate-400 mb-1">
              Remote Chain ID
            </label>
            <input
              type="number"
              value={remoteChainFilter}
              onChange={(e) => setRemoteChainFilter(e.target.value)}
              placeholder="e.g., 2"
              className="input w-full text-sm"
              disabled={loading}
            />
          </div>
          {(localChainFilter || remoteChainFilter) && (
            <button
              onClick={handleClearFilters}
              className="btn-secondary text-sm"
              disabled={loading}
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
              Clear
            </button>
          )}
        </div>
      </div>

      {prices.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-slate-500">
          <svg className="h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6"
            />
          </svg>
          <p className="mt-4 text-sm">No DEX prices available</p>
          {(localChainFilter || remoteChainFilter) && (
            <p className="mt-1 text-xs text-slate-600">
              Try adjusting your chain filters
            </p>
          )}
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Local Chain</th>
                  <th>Remote Chain</th>
                  <th>Price</th>
                  <th>Local Pool</th>
                  <th>Remote Pool</th>
                  <th>Height</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {prices.map((price, idx) => (
                  <tr key={`${price.local_chain_id}-${price.remote_chain_id}-${price.height}-${idx}`}>
                    <td className="font-mono text-sm text-blue-400">
                      Chain {price.local_chain_id}
                    </td>
                    <td className="font-mono text-sm text-purple-400">
                      Chain {price.remote_chain_id}
                    </td>
                    <td className="font-mono text-sm font-semibold text-emerald-400">
                      {formatPrice(price.price_e6)}
                    </td>
                    <td className="font-mono text-sm text-slate-300">
                      {formatNumber(price.local_pool)}
                    </td>
                    <td className="font-mono text-sm text-slate-300">
                      {formatNumber(price.remote_pool)}
                    </td>
                    <td className="font-mono text-sm text-slate-400">
                      #{price.height.toLocaleString()}
                    </td>
                    <td className="text-xs text-slate-400">
                      {formatTime(price.height_time)}
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