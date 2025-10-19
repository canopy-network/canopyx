'use client'

interface Transaction {
  height: number
  tx_hash: string
  message_type: string
  signer: string
  counterparty?: string | null
  amount?: number | null
  fee: number
  time: string
}

interface TransactionListProps {
  transactions: Transaction[]
  loading?: boolean
}

// Color mapping for transaction type badges
const TX_TYPE_COLORS: { [key: string]: string } = {
  send: 'bg-blue-500/20 text-blue-300 border-blue-500/30',
  delegate: 'bg-purple-500/20 text-purple-300 border-purple-500/30',
  undelegate: 'bg-purple-300/20 text-purple-200 border-purple-300/30',
  stake: 'bg-green-500/20 text-green-300 border-green-500/30',
  unstake: 'bg-green-300/20 text-green-200 border-green-300/30',
  vote: 'bg-yellow-500/20 text-yellow-300 border-yellow-500/30',
  proposal: 'bg-orange-500/20 text-orange-300 border-orange-500/30',
  contract: 'bg-red-500/20 text-red-300 border-red-500/30',
  system: 'bg-gray-500/20 text-gray-300 border-gray-500/30',
}

// Get color classes for a transaction type badge
function getBadgeColorForType(type: string): string {
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
  return 'bg-slate-500/20 text-slate-300 border-slate-500/30'
}

// Format transaction type for display
function formatTypeName(type: string): string {
  let formatted = type.replace(/^(cosmos\.)?/, '')
  formatted = formatted.replace(/^Msg/, '')
  formatted = formatted.replace(/([A-Z])/g, ' $1').trim()
  return formatted
}

// Truncate address with ellipsis
function truncateAddress(address: string, prefixLen = 8, suffixLen = 6): string {
  if (address.length <= prefixLen + suffixLen) {
    return address
  }
  return `${address.slice(0, prefixLen)}...${address.slice(-suffixLen)}`
}

// Format amount with proper decimals (assuming 6 decimals for most cosmos chains)
function formatAmount(amount: number | null | undefined): string {
  if (amount === null || amount === undefined) {
    return '—'
  }
  // Convert from base units (uatom, etc.) to display units
  const displayAmount = amount / 1_000_000
  if (displayAmount >= 1_000_000) {
    return `${(displayAmount / 1_000_000).toFixed(2)}M`
  }
  if (displayAmount >= 1_000) {
    return `${(displayAmount / 1_000).toFixed(2)}K`
  }
  return displayAmount.toFixed(2)
}

// Format fee
function formatFee(fee: number): string {
  const displayFee = fee / 1_000_000
  return displayFee.toFixed(4)
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

export function TransactionList({ transactions, loading = false }: TransactionListProps) {
  if (loading) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Recent Transactions</h3>
        </div>
        <div className="flex items-center justify-center py-12">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
            <p className="text-slate-400">Loading transactions...</p>
          </div>
        </div>
      </div>
    )
  }

  if (!transactions || transactions.length === 0) {
    return (
      <div className="card">
        <div className="card-header">
          <h3 className="card-title">Recent Transactions</h3>
        </div>
        <div className="flex flex-col items-center justify-center py-12 text-slate-500">
          <svg className="h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
            />
          </svg>
          <p className="mt-4 text-sm">No transactions available</p>
        </div>
      </div>
    )
  }

  return (
    <div className="card">
      <div className="card-header">
        <h3 className="card-title">Recent Transactions</h3>
        <span className="text-xs text-slate-500">{transactions.length} shown</span>
      </div>

      <div className="space-y-3">
        {transactions.map((tx) => (
          <div
            key={`${tx.height}-${tx.tx_hash}`}
            className="group rounded-lg border border-slate-800 bg-slate-900/30 p-4 transition-all hover:border-indigo-500/50 hover:bg-slate-900/50 hover:shadow-lg hover:shadow-indigo-500/10"
          >
            {/* Top row: Type badge and hash */}
            <div className="mb-3 flex items-start justify-between gap-3">
              <span
                className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition-all group-hover:scale-105 ${getBadgeColorForType(tx.message_type)}`}
              >
                <svg className="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
                  <circle cx="10" cy="10" r="3" />
                </svg>
                {formatTypeName(tx.message_type)}
              </span>

              <div className="flex items-center gap-2 text-xs text-slate-500">
                <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M7 20l4-16m2 16l4-16M6 9h14M4 15h14"
                  />
                </svg>
                <span className="font-mono">{truncateAddress(tx.tx_hash, 6, 6)}</span>
              </div>
            </div>

            {/* Middle row: From/To addresses */}
            <div className="mb-3 flex items-center gap-2 text-sm">
              <div className="flex items-center gap-1.5 rounded bg-slate-800/50 px-2.5 py-1">
                <svg className="h-3.5 w-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                  />
                </svg>
                <span className="font-mono text-xs text-slate-300">{truncateAddress(tx.signer)}</span>
              </div>

              {tx.counterparty && (
                <>
                  <svg className="h-4 w-4 text-slate-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                  </svg>
                  <div className="flex items-center gap-1.5 rounded bg-slate-800/50 px-2.5 py-1">
                    <svg className="h-3.5 w-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                      />
                    </svg>
                    <span className="font-mono text-xs text-slate-300">{truncateAddress(tx.counterparty)}</span>
                  </div>
                </>
              )}
            </div>

            {/* Bottom row: Amount, Fee, Height, Time */}
            <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs">
              {tx.amount !== null && tx.amount !== undefined && (
                <div className="flex items-center gap-1.5">
                  <svg className="h-3.5 w-3.5 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span className="text-slate-500">Amount:</span>
                  <span className="font-mono font-semibold text-emerald-400">{formatAmount(tx.amount)}</span>
                </div>
              )}

              <div className="flex items-center gap-1.5">
                <svg className="h-3.5 w-3.5 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M17 9V7a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2m2 4h10a2 2 0 002-2v-6a2 2 0 00-2-2H9a2 2 0 00-2 2v6a2 2 0 002 2zm7-5a2 2 0 11-4 0 2 2 0 014 0z"
                  />
                </svg>
                <span className="text-slate-500">Fee:</span>
                <span className="font-mono text-amber-400">{formatFee(tx.fee)}</span>
              </div>

              <div className="flex items-center gap-1.5">
                <svg className="h-3.5 w-3.5 text-indigo-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"
                  />
                </svg>
                <span className="text-slate-500">Block:</span>
                <span className="font-mono text-indigo-400">#{tx.height.toLocaleString()}</span>
              </div>

              <div className="flex items-center gap-1.5">
                <svg className="h-3.5 w-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                <span className="text-slate-500">{formatTime(tx.time)}</span>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}