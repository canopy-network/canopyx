'use client'

import { useEffect, useState } from 'react'
import { apiFetch } from '../../lib/api'
import { useToast } from '../../components/ToastProvider'

// Types
type Chain = {
  chain_id: number
  chain_name: string
  deleted?: boolean
}

type EntityInfo = {
  name: string
  table_name: string
  route_path: string
}

type EntitiesResponse = {
  entities: EntityInfo[]
}

type SchemaColumn = {
  name: string
  type: string
}

type SchemaResponse = {
  columns: SchemaColumn[]
}

type PaginatedResponse<T> = {
  data: T[]
  limit: number
  next_cursor: number | null
}

export default function ExplorerPage() {
  const { notify } = useToast()

  // Chain filter state
  const [chains, setChains] = useState<Chain[]>([])
  const [selectedChainIds, setSelectedChainIds] = useState<number[]>([])
  const [loadingChains, setLoadingChains] = useState(true)

  // Entities state
  const [entities, setEntities] = useState<EntityInfo[]>([])
  const [loadingEntities, setLoadingEntities] = useState(true)

  // Table browsing state
  const [selectedTable, setSelectedTable] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>('')
  const [schema, setSchema] = useState<SchemaColumn[]>([])
  const [data, setData] = useState<any[]>([])
  const [nextCursor, setNextCursor] = useState<number | null>(null)
  const [cursors, setCursors] = useState<(number | null)[]>([null])
  const [currentPageIndex, setCurrentPageIndex] = useState(0)
  const [itemsPerPage, setItemsPerPage] = useState(50)
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc')

  // Fetch available chains
  useEffect(() => {
    const fetchChains = async () => {
      setLoadingChains(true)
      try {
        const response = await apiFetch('/api/chains')
        if (response.ok) {
          const data = await response.json()
          setChains(data.filter((c: Chain) => !c.deleted))
        }
      } catch (err) {
        console.error('Failed to fetch chains:', err)
      } finally {
        setLoadingChains(false)
      }
    }
    fetchChains()
  }, [])

  // Fetch entities on mount
  useEffect(() => {
    const fetchEntities = async () => {
      setLoadingEntities(true)
      try {
        const response = await apiFetch('/api/entities')
        if (!response.ok) {
          throw new Error('Failed to fetch entities')
        }
        const data: EntitiesResponse = await response.json()
        setEntities(data.entities)

        if (data.entities.length > 0) {
          setSelectedTable(data.entities[0].name)
        }
      } catch (err: any) {
        console.error('Entities fetch error:', err)
        notify('Failed to load entities list', 'error')
        // Fallback
        const fallbackEntities: EntityInfo[] = [
          { name: 'blocks', table_name: 'blocks', route_path: 'blocks' },
          { name: 'block_summaries', table_name: 'block_summaries', route_path: 'block-summaries' },
          { name: 'transactions', table_name: 'transactions', route_path: 'transactions' },
          { name: 'accounts', table_name: 'accounts', route_path: 'accounts' },
        ]
        setEntities(fallbackEntities)
        setSelectedTable('blocks')
      } finally {
        setLoadingEntities(false)
      }
    }

    fetchEntities()
  }, [notify])

  // Fetch schema when table changes
  useEffect(() => {
    if (!selectedTable) return

    const fetchSchema = async () => {
      setLoading(true)
      setError('')
      try {
        // Use global explorer endpoint for schema (no chain_id needed)
        const response = await apiFetch(
          `/api/explorer/schema?table=${selectedTable}`
        )

        if (!response.ok) {
          throw new Error(`Failed to fetch schema: ${response.statusText}`)
        }

        const schemaData: SchemaResponse = await response.json()
        setSchema(schemaData.columns)
      } catch (err: any) {
        console.error('Schema fetch error:', err)
        setError(err.message || 'Failed to load schema')
        setSchema([])
      } finally {
        setLoading(false)
      }
    }

    fetchSchema()
  }, [selectedTable])

  // Fetch data when table, page, or chain filter changes
  useEffect(() => {
    const fetchData = async () => {
      if (schema.length === 0) return

      setLoading(true)
      setError('')
      try {
        const cursor = cursors[currentPageIndex]
        const cursorParam = cursor !== null ? `&cursor=${cursor}` : ''

        // Build chain_ids parameter for filtering (optional)
        const chainIdsParam = selectedChainIds.length > 0
          ? `&chain_ids=${selectedChainIds.join(',')}`
          : ''

        // Use global explorer endpoint (no chain_id in path)
        const response = await apiFetch(
          `/api/explorer/entity/${selectedTable}?limit=${itemsPerPage}&sort=${sortOrder}${cursorParam}${chainIdsParam}`
        )

        if (!response.ok) {
          throw new Error(`Failed to fetch data: ${response.statusText}`)
        }

        const result: PaginatedResponse<any> = await response.json()
        setData(result.data || [])
        setNextCursor(result.next_cursor ?? null)
      } catch (err: any) {
        console.error('Data fetch error:', err)
        setError(err.message || 'Failed to load data')
        setData([])
        setNextCursor(null)
      } finally {
        setLoading(false)
      }
    }

    fetchData()
  }, [selectedTable, currentPageIndex, cursors, schema.length, itemsPerPage, sortOrder, selectedChainIds])

  const handleTableChange = (newTable: string) => {
    setSelectedTable(newTable)
    setCursors([null])
    setCurrentPageIndex(0)
    setData([])
    setNextCursor(null)
    setError('')
  }

  const handleChainToggle = (chainId: number) => {
    setSelectedChainIds(prev => {
      if (prev.includes(chainId)) {
        return prev.filter(id => id !== chainId)
      } else {
        return [...prev, chainId]
      }
    })
    // Reset pagination when chain filter changes
    setCursors([null])
    setCurrentPageIndex(0)
  }

  const handleSelectAllChains = () => {
    if (selectedChainIds.length === chains.length) {
      setSelectedChainIds([])
    } else {
      setSelectedChainIds(chains.map(c => c.chain_id))
    }
    setCursors([null])
    setCurrentPageIndex(0)
  }

  const handleClearChainFilter = () => {
    setSelectedChainIds([])
    setCursors([null])
    setCurrentPageIndex(0)
  }

  const handleItemsPerPageChange = (newLimit: number) => {
    setItemsPerPage(newLimit)
    setCursors([null])
    setCurrentPageIndex(0)
  }

  const handleNextPage = () => {
    if (nextCursor !== null) {
      const newCursors = [...cursors.slice(0, currentPageIndex + 1), nextCursor]
      setCursors(newCursors)
      setCurrentPageIndex(currentPageIndex + 1)
    }
  }

  const handlePreviousPage = () => {
    if (currentPageIndex > 0) {
      setCurrentPageIndex(currentPageIndex - 1)
    }
  }

  // Get column names for display
  const columnNames = schema.map(col => col.name)

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Global Explorer</h1>
          <p className="mt-1 text-sm text-slate-400">
            Browse data across all chains or filter by specific chains
          </p>
        </div>
      </div>

      {/* Chain Filter */}
      <div className="card">
        <div className="card-header">
          <div className="flex items-center gap-2">
            <svg className="h-5 w-5 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
            </svg>
            <h3 className="card-title">Chain Filter</h3>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={handleClearChainFilter}
              className="btn-secondary text-xs"
              disabled={selectedChainIds.length === 0}
            >
              Clear
            </button>
            <button
              onClick={handleSelectAllChains}
              className="btn-secondary text-xs"
            >
              {selectedChainIds.length === chains.length ? 'Deselect All' : 'Select All'}
            </button>
          </div>
        </div>

        {loadingChains ? (
          <div className="flex items-center gap-2 text-slate-400">
            <div className="h-4 w-4 animate-spin rounded-full border-2 border-slate-700 border-t-indigo-500"></div>
            Loading chains...
          </div>
        ) : chains.length === 0 ? (
          <p className="text-sm text-slate-500">No chains available</p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {/* All Chains option */}
            <button
              onClick={handleClearChainFilter}
              className={`rounded-lg border px-3 py-1.5 text-sm font-medium transition-all ${
                selectedChainIds.length === 0
                  ? 'border-indigo-500 bg-indigo-500/20 text-indigo-300'
                  : 'border-slate-700 bg-slate-800/50 text-slate-400 hover:border-slate-600 hover:text-slate-300'
              }`}
            >
              All Chains
            </button>

            {chains.map(chain => (
              <button
                key={chain.chain_id}
                onClick={() => handleChainToggle(chain.chain_id)}
                className={`rounded-lg border px-3 py-1.5 text-sm font-medium transition-all ${
                  selectedChainIds.includes(chain.chain_id)
                    ? 'border-indigo-500 bg-indigo-500/20 text-indigo-300'
                    : 'border-slate-700 bg-slate-800/50 text-slate-400 hover:border-slate-600 hover:text-slate-300'
                }`}
              >
                {chain.chain_name || `Chain ${chain.chain_id}`}
                <span className="ml-1.5 text-xs text-slate-500">#{chain.chain_id}</span>
              </button>
            ))}
          </div>
        )}

        {/* Active filter indicator */}
        {selectedChainIds.length > 0 && (
          <div className="mt-3 flex items-center gap-2 rounded-lg border border-indigo-500/30 bg-indigo-500/10 px-3 py-2">
            <svg className="h-4 w-4 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span className="text-xs text-indigo-300">
              Filtering by {selectedChainIds.length} chain{selectedChainIds.length > 1 ? 's' : ''}:
              {' '}
              <span className="font-mono">
                chain_id IN ({selectedChainIds.join(', ')})
              </span>
            </span>
          </div>
        )}
      </div>

      {/* Error Banner */}
      {error && (
        <div className="rounded-lg border border-rose-500/50 bg-rose-500/10 p-4 text-rose-200">
          <div className="flex items-start gap-3">
            <svg className="h-5 w-5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <div>
              <p className="font-semibold">Error Loading Data</p>
              <p className="mt-1 text-sm text-rose-300">{error}</p>
            </div>
          </div>
        </div>
      )}

      {/* Entity Selector */}
      {!loadingEntities && entities.length > 0 && (
        <div className="card">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-3">
              <label className="text-sm font-medium text-slate-300 whitespace-nowrap">Entity:</label>
              <select
                value={selectedTable}
                onChange={(e) => handleTableChange(e.target.value)}
                className="input w-auto min-w-[200px]"
                disabled={loading || loadingEntities}
              >
                {entities.map((entity) => (
                  <option key={entity.name} value={entity.name}>
                    {entity.name}
                  </option>
                ))}
              </select>
            </div>
          </div>
        </div>
      )}

      {/* Table Controls */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <label className="text-sm font-medium text-slate-300">Items per page:</label>
          <select
            value={itemsPerPage}
            onChange={(e) => handleItemsPerPageChange(Number(e.target.value))}
            className="input w-auto"
            disabled={loading}
          >
            <option value={10}>10</option>
            <option value={25}>25</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
          </select>

          <div className="h-6 w-px bg-slate-700"></div>

          <button
            onClick={() => setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')}
            className="btn-secondary text-sm"
            disabled={loading}
            title={`Sort ${sortOrder === 'asc' ? 'descending' : 'ascending'}`}
          >
            <svg
              className={`h-4 w-4 transition-transform ${sortOrder === 'desc' ? 'rotate-180' : ''}`}
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4h13M3 8h9m-9 4h6m4 0l4-4m0 0l4 4m-4-4v12" />
            </svg>
            {sortOrder === 'asc' ? 'Oldest First' : 'Newest First'}
          </button>
        </div>

        {/* Pagination Controls */}
        <div className="flex items-center gap-2">
          <button
            onClick={handlePreviousPage}
            disabled={currentPageIndex === 0 || loading}
            className="btn-secondary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Previous
          </button>
          <span className="text-sm text-slate-400">
            Page {currentPageIndex + 1}
          </span>
          <button
            onClick={handleNextPage}
            disabled={nextCursor === null || loading}
            className="btn-secondary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Next
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
            </svg>
          </button>
        </div>
      </div>

      {/* Schema Display */}
      {schema.length > 0 && (
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">Schema: {selectedTable}</h3>
            <span className="text-xs text-slate-500">{schema.length} columns</span>
          </div>
          <div className="flex flex-wrap gap-2">
            {schema.map((col) => (
              <span key={col.name} className="badge-neutral" title={col.type}>
                {col.name}
                <span className="ml-1 text-slate-500 text-xs">({col.type})</span>
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Data Table */}
      <div className="card overflow-hidden p-0">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <div className="flex items-center gap-3">
              <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
              <p className="text-slate-400">Loading data...</p>
            </div>
          </div>
        ) : data.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-slate-500">
            <div className="text-center">
              <svg className="mx-auto h-16 w-16 text-slate-700" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
              </svg>
              <p className="mt-4 text-sm">No data available</p>
              <p className="mt-1 text-xs text-slate-600">
                {selectedChainIds.length === 0
                  ? 'Select a chain filter or browse all chains'
                  : 'This table may be empty for the selected chains'}
              </p>
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  {columnNames.map((col) => (
                    <th key={col}>{col}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.map((row, idx) => (
                  <tr key={idx}>
                    {columnNames.map((col) => (
                      <td key={col} className="font-mono text-xs">
                        {row[col] !== null && row[col] !== undefined
                          ? String(row[col])
                          : '—'}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Results Info */}
      {data.length > 0 && (
        <div className="text-center text-xs text-slate-500">
          Showing {data.length} {data.length === 1 ? 'row' : 'rows'}
          {nextCursor !== null && ' • More results available'}
        </div>
      )}
    </div>
  )
}