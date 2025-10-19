'use client'

interface EventTypeBreakdownProps {
  eventCounts: { [key: string]: number }
}

// Color mapping for event types
const EVENT_TYPE_COLORS: { [key: string]: string } = {
  reward: 'bg-green-500',
  slash: 'bg-red-500',
  'dex-liquidity-deposit': 'bg-blue-500',
  'dex-liquidity-withdraw': 'bg-yellow-500',
  'dex-swap': 'bg-purple-500',
  'order-book-swap': 'bg-cyan-500',
  'automatic-pause': 'bg-orange-500',
  'automatic-begin-unstaking': 'bg-pink-500',
  'automatic-finish-unstaking': 'bg-indigo-500',
}

// Get color for an event type (with fallback)
function getColorForType(type: string): string {
  return EVENT_TYPE_COLORS[type] || 'bg-slate-500'
}

// Format event type for display
function formatEventType(type: string): string {
  return type
    .split('-')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

export function EventTypeBreakdown({ eventCounts }: EventTypeBreakdownProps) {
  // Calculate total events
  const total = Object.values(eventCounts).reduce((sum, count) => sum + count, 0)

  // No data state
  if (total === 0 || Object.keys(eventCounts).length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Event Type Breakdown</h3>
        </div>
        <div className="flex flex-col items-center justify-center py-12 text-slate-500">
          <svg className="h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
            />
          </svg>
          <p className="mt-4 text-sm">No event data available</p>
        </div>
      </div>
    )
  }

  // Sort by count (descending) and calculate percentages
  const sortedTypes = Object.entries(eventCounts)
    .map(([type, count]) => ({
      type,
      count,
      percentage: (count / total) * 100,
      color: getColorForType(type),
    }))
    .sort((a, b) => b.count - a.count)

  return (
    <div className="card">
      <div className="card-header">
        <h3 className="card-title">Event Type Breakdown</h3>
        <div className="flex items-center gap-2">
          <span className="text-xs text-slate-500">{total.toLocaleString()} total</span>
          <span className="text-xs text-slate-600">•</span>
          <span className="text-xs text-slate-500">{sortedTypes.length} types</span>
        </div>
      </div>

      <div className="space-y-3">
        {sortedTypes.map(({ type, count, percentage, color }) => (
          <div key={type} className="group">
            {/* Label row */}
            <div className="mb-1.5 flex items-center justify-between text-sm">
              <div className="flex items-center gap-2">
                <div
                  className={`h-2.5 w-2.5 rounded-full ${color} transition-transform group-hover:scale-125`}
                ></div>
                <span className="font-medium text-slate-200">{formatEventType(type)}</span>
              </div>
              <div className="flex items-center gap-3">
                <span className="text-xs text-slate-400">{count.toLocaleString()}</span>
                <span className="min-w-[3rem] text-right font-mono text-xs font-semibold text-white">
                  {percentage.toFixed(1)}%
                </span>
              </div>
            </div>

            {/* Progress bar */}
            <div className="relative h-2 overflow-hidden rounded-full bg-slate-800/50">
              <div
                className={`h-full ${color} transition-all duration-500 ease-out group-hover:opacity-90`}
                style={{ width: `${percentage}%` }}
              >
                {/* Shimmer effect */}
                <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/10 to-transparent opacity-0 transition-opacity group-hover:opacity-100"></div>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Summary footer */}
      <div className="mt-6 rounded-lg border border-slate-800 bg-slate-900/30 p-4">
        <div className="flex items-center justify-between text-sm">
          <div className="flex items-center gap-2">
            <svg className="h-4 w-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
              />
            </svg>
            <span className="text-slate-400">Total Events</span>
          </div>
          <span className="font-mono text-lg font-bold text-white">{total.toLocaleString()}</span>
        </div>
      </div>
    </div>
  )
}