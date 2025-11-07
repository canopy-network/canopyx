'use client'

import { useState, useCallback, useEffect } from 'react'
import { apiFetch } from '../../lib/api'
import { useToast } from '../../components/ToastProvider'

// API Response Types
interface EntityInfo {
  name: string
  table_name: string
  global_table: string
  route: string
}

interface EntityListResponse {
  entities: EntityInfo[]
}

interface QueryMetadata {
  total: number
  limit: number
  offset: number
  has_more: boolean
}

interface EntityDataResponse {
  data: Record<string, any>[]
  total: number
  limit: number
  offset: number
  has_more: boolean
}

// UI Configuration for each entity type
interface EntityUIConfig {
  label: string
  icon: string
  description: string
  defaultColumns: string[] // Columns to display by default
  filterableFields?: { field: string; label: string; placeholder: string }[] // Common fields to filter
}

const ENTITY_UI_CONFIG: Record<string, EntityUIConfig> = {
  'accounts': {
    label: 'Accounts',
    icon: '👤',
    description: 'Account balances and rewards across chains',
    defaultColumns: ['chain_id', 'address', 'amount', 'rewards', 'height', 'height_time'],
    filterableFields: [
      { field: 'address', label: 'Address', placeholder: 'e.g., canopy1abc...' },
    ],
  },
  'validators': {
    label: 'Validators',
    icon: '🔐',
    description: 'Validator information and status',
    defaultColumns: ['chain_id', 'address', 'tokens', 'delegator_shares', 'status', 'height'],
    filterableFields: [
      { field: 'address', label: 'Validator Address', placeholder: 'e.g., canopyvaloper1...' },
      { field: 'status', label: 'Status', placeholder: 'e.g., BOND_STATUS_BONDED' },
    ],
  },
  'validator-signing-info': {
    label: 'Validator Signing Info',
    icon: '✍️',
    description: 'Validator signing activity and uptime',
    defaultColumns: ['chain_id', 'address', 'start_height', 'missed_blocks_counter', 'height'],
    filterableFields: [
      { field: 'address', label: 'Validator Address', placeholder: 'e.g., canopyvalcons1...' },
    ],
  },
  'validator-double-signing-info': {
    label: 'Validator Double Signing',
    icon: '⚠️',
    description: 'Double signing violations',
    defaultColumns: ['chain_id', 'address', 'violation_height', 'height'],
    filterableFields: [
      { field: 'address', label: 'Validator Address', placeholder: 'e.g., canopyvalcons1...' },
    ],
  },
  'pools': {
    label: 'Pools',
    icon: '💧',
    description: 'Liquidity pool information',
    defaultColumns: ['chain_id', 'pool_id', 'pool_supply', 'reserves', 'height'],
    filterableFields: [
      { field: 'pool_id', label: 'Pool ID', placeholder: 'e.g., 1' },
    ],
  },
  'pool-points': {
    label: 'Pool Points',
    icon: '⭐',
    description: 'Pool points by holder',
    defaultColumns: ['chain_id', 'address', 'pool_id', 'points', 'height'],
    filterableFields: [
      { field: 'address', label: 'Holder Address', placeholder: 'e.g., canopy1abc...' },
      { field: 'pool_id', label: 'Pool ID', placeholder: 'e.g., 1' },
    ],
  },
  'orders': {
    label: 'Orders',
    icon: '📋',
    description: 'Order book entries',
    defaultColumns: ['chain_id', 'order_id', 'order_type', 'amount', 'price', 'status', 'height'],
    filterableFields: [
      { field: 'order_id', label: 'Order ID', placeholder: 'e.g., 12345' },
      { field: 'status', label: 'Status', placeholder: 'e.g., open, filled, cancelled' },
    ],
  },
  'dex-orders': {
    label: 'DEX Orders',
    icon: '💱',
    description: 'DEX swap orders',
    defaultColumns: ['chain_id', 'order_id', 'sender', 'amount_in', 'amount_out', 'height'],
    filterableFields: [
      { field: 'sender', label: 'Sender Address', placeholder: 'e.g., canopy1abc...' },
      { field: 'order_id', label: 'Order ID', placeholder: 'e.g., 12345' },
    ],
  },
  'dex-deposits': {
    label: 'DEX Deposits',
    icon: '📥',
    description: 'DEX liquidity deposits',
    defaultColumns: ['chain_id', 'order_id', 'sender', 'deposit_amount', 'height'],
    filterableFields: [
      { field: 'sender', label: 'Sender Address', placeholder: 'e.g., canopy1abc...' },
      { field: 'order_id', label: 'Order ID', placeholder: 'e.g., 12345' },
    ],
  },
  'dex-withdrawals': {
    label: 'DEX Withdrawals',
    icon: '📤',
    description: 'DEX liquidity withdrawals',
    defaultColumns: ['chain_id', 'order_id', 'sender', 'withdrawal_amount', 'height'],
    filterableFields: [
      { field: 'sender', label: 'Sender Address', placeholder: 'e.g., canopy1abc...' },
      { field: 'order_id', label: 'Order ID', placeholder: 'e.g., 12345' },
    ],
  },
  'block-summaries': {
    label: 'Block Summaries',
    icon: '🔗',
    description: 'Block information across chains',
    defaultColumns: ['chain_id', 'height', 'block_time', 'tx_count', 'proposer'],
    filterableFields: [
      { field: 'proposer', label: 'Proposer Address', placeholder: 'e.g., canopyvalcons1...' },
    ],
  },
}

export default function CrossChainPage() {
  // State
  const [availableEntities, setAvailableEntities] = useState<EntityInfo[]>([])
  const [selectedEntity, setSelectedEntity] = useState<string>('accounts')
  const [chainFilter, setChainFilter] = useState<string>('')
  const [fieldFilters, setFieldFilters] = useState<Record<string, string>>({})
  const [data, setData] = useState<Record<string, any>[]>([])
  const [metadata, setMetadata] = useState<QueryMetadata | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>('')
  const [pageSize, setPageSize] = useState(50)
  const [currentOffset, setCurrentOffset] = useState(0)
  const [orderBy, setOrderBy] = useState('height')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc')
  const { notify } = useToast()

  // Fetch available entities on mount
  useEffect(() => {
    const fetchEntities = async () => {
      try {
        const res = await apiFetch('/api/crosschain/entities')
        if (!res.ok) throw new Error('Failed to fetch entities')
        const data: EntityListResponse = await res.json()
        setAvailableEntities(data.entities)
      } catch (err: any) {
        console.error('Failed to fetch entities:', err)
        notify('Failed to load available entities', 'error')
      }
    }
    fetchEntities()
  }, [notify])

  // Fetch entity data
  const fetchData = useCallback(async () => {
    setLoading(true)
    setError('')

    try {
      const params = new URLSearchParams()
      if (chainFilter) params.set('chain_id', chainFilter)

      // Add field-based filters with "filter_" prefix
      Object.entries(fieldFilters).forEach(([field, value]) => {
        if (value) {
          params.set(`filter_${field}`, value)
        }
      })

      params.set('limit', pageSize.toString())
      params.set('offset', currentOffset.toString())
      params.set('order_by', orderBy)
      params.set('sort', sortOrder)

      const res = await apiFetch(`/api/crosschain/entities/${selectedEntity}?${params.toString()}`)
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}))
        throw new Error(errData.error || 'Failed to fetch data')
      }

      const result: EntityDataResponse = await res.json()
      setData(result.data)
      setMetadata({
        total: result.total,
        limit: result.limit,
        offset: result.offset,
        has_more: result.has_more,
      })
    } catch (err: any) {
      console.error('Failed to fetch entity data:', err)
      setError(err.message || 'Failed to fetch data')
      setData([])
      setMetadata(null)
    } finally {
      setLoading(false)
    }
  }, [selectedEntity, chainFilter, fieldFilters, pageSize, currentOffset, orderBy, sortOrder])

  // Fetch data when parameters change
  useEffect(() => {
    fetchData()
  }, [fetchData])

  // Handle entity change
  const handleEntityChange = (entityName: string) => {
    setSelectedEntity(entityName)
    setCurrentOffset(0) // Reset pagination
    setFieldFilters({}) // Reset field filters
    setData([])
    setMetadata(null)
    setError('')
  }

  // Handle page navigation
  const handlePreviousPage = () => {
    if (currentOffset > 0) {
      setCurrentOffset(Math.max(0, currentOffset - pageSize))
    }
  }

  const handleNextPage = () => {
    if (metadata?.has_more) {
      setCurrentOffset(currentOffset + pageSize)
    }
  }

  // Handle sorting
  const handleSort = (column: string) => {
    if (orderBy === column) {
      // Toggle sort order
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')
    } else {
      setOrderBy(column)
      setSortOrder('desc')
    }
    setCurrentOffset(0) // Reset to first page
  }

  // Export functionality
  const handleExport = (format: 'csv' | 'json') => {
    if (!data || data.length === 0) {
      notify('No data to export', 'error')
      return
    }

    try {
      let content: string
      let mimeType: string
      let filename: string

      if (format === 'json') {
        content = JSON.stringify(data, null, 2)
        mimeType = 'application/json'
        filename = `cross-chain-${selectedEntity}-${Date.now()}.json`
      } else {
        // CSV export
        const headers = Object.keys(data[0])
        const csvRows = [
          headers.join(','),
          ...data.map((row) =>
            headers.map((header) => {
              const value = row[header]
              // Escape quotes and wrap in quotes if contains comma
              const stringValue = value === null || value === undefined ? '' : String(value)
              return stringValue.includes(',') || stringValue.includes('"')
                ? `"${stringValue.replace(/"/g, '""')}"`
                : stringValue
            }).join(',')
          ),
        ]
        content = csvRows.join('\n')
        mimeType = 'text/csv'
        filename = `cross-chain-${selectedEntity}-${Date.now()}.csv`
      }

      const blob = new Blob([content], { type: mimeType })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = filename
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(url)

      notify(`Exported ${data.length} records as ${format.toUpperCase()}`)
    } catch (err) {
      console.error('Export error:', err)
      notify('Failed to export data', 'error')
    }
  }

  // Get chain badge color
  const getChainBadgeColor = (chainId: number) => {
    const colors = [
      'bg-indigo-500/20 text-indigo-300 border-indigo-500/30',
      'bg-purple-500/20 text-purple-300 border-purple-500/30',
      'bg-pink-500/20 text-pink-300 border-pink-500/30',
      'bg-blue-500/20 text-blue-300 border-blue-500/30',
      'bg-green-500/20 text-green-300 border-green-500/30',
      'bg-yellow-500/20 text-yellow-300 border-yellow-500/30',
      'bg-red-500/20 text-red-300 border-red-500/30',
      'bg-teal-500/20 text-teal-300 border-teal-500/30',
    ]
    return colors[chainId % colors.length]
  }

  // Format column name for display
  const formatColumnName = (column: string) => {
    return column
      .replace(/_/g, ' ')
      .replace(/\b\w/g, (l) => l.toUpperCase())
  }

  // Format cell value
  const formatCellValue = (value: any, column: string) => {
    if (value === null || value === undefined) {
      return <span className="text-slate-500">—</span>
    }

    // Format timestamps
    if (column.includes('time') && typeof value === 'string') {
      try {
        const date = new Date(value)
        return date.toLocaleString()
      } catch {
        return value
      }
    }

    // Format large numbers
    if (typeof value === 'number' && value > 1000) {
      return value.toLocaleString()
    }

    // Truncate long strings (addresses, hashes)
    if (typeof value === 'string' && value.length > 50) {
      return (
        <span className="font-mono text-xs" title={value}>
          {value.substring(0, 20)}...{value.substring(value.length - 20)}
        </span>
      )
    }

    return <span className="font-mono text-sm">{String(value)}</span>
  }

  const entityConfig = ENTITY_UI_CONFIG[selectedEntity]
  const displayColumns = data.length > 0 ? Object.keys(data[0]) : []
  const currentPage = Math.floor(currentOffset / pageSize) + 1
  const totalPages = metadata ? Math.ceil(metadata.total / pageSize) : 0

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Cross-Chain Explorer</h1>
          <p className="mt-2 text-slate-400">
            Query and analyze data across all blockchain chains
          </p>
        </div>

        {/* Export buttons */}
        {data && data.length > 0 && (
          <div className="flex gap-2">
            <button
              onClick={() => handleExport('csv')}
              className="btn-secondary text-xs"
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                />
              </svg>
              Export CSV
            </button>
            <button
              onClick={() => handleExport('json')}
              className="btn-secondary text-xs"
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                />
              </svg>
              Export JSON
            </button>
          </div>
        )}
      </div>

      {/* Entity Selector */}
      <div className="card">
        <h3 className="mb-4 text-lg font-semibold text-white">Select Entity Type</h3>
        <div className="grid gap-3 md:grid-cols-3 lg:grid-cols-4">
          {Object.entries(ENTITY_UI_CONFIG).map(([key, config]) => (
            <button
              key={key}
              onClick={() => handleEntityChange(key)}
              className={`flex items-start gap-3 rounded-lg border p-4 text-left transition-all ${
                selectedEntity === key
                  ? 'border-indigo-500 bg-indigo-500/10'
                  : 'border-slate-700 bg-slate-800/30 hover:border-slate-600 hover:bg-slate-800/50'
              }`}
            >
              <span className="text-3xl">{config.icon}</span>
              <div className="flex-1 min-w-0">
                <div className={`font-medium ${selectedEntity === key ? 'text-white' : 'text-slate-300'}`}>
                  {config.label}
                </div>
                <div className="mt-1 text-xs text-slate-500">
                  {config.description}
                </div>
              </div>
            </button>
          ))}
        </div>
      </div>

      {/* Filters and Controls */}
      <div className="card">
        <h3 className="mb-4 text-lg font-semibold text-white">Filters and Options</h3>
        <div className="grid gap-4 md:grid-cols-3">
          <div>
            <label className="mb-2 block text-sm font-medium text-slate-300">
              Filter by Chain IDs
            </label>
            <input
              type="text"
              value={chainFilter}
              onChange={(e) => {
                setChainFilter(e.target.value)
                setCurrentOffset(0) // Reset to first page
              }}
              className="input"
              placeholder="e.g., 1,2,3 or leave empty for all"
            />
            <p className="mt-1 text-xs text-slate-500">Comma-separated chain IDs</p>
          </div>

          <div>
            <label className="mb-2 block text-sm font-medium text-slate-300">
              Page Size
            </label>
            <select
              value={pageSize}
              onChange={(e) => {
                setPageSize(Number(e.target.value))
                setCurrentOffset(0) // Reset to first page
              }}
              className="input"
            >
              <option value="10">10 per page</option>
              <option value="25">25 per page</option>
              <option value="50">50 per page</option>
              <option value="100">100 per page</option>
              <option value="250">250 per page</option>
            </select>
          </div>

          <div>
            <label className="mb-2 block text-sm font-medium text-slate-300">
              Sort By
            </label>
            <div className="flex gap-2">
              <select
                value={orderBy}
                onChange={(e) => {
                  setOrderBy(e.target.value)
                  setCurrentOffset(0) // Reset to first page
                }}
                className="input flex-1"
              >
                {displayColumns.map((col) => (
                  <option key={col} value={col}>
                    {formatColumnName(col)}
                  </option>
                ))}
              </select>
              <button
                onClick={() => {
                  setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')
                  setCurrentOffset(0)
                }}
                className="btn-secondary px-3"
                title={sortOrder === 'asc' ? 'Ascending' : 'Descending'}
              >
                {sortOrder === 'asc' ? '↑' : '↓'}
              </button>
            </div>
          </div>
        </div>

        {/* Entity-specific field filters */}
        {entityConfig?.filterableFields && entityConfig.filterableFields.length > 0 && (
          <div className="mt-4 border-t border-slate-700 pt-4">
            <h4 className="mb-3 text-sm font-semibold text-slate-300">Field Filters</h4>
            <div className="grid gap-4 md:grid-cols-3">
              {entityConfig.filterableFields.map((filter) => (
                <div key={filter.field}>
                  <label className="mb-2 block text-sm font-medium text-slate-300">
                    {filter.label}
                  </label>
                  <input
                    type="text"
                    value={fieldFilters[filter.field] || ''}
                    onChange={(e) => {
                      setFieldFilters({
                        ...fieldFilters,
                        [filter.field]: e.target.value,
                      })
                      setCurrentOffset(0) // Reset to first page
                    }}
                    className="input"
                    placeholder={filter.placeholder}
                  />
                </div>
              ))}
              {Object.keys(fieldFilters).some((key) => fieldFilters[key]) && (
                <div className="flex items-end">
                  <button
                    onClick={() => {
                      setFieldFilters({})
                      setCurrentOffset(0)
                    }}
                    className="btn-secondary text-xs"
                  >
                    Clear Field Filters
                  </button>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Error Display */}
      {error && (
        <div className="rounded-xl border border-rose-500/50 bg-rose-500/10 p-6 text-rose-200">
          <div className="flex items-start gap-3">
            <svg className="h-5 w-5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <div>
              <h4 className="font-semibold">Query Error</h4>
              <p className="mt-1 text-sm">{error}</p>
            </div>
          </div>
        </div>
      )}

      {/* Results Display */}
      {!error && (
        <div className="card">
          <div className="mb-6 flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold text-white">
                {entityConfig?.label || 'Results'}
              </h3>
              {metadata && (
                <p className="mt-1 text-sm text-slate-400">
                  Showing {currentOffset + 1}-{Math.min(currentOffset + pageSize, metadata.total)} of{' '}
                  {metadata.total.toLocaleString()} total records
                </p>
              )}
            </div>

            {/* Pagination Controls */}
            {metadata && metadata.total > pageSize && (
              <div className="flex items-center gap-2">
                <button
                  onClick={handlePreviousPage}
                  disabled={currentOffset === 0}
                  className="btn-secondary text-xs disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  ← Previous
                </button>
                <span className="text-sm text-slate-400">
                  Page {currentPage} of {totalPages}
                </span>
                <button
                  onClick={handleNextPage}
                  disabled={!metadata.has_more}
                  className="btn-secondary text-xs disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Next →
                </button>
              </div>
            )}
          </div>

          {/* Loading State */}
          {loading && (
            <div className="flex items-center justify-center py-12">
              <div className="flex items-center gap-3">
                <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
                <p className="text-slate-400">Loading data...</p>
              </div>
            </div>
          )}

          {/* Data Table */}
          {!loading && data.length > 0 && (
            <div className="overflow-x-auto">
              <table className="table">
                <thead>
                  <tr>
                    {displayColumns.map((col) => (
                      <th
                        key={col}
                        className={col !== 'chain_id' ? 'cursor-pointer hover:bg-slate-800/50' : ''}
                        onClick={() => col !== 'chain_id' && handleSort(col)}
                      >
                        <div className="flex items-center gap-2">
                          {formatColumnName(col)}
                          {orderBy === col && (
                            <span className="text-indigo-400">
                              {sortOrder === 'asc' ? '↑' : '↓'}
                            </span>
                          )}
                        </div>
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {data.map((row, idx) => (
                    <tr key={idx}>
                      {displayColumns.map((col) => (
                        <td key={col}>
                          {col === 'chain_id' ? (
                            <span className={`badge border ${getChainBadgeColor(row[col])}`}>
                              Chain {row[col]}
                            </span>
                          ) : (
                            formatCellValue(row[col], col)
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Empty State */}
          {!loading && data.length === 0 && !error && (
            <div className="flex flex-col items-center justify-center py-12">
              <div className="flex h-16 w-16 items-center justify-center rounded-full bg-slate-800/50">
                <svg className="h-8 w-8 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
                  />
                </svg>
              </div>
              <h3 className="mt-4 text-lg font-semibold text-slate-300">No Data Found</h3>
              <p className="mt-2 text-center text-sm text-slate-500">
                No records found for the selected filters. Try adjusting your filter criteria.
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
