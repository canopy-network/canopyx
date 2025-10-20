'use client'

import { useState, useEffect } from 'react'

// Order type definition
interface Order {
  order_id: string
  height: number
  committee: number
  amount_for_sale: number
  requested_amount: number
  seller_address: string
  buyer_address: string | null
  deadline: number | null
  status: string
  created_height: number
  height_time: string
}

interface OrdersResponse {
  data: Order[]
  limit: number
  next_cursor?: number
}

interface OrderListProps {
  chainId: string
}

// Order status options
type OrderStatus = 'all' | 'open' | 'filled' | 'cancelled' | 'expired'

// Status badge color mapping
const STATUS_COLORS: { [key: string]: string } = {
  open: 'bg-blue-500/20 text-blue-300 border-blue-500/30',
  filled: 'bg-green-500/20 text-green-300 border-green-500/30',
  cancelled: 'bg-gray-500/20 text-gray-300 border-gray-500/30',
  expired: 'bg-red-500/20 text-red-300 border-red-500/30',
}

// Get badge color for status
function getBadgeColorForStatus(status: string): string {
  return STATUS_COLORS[status.toLowerCase()] || 'bg-slate-500/20 text-slate-300 border-slate-500/30'
}

// Format large numbers with commas
function formatNumber(n: number | undefined): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(2)}K`
  return n.toLocaleString()
}

// Truncate string (for order ID and addresses)
function truncate(str: string, length: number = 8): string {
  if (str.length <= length) return str
  return `${str.slice(0, length)}...`
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

// Calculate price
function calculatePrice(requestedAmount: number, amountForSale: number): string {
  if (amountForSale === 0) return '—'
  const price = requestedAmount / amountForSale
  return price.toFixed(6)
}

export function OrderList({ chainId }: OrderListProps) {
  const [orders, setOrders] = useState<Order[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [cursor, setCursor] = useState<number | undefined>(undefined)
  const [hasMore, setHasMore] = useState(false)
  const [selectedStatus, setSelectedStatus] = useState<OrderStatus>('all')

  const limit = 50

  // Status tabs
  const statusTabs: { value: OrderStatus; label: string }[] = [
    { value: 'all', label: 'All Orders' },
    { value: 'open', label: 'Open' },
    { value: 'filled', label: 'Filled' },
    { value: 'cancelled', label: 'Cancelled' },
    { value: 'expired', label: 'Expired' },
  ]

  // Fetch orders from API
  const fetchOrders = async (resetCursor = false) => {
    setLoading(true)
    setError('')

    try {
      const params = new URLSearchParams()
      params.set('limit', limit.toString())
      if (!resetCursor && cursor !== undefined) {
        params.set('cursor', cursor.toString())
      }
      if (selectedStatus !== 'all') {
        params.set('status', selectedStatus)
      }

      const response = await fetch(`/api/query/chains/${chainId}/orders?${params.toString()}`)

      if (!response.ok) {
        throw new Error(`Failed to fetch orders: ${response.statusText}`)
      }

      const result: OrdersResponse = await response.json()

      // Append to existing orders if loading more, otherwise replace
      if (resetCursor) {
        setOrders(result.data || [])
      } else {
        setOrders((prev) => [...prev, ...(result.data || [])])
      }

      setHasMore(result.next_cursor !== undefined && result.next_cursor !== null)
      setCursor(result.next_cursor)
    } catch (err: any) {
      console.error('Orders fetch error:', err)
      setError(err.message || 'Failed to load orders')
      if (resetCursor) {
        setOrders([])
      }
      setHasMore(false)
    } finally {
      setLoading(false)
    }
  }

  // Fetch orders on mount and when status filter changes
  useEffect(() => {
    fetchOrders(true)
  }, [chainId, selectedStatus])

  // Handle status tab change
  const handleStatusChange = (status: OrderStatus) => {
    setSelectedStatus(status)
    setCursor(undefined)
  }

  // Handle load more
  const handleLoadMore = () => {
    if (!loading && hasMore) {
      fetchOrders(false)
    }
  }

  if (loading && orders.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Order Book</h3>
        </div>
        <div className="flex items-center justify-center py-12">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
            <p className="text-slate-400">Loading orders...</p>
          </div>
        </div>
      </div>
    )
  }

  if (error && orders.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Order Book</h3>
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
            onClick={() => fetchOrders(true)}
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
            <h3 className="card-title">Order Book</h3>
            <span className="text-xs text-slate-500">{orders.length} shown</span>
          </div>
        </div>
      </div>

      {/* Status Filter Tabs */}
      <div className="border-b border-slate-800 px-4">
        <div className="flex gap-2 overflow-x-auto">
          {statusTabs.map((tab) => (
            <button
              key={tab.value}
              onClick={() => handleStatusChange(tab.value)}
              disabled={loading}
              className={`whitespace-nowrap border-b-2 px-4 py-3 text-sm font-medium transition-colors disabled:opacity-50 ${
                selectedStatus === tab.value
                  ? 'border-indigo-500 text-white'
                  : 'border-transparent text-slate-400 hover:text-white'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {orders.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-slate-500">
          <svg className="h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
            />
          </svg>
          <p className="mt-4 text-sm">No orders available</p>
          {selectedStatus !== 'all' && (
            <p className="mt-1 text-xs text-slate-600">
              Try selecting a different status filter
            </p>
          )}
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Order ID</th>
                  <th>Committee</th>
                  <th>Status</th>
                  <th>Amount For Sale</th>
                  <th>Requested Amount</th>
                  <th>Price</th>
                  <th>Seller</th>
                  <th>Height</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {orders.map((order) => (
                  <tr key={`${order.order_id}-${order.height}`}>
                    <td className="font-mono text-sm text-indigo-400">
                      {truncate(order.order_id, 8)}
                    </td>
                    <td className="font-mono text-sm text-slate-300">
                      {order.committee}
                    </td>
                    <td>
                      <span
                        className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium ${getBadgeColorForStatus(order.status)}`}
                      >
                        <svg className="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
                          <circle cx="10" cy="10" r="3" />
                        </svg>
                        {order.status.charAt(0).toUpperCase() + order.status.slice(1)}
                      </span>
                    </td>
                    <td className="font-mono text-sm text-amber-400">
                      {formatNumber(order.amount_for_sale)}
                    </td>
                    <td className="font-mono text-sm text-emerald-400">
                      {formatNumber(order.requested_amount)}
                    </td>
                    <td className="font-mono text-sm text-purple-400">
                      {calculatePrice(order.requested_amount, order.amount_for_sale)}
                    </td>
                    <td className="font-mono text-xs text-slate-400">
                      {truncate(order.seller_address, 12)}
                    </td>
                    <td className="font-mono text-sm text-slate-400">
                      #{order.height.toLocaleString()}
                    </td>
                    <td className="text-xs text-slate-400">
                      {formatTime(order.height_time)}
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