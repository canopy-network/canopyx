'use client'

interface TransactionTypeBreakdownProps {
  txCounts: { [key: string]: number }
}

// Color mapping for transaction types
const TX_TYPE_COLORS: { [key: string]: string } = {
  send: 'bg-blue-500',
  delegate: 'bg-purple-500',
  undelegate: 'bg-purple-300',
  stake: 'bg-green-500',
  unstake: 'bg-green-300',
  vote: 'bg-yellow-500',
  proposal: 'bg-orange-500',
  contract: 'bg-red-500',
  system: 'bg-gray-500',
}

// Get color for a transaction type (with fallback)
function getColorForType(type: string): string {
  const lowerType = type.toLowerCase()

  // Direct match
  if (TX_TYPE_COLORS[lowerType]) {
    return TX_TYPE_COLORS[lowerType]
  }

  // Pattern matching for common prefixes
  if (lowerType.includes('send') || lowerType.includes('transfer')) return TX_TYPE_COLORS.send
  if (lowerType.includes('delegate')) return TX_TYPE_COLORS.delegate
  if (lowerType.includes('undelegate') || lowerType.includes('unbond')) return TX_TYPE_COLORS.undelegate
  if (lowerType.includes('stake')) return TX_TYPE_COLORS.stake
  if (lowerType.includes('unstake')) return TX_TYPE_COLORS.unstake
  if (lowerType.includes('vote')) return TX_TYPE_COLORS.vote
  if (lowerType.includes('proposal') || lowerType.includes('governance')) return TX_TYPE_COLORS.proposal
  if (lowerType.includes('contract') || lowerType.includes('execute')) return TX_TYPE_COLORS.contract

  // Default fallback
  return 'bg-slate-500'
}

// Format transaction type for display (clean up common patterns)
function formatTypeName(type: string): string {
  // Remove common prefixes
  let formatted = type.replace(/^(cosmos\.)?/, '')
  formatted = formatted.replace(/^Msg/, '')

  // Add spaces before capital letters
  formatted = formatted.replace(/([A-Z])/g, ' $1').trim()

  return formatted
}

export function TransactionTypeBreakdown({ txCounts }: TransactionTypeBreakdownProps) {
  // Calculate total transactions
  const total = Object.values(txCounts).reduce((sum, count) => sum + count, 0)

  // No data state
  if (total === 0 || Object.keys(txCounts).length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Transaction Type Breakdown</h3>
        </div>
        <div className="flex flex-col items-center justify-center py-12 text-slate-500">
          <svg className="h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
          </svg>
          <p className="mt-4 text-sm">No transaction data available</p>
        </div>
      </div>
    )
  }

  // Sort by count (descending) and calculate percentages
  const sortedTypes = Object.entries(txCounts)
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
        <h3 className="card-title">Transaction Type Breakdown</h3>
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
                <div className={`h-2.5 w-2.5 rounded-full ${color} transition-transform group-hover:scale-125`}></div>
                <span className="font-medium text-slate-200">{formatTypeName(type)}</span>
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
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
            </svg>
            <span className="text-slate-400">Total Transactions</span>
          </div>
          <span className="font-mono text-lg font-bold text-white">{total.toLocaleString()}</span>
        </div>
      </div>
    </div>
  )
}