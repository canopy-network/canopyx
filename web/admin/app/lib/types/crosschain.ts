// Cross-Chain API Type Definitions

/**
 * Information about an available cross-chain entity
 */
export interface EntityInfo {
  name: string
  table_name: string
  global_table: string
  route: string
}

/**
 * Response from GET /api/crosschain/entities
 */
export interface EntityListResponse {
  entities: EntityInfo[]
}

/**
 * Metadata for paginated query results
 */
export interface QueryMetadata {
  total: number
  limit: number
  offset: number
  has_more: boolean
}

/**
 * Response from GET /api/crosschain/entities/{entity}
 */
export interface EntityDataResponse {
  data: Record<string, any>[]
  total: number
  limit: number
  offset: number
  has_more: boolean
}

/**
 * Schema information for a cross-chain entity
 * Response from GET /api/crosschain/entities/{entity}/schema
 */
export interface EntitySchemaResponse {
  entity: string
  global_table: string
  primary_key: string[]
}

/**
 * Health status for cross-chain sync system
 * Response from GET /api/crosschain/health
 */
export interface CrossChainHealthResponse {
  database_name: string
  total_chains_synced: number
  max_lag: number
  tables_healthy: number
  tables_unhealthy: number
  details: ChainHealthDetail[]
  checked_at: string
}

export interface ChainHealthDetail {
  chain_id: number
  tables_healthy: number
  tables_with_lag: number
  max_lag: number
}

/**
 * Sync status for a specific chain and table
 * Response from GET /api/crosschain/sync-status/{chainID}/{table}
 */
export interface SyncStatusResponse {
  chain_id: number
  table_name: string
  source_row_count: number
  global_row_count: number
  lag: number
  last_source_update: string
  last_global_update: string
  materialized_view: string
  materialized_view_exists: boolean
}

/**
 * Cross-chain account query response
 * Response from GET /api/accounts/{address}/cross-chain
 */
export interface CrossChainAccountResponse {
  address: string
  total_balance: number
  chains_count: number
  chain_balances: ChainBalance[]
}

export interface ChainBalance {
  chain_id: number
  address: string
  amount: number
  rewards: number
  slashes: number
  height: number
  height_time: string
  updated_at: string
}

/**
 * Query options for cross-chain entity queries
 */
export interface CrossChainQueryOptions {
  chain_id?: string // Comma-separated chain IDs
  limit?: number
  offset?: number
  order_by?: string
  sort?: 'asc' | 'desc'
}
