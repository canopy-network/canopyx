'use client'

import { useState, useEffect } from 'react'
import { apiFetch } from '../../../lib/api'

// Event type definitions
interface Event {
  height: number
  address: string
  reference: string
  event_type: string
  amount?: number
  sold_amount?: number
  bought_amount?: number
  local_amount?: number
  remote_amount?: number
  success?: boolean
  local_origin?: boolean
  order_id?: string
  height_time: string
}

interface EventsResponse {
  data: Event[]
  limit: number
  next_cursor?: number
}

interface EventListProps {
  chainId: string
}

// Color mapping for event type badges
const EVENT_TYPE_COLORS: { [key: string]: string } = {
  reward: 'bg-green-500/20 text-green-300 border-green-500/30',
  slash: 'bg-red-500/20 text-red-300 border-red-500/30',
  'dex-liquidity-deposit': 'bg-blue-500/20 text-blue-300 border-blue-500/30',
  'dex-liquidity-withdraw': 'bg-yellow-500/20 text-yellow-300 border-yellow-500/30',
  'dex-swap': 'bg-purple-500/20 text-purple-300 border-purple-500/30',
  'order-book-swap': 'bg-cyan-500/20 text-cyan-300 border-cyan-500/30',
  'automatic-pause': 'bg-orange-500/20 text-orange-300 border-orange-500/30',
  'automatic-begin-unstaking': 'bg-pink-500/20 text-pink-300 border-pink-500/30',
  'automatic-finish-unstaking': 'bg-indigo-500/20 text-indigo-300 border-indigo-500/30',
}

// Get color classes for an event type badge
function getBadgeColorForType(type: string): string {
  return EVENT_TYPE_COLORS[type] || 'bg-slate-500/20 text-slate-300 border-slate-500/30'
}

// Format event type for display
function formatEventType(type: string): string {
  return type
    .split('-')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

// Truncate address with ellipsis
function truncateAddress(address: string, prefixLen = 8, suffixLen = 6): string {
  if (address.length <= prefixLen + suffixLen) {
    return address
  }
  return `${address.slice(0, prefixLen)}...${address.slice(-suffixLen)}`
}

// Format amount with proper decimals (assuming 6 decimals)
function formatAmount(amount: number | null | undefined): string {
  if (amount === null || amount === undefined) {
    return '—'
  }
  const displayAmount = amount / 1_000_000
  if (displayAmount >= 1_000_000) {
    return `${(displayAmount / 1_000_000).toFixed(2)}M`
  }
  if (displayAmount >= 1_000) {
    return `${(displayAmount / 1_000).toFixed(2)}K`
  }
  return displayAmount.toFixed(2)
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

// Copy to clipboard helper
function copyToClipboard(text: string) {
  navigator.clipboard.writeText(text).catch((err) => {
    console.error('Failed to copy:', err)
  })
}

export function EventList({ chainId }: EventListProps) {
  const [events, setEvents] = useState<Event[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [cursor, setCursor] = useState<number | undefined>(undefined)
  const [hasMore, setHasMore] = useState(false)
  const [selectedType, setSelectedType] = useState<string>('')
  const [copiedAddress, setCopiedAddress] = useState<string | null>(null)

  const limit = 50

  // All available event types for the filter dropdown
  const eventTypes = [
    { value: '', label: 'All Events' },
    { value: 'reward', label: 'Reward' },
    { value: 'slash', label: 'Slash' },
    { value: 'dex-liquidity-deposit', label: 'DEX Liquidity Deposit' },
    { value: 'dex-liquidity-withdraw', label: 'DEX Liquidity Withdraw' },
    { value: 'dex-swap', label: 'DEX Swap' },
    { value: 'order-book-swap', label: 'Order Book Swap' },
    { value: 'automatic-pause', label: 'Automatic Pause' },
    { value: 'automatic-begin-unstaking', label: 'Automatic Begin Unstaking' },
    { value: 'automatic-finish-unstaking', label: 'Automatic Finish Unstaking' },
  ]

  // Fetch events from API
  const fetchEvents = async (resetCursor = false) => {
    setLoading(true)
    setError('')

    try {
      const params = new URLSearchParams()
      params.set('limit', limit.toString())
      if (!resetCursor && cursor !== undefined) {
        params.set('cursor', cursor.toString())
      }
      if (selectedType) {
        params.set('type', selectedType)
      }

      const response = await apiFetch(`/api/admin/chains/${chainId}/events?${params.toString()}`)

      if (!response.ok) {
        throw new Error(`Failed to fetch events: ${response.statusText}`)
      }

      const result: EventsResponse = await response.json()
      setEvents(result.data || [])
      setHasMore(result.next_cursor !== undefined && result.next_cursor !== null)
      setCursor(result.next_cursor)
    } catch (err: any) {
      console.error('Events fetch error:', err)
      setError(err.message || 'Failed to load events')
      setEvents([])
      setHasMore(false)
    } finally {
      setLoading(false)
    }
  }

  // Fetch events on mount and when filter changes
  useEffect(() => {
    fetchEvents(true)
  }, [chainId, selectedType])

  // Handle type filter change
  const handleTypeChange = (type: string) => {
    setSelectedType(type)
    setCursor(undefined)
  }

  // Handle load more
  const handleLoadMore = () => {
    if (!loading && hasMore) {
      fetchEvents(false)
    }
  }

  // Handle copy address
  const handleCopyAddress = (address: string) => {
    copyToClipboard(address)
    setCopiedAddress(address)
    setTimeout(() => setCopiedAddress(null), 2000)
  }

  // Render reference column
  const renderReference = (reference: string) => {
    if (reference === 'begin_block' || reference === 'end_block') {
      return (
        <span className="rounded bg-slate-800/50 px-2 py-1 text-xs text-slate-400">
          {reference === 'begin_block' ? 'Begin Block' : 'End Block'}
        </span>
      )
    }
    return (
      <div className="flex items-center gap-2">
        <span className="font-mono text-xs text-slate-300">{truncateAddress(reference, 6, 6)}</span>
        <button
          onClick={() => handleCopyAddress(reference)}
          className="text-slate-500 hover:text-slate-300 transition-colors"
          title="Copy transaction hash"
        >
          {copiedAddress === reference ? (
            <svg className="h-3.5 w-3.5 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          ) : (
            <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
              />
            </svg>
          )}
        </button>
      </div>
    )
  }

  // Render amount column based on event type
  const renderAmount = (event: Event) => {
    if (event.amount !== undefined && event.amount !== null) {
      return (
        <span className="font-mono text-xs text-emerald-400">{formatAmount(event.amount)}</span>
      )
    }

    if (event.event_type === 'dex-swap' || event.event_type === 'order-book-swap') {
      return (
        <div className="text-xs">
          <div className="text-red-400">
            Sold: <span className="font-mono">{formatAmount(event.sold_amount)}</span>
          </div>
          <div className="text-green-400">
            Bought: <span className="font-mono">{formatAmount(event.bought_amount)}</span>
          </div>
        </div>
      )
    }

    if (event.event_type === 'dex-liquidity-deposit' || event.event_type === 'dex-liquidity-withdraw') {
      return (
        <div className="text-xs">
          <div className="text-blue-400">
            Local: <span className="font-mono">{formatAmount(event.local_amount)}</span>
          </div>
          <div className="text-purple-400">
            Remote: <span className="font-mono">{formatAmount(event.remote_amount)}</span>
          </div>
        </div>
      )
    }

    return <span className="text-slate-500">—</span>
  }

  if (loading && events.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Events</h3>
        </div>
        <div className="flex items-center justify-center py-12">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
            <p className="text-slate-400">Loading events...</p>
          </div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Events</h3>
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
            onClick={() => fetchEvents(true)}
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
            <h3 className="card-title">Events</h3>
            <span className="text-xs text-slate-500">{events.length} shown</span>
          </div>
          <div className="flex items-center gap-3">
            <label className="text-xs text-slate-400">Filter:</label>
            <select
              value={selectedType}
              onChange={(e) => handleTypeChange(e.target.value)}
              className="input w-auto text-sm py-1"
              disabled={loading}
            >
              {eventTypes.map((type) => (
                <option key={type.value} value={type.value}>
                  {type.label}
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {events.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-slate-500">
          <svg className="h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
            />
          </svg>
          <p className="mt-4 text-sm">No events available</p>
          {selectedType && (
            <p className="mt-1 text-xs text-slate-600">
              Try selecting a different event type
            </p>
          )}
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Height</th>
                  <th>Event Type</th>
                  <th>Address</th>
                  <th>Amount</th>
                  <th>Reference</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {events.map((event, idx) => (
                  <tr key={`${event.height}-${event.event_type}-${idx}`}>
                    <td className="font-mono text-sm">
                      #{event.height.toLocaleString()}
                    </td>
                    <td>
                      <span
                        className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium ${getBadgeColorForType(event.event_type)}`}
                      >
                        <svg className="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
                          <circle cx="10" cy="10" r="3" />
                        </svg>
                        {formatEventType(event.event_type)}
                      </span>
                    </td>
                    <td>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-xs text-slate-300">
                          {truncateAddress(event.address)}
                        </span>
                        <button
                          onClick={() => handleCopyAddress(event.address)}
                          className="text-slate-500 hover:text-slate-300 transition-colors"
                          title="Copy address"
                        >
                          {copiedAddress === event.address ? (
                            <svg className="h-3.5 w-3.5 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                            </svg>
                          ) : (
                            <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                strokeWidth={2}
                                d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                              />
                            </svg>
                          )}
                        </button>
                      </div>
                    </td>
                    <td>{renderAmount(event)}</td>
                    <td>{renderReference(event.reference)}</td>
                    <td className="text-xs text-slate-400">
                      {formatTime(event.height_time)}
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