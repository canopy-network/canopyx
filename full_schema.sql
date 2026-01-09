-- Database: canopyx_indexer
-- Table: chains
CREATE TABLE canopyx_indexer.chains (
    `chain_id` UInt64,
    `chain_name` String,
    `rpc_endpoints` Array(String),
    `paused` UInt8 DEFAULT 0,
    `deleted` UInt8 DEFAULT 0,
    `image` String DEFAULT '',
    `min_replicas` UInt16 DEFAULT 1,
    `max_replicas` UInt16 DEFAULT 3,
    `notes` String DEFAULT '',
    `created_at` DateTime DEFAULT now(),
    `updated_at` DateTime DEFAULT now(),
    `rpc_health_status` String DEFAULT 'unknown',
    `rpc_health_message` String DEFAULT '',
    `rpc_health_updated_at` DateTime DEFAULT now(),
    `queue_health_status` String DEFAULT 'unknown',
    `queue_health_message` String DEFAULT '',
    `queue_health_updated_at` DateTime DEFAULT now(),
    `deployment_health_status` String DEFAULT 'unknown',
    `deployment_health_message` String DEFAULT '',
    `deployment_health_updated_at` DateTime DEFAULT now(),
    `overall_health_status` String DEFAULT 'unknown',
    `overall_health_updated_at` DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY chain_id
SETTINGS index_granularity = 8192;


-- Database: canopyx_indexer
-- Table: index_progress
CREATE TABLE canopyx_indexer.index_progress (
    `chain_id` UInt64,
    `height` UInt64,
    `indexed_at` DateTime64(6),
    `indexing_time` Float64,
    `indexing_time_ms` Float64,
    `indexing_detail` String
) ENGINE = MergeTree
ORDER BY (chain_id, height)
SETTINGS index_granularity = 8192;


-- Database: canopyx_indexer
-- Table: index_progress_agg
CREATE TABLE canopyx_indexer.index_progress_agg (
    `chain_id` UInt64,
    `max_height` AggregateFunction(max, UInt64)
) ENGINE = AggregatingMergeTree
ORDER BY chain_id
SETTINGS index_granularity = 8192;


-- Database: canopyx_indexer
-- Table: reindex_requests
CREATE TABLE canopyx_indexer.reindex_requests (
    `chain_id` UInt64,
    `height` UInt64,
    `requested_by` String,
    `status` String DEFAULT 'queued',
    `workflow_id` String,
    `run_id` String,
    `requested_at` DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(requested_at)
ORDER BY (chain_id, requested_at, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.006c3073-71b1-4d53-b204-9aa04ed44901
CREATE TABLE chain_1.`.inner_id.006c3073-71b1-4d53-b204-9aa04ed44901` (
    `order_id` String,
    `created_height` UInt64
) ENGINE = AggregatingMergeTree
ORDER BY order_id
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.25e6ca47-f9d3-481d-828e-d0d7bb63da24
CREATE TABLE chain_1.`.inner_id.25e6ca47-f9d3-481d-828e-d0d7bb63da24` (
    `address` String,
    `pool_id` UInt64,
    `created_height` UInt64
) ENGINE = AggregatingMergeTree
ORDER BY (address, pool_id)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.2bfde2bc-1ce0-4f12-9d3d-a2b819b5af71
CREATE TABLE chain_1.`.inner_id.2bfde2bc-1ce0-4f12-9d3d-a2b819b5af71` (
    `order_id` String,
    `created_height` UInt64
) ENGINE = AggregatingMergeTree
ORDER BY order_id
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.6f1c922d-e150-4df1-b359-182d600298d9
CREATE TABLE chain_1.`.inner_id.6f1c922d-e150-4df1-b359-182d600298d9` (
    `height` UInt64,
    `height_time` DateTime64(6)
) ENGINE = AggregatingMergeTree
ORDER BY height
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.70ebb028-cb41-480d-af5e-eb74701e5128
CREATE TABLE chain_1.`.inner_id.70ebb028-cb41-480d-af5e-eb74701e5128` (
    `chain_id` UInt64,
    `created_height` UInt64,
    `height_time` DateTime64(6)
) ENGINE = AggregatingMergeTree
ORDER BY chain_id
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.9e0d8105-fdfc-4a44-a3f0-18a2187b839f
CREATE TABLE chain_1.`.inner_id.9e0d8105-fdfc-4a44-a3f0-18a2187b839f` (
    `order_id` String,
    `created_height` UInt64
) ENGINE = AggregatingMergeTree
ORDER BY order_id
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.a271a878-d500-49f6-8aea-e4bea1f728b0
CREATE TABLE chain_1.`.inner_id.a271a878-d500-49f6-8aea-e4bea1f728b0` (
    `address` String,
    `created_height` UInt64
) ENGINE = AggregatingMergeTree
ORDER BY address
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.ee982158-5ef1-40df-a4bb-52bbcc1cd038
CREATE TABLE chain_1.`.inner_id.ee982158-5ef1-40df-a4bb-52bbcc1cd038` (
    `address` String,
    `created_height` UInt64
) ENGINE = AggregatingMergeTree
ORDER BY address
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: .inner_id.f84f6873-2ab2-4fbd-8946-af86c4de24a1
CREATE TABLE chain_1.`.inner_id.f84f6873-2ab2-4fbd-8946-af86c4de24a1` (
    `order_id` String,
    `created_height` UInt64
) ENGINE = AggregatingMergeTree
ORDER BY order_id
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: accounts
CREATE TABLE chain_1.accounts (
    `address` String,
    `amount` UInt64),
    `height` UInt64,
    `height_time` DateTime64(6)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (address, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: block_summaries
CREATE TABLE chain_1.block_summaries (
    `height` UInt64,
    `height_time` DateTime64(6),
    `num_txs` UInt32 DEFAULT 0),
    `num_txs_send` UInt32 DEFAULT 0),
    `num_txs_delegate` UInt32 DEFAULT 0),
    `num_txs_undelegate` UInt32 DEFAULT 0),
    `num_txs_stake` UInt32 DEFAULT 0),
    `num_txs_unstake` UInt32 DEFAULT 0),
    `num_txs_edit_stake` UInt32 DEFAULT 0),
    `num_txs_vote` UInt32 DEFAULT 0),
    `num_txs_proposal` UInt32 DEFAULT 0),
    `num_txs_contract` UInt32 DEFAULT 0),
    `num_txs_system` UInt32 DEFAULT 0),
    `num_txs_unknown` UInt32 DEFAULT 0),
    `num_txs_pause` UInt32 DEFAULT 0),
    `num_txs_unpause` UInt32 DEFAULT 0),
    `num_txs_change_parameter` UInt32 DEFAULT 0),
    `num_txs_dao_transfer` UInt32 DEFAULT 0),
    `num_txs_certificate_results` UInt32 DEFAULT 0),
    `num_txs_subsidy` UInt32 DEFAULT 0),
    `num_txs_create_order` UInt32 DEFAULT 0),
    `num_txs_edit_order` UInt32 DEFAULT 0),
    `num_txs_delete_order` UInt32 DEFAULT 0),
    `num_txs_dex_limit_order` UInt32 DEFAULT 0),
    `num_txs_dex_liquidity_deposit` UInt32 DEFAULT 0),
    `num_txs_dex_liquidity_withdraw` UInt32 DEFAULT 0),
    `num_accounts` UInt32 DEFAULT 0),
    `num_accounts_new` UInt32 DEFAULT 0),
    `num_events` UInt32 DEFAULT 0),
    `num_events_reward` UInt32 DEFAULT 0),
    `num_events_slash` UInt32 DEFAULT 0),
    `num_events_dex_liquidity_deposit` UInt32 DEFAULT 0),
    `num_events_dex_liquidity_withdraw` UInt32 DEFAULT 0),
    `num_events_dex_swap` UInt32 DEFAULT 0),
    `num_events_order_book_swap` UInt32 DEFAULT 0),
    `num_events_automatic_pause` UInt32 DEFAULT 0),
    `num_events_automatic_begin_unstaking` UInt32 DEFAULT 0),
    `num_events_automatic_finish_unstaking` UInt32 DEFAULT 0),
    `num_orders` UInt32 DEFAULT 0),
    `num_orders_new` UInt32 DEFAULT 0),
    `num_orders_open` UInt32 DEFAULT 0),
    `num_orders_filled` UInt32 DEFAULT 0),
    `num_orders_cancelled` UInt32 DEFAULT 0),
    `num_orders_expired` UInt32 DEFAULT 0),
    `num_pools` UInt32 DEFAULT 0),
    `num_pools_new` UInt32 DEFAULT 0),
    `num_dex_prices` UInt32 DEFAULT 0),
    `num_dex_orders` UInt32 DEFAULT 0),
    `num_dex_orders_future` UInt32 DEFAULT 0),
    `num_dex_orders_locked` UInt32 DEFAULT 0),
    `num_dex_orders_complete` UInt32 DEFAULT 0),
    `num_dex_orders_success` UInt32 DEFAULT 0),
    `num_dex_orders_failed` UInt32 DEFAULT 0),
    `num_dex_deposits` UInt32 DEFAULT 0),
    `num_dex_deposits_pending` UInt32 DEFAULT 0),
    `num_dex_deposits_complete` UInt32 DEFAULT 0),
    `num_dex_withdrawals` UInt32 DEFAULT 0),
    `num_dex_withdrawals_pending` UInt32 DEFAULT 0),
    `num_dex_withdrawals_complete` UInt32 DEFAULT 0),
    `num_pool_points_holders` UInt32 DEFAULT 0),
    `num_pool_points_holders_new` UInt32 DEFAULT 0),
    `params_changed` Bool DEFAULT false,
    `num_validators` UInt32 DEFAULT 0),
    `num_validators_new` UInt32 DEFAULT 0),
    `num_validators_active` UInt32 DEFAULT 0),
    `num_validators_paused` UInt32 DEFAULT 0),
    `num_validators_unstaking` UInt32 DEFAULT 0),
    `num_validator_signing_info` UInt32 DEFAULT 0),
    `num_validator_signing_info_new` UInt32 DEFAULT 0),
    `num_committees` UInt32 DEFAULT 0),
    `num_committees_new` UInt32 DEFAULT 0),
    `num_committees_subsidized` UInt32 DEFAULT 0),
    `num_committees_retired` UInt32 DEFAULT 0),
    `num_committee_validators` UInt32 DEFAULT 0),
    `num_poll_snapshots` UInt32 DEFAULT 0)
) ENGINE = ReplacingMergeTree(height)
ORDER BY height
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: blocks
CREATE TABLE chain_1.blocks (
    `height` UInt64,
    `hash` String,
    `time` DateTime64(6),
    `parent_hash` String,
    `proposer_address` String,
    `size` Int32
) ENGINE = ReplacingMergeTree(height)
ORDER BY height
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: committee_validators
CREATE TABLE chain_1.committee_validators (
    `committee_id` UInt64),
    `validator_address` String,
    `staked_amount` UInt64),
    `status` LowCardinality(String),
    `height` UInt64),
    `height_time` DateTime64(3)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (committee_id, validator_address, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: committees
CREATE TABLE chain_1.committees (
    `chain_id` UInt64),
    `last_root_height_updated` UInt64),
    `last_chain_height_updated` UInt64),
    `number_of_samples` UInt64),
    `subsidized` UInt8,
    `retired` UInt8,
    `height` UInt64),
    `height_time` DateTime64(6)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (chain_id, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: dex_deposits
CREATE TABLE chain_1.dex_deposits (
    `order_id` String,
    `height` UInt64,
    `height_time` DateTime64(6),
    `committee` UInt64),
    `address` String,
    `amount` UInt64),
    `state` LowCardinality(String),
    `local_origin` Bool,
    `points_received` UInt64)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (order_id, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: dex_orders
CREATE TABLE chain_1.dex_orders (
    `order_id` String,
    `height` UInt64,
    `height_time` DateTime64(6),
    `committee` UInt64),
    `address` String,
    `amount_for_sale` UInt64),
    `requested_amount` UInt64),
    `state` LowCardinality(String),
    `success` Bool,
    `sold_amount` UInt64),
    `bought_amount` UInt64),
    `local_origin` Bool,
    `locked_height` UInt64
) ENGINE = ReplacingMergeTree(height)
ORDER BY (order_id, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: dex_prices
CREATE TABLE chain_1.dex_prices (
    `local_chain_id` UInt64,
    `remote_chain_id` UInt64,
    `height` UInt64,
    `local_pool` UInt64,
    `remote_pool` UInt64,
    `price_e6` UInt64,
    `height_time` DateTime64(6)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (local_chain_id, remote_chain_id, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: dex_withdrawals
CREATE TABLE chain_1.dex_withdrawals (
    `order_id` String,
    `height` UInt64,
    `height_time` DateTime64(6),
    `committee` UInt64),
    `address` String,
    `percent` UInt64),
    `state` LowCardinality(String),
    `local_amount` UInt64),
    `remote_amount` UInt64),
    `points_burned` UInt64)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (order_id, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: events
CREATE TABLE chain_1.events (
    `height` UInt64,
    `chain_id` UInt64,
    `address` String,
    `reference` String,
    `event_type` LowCardinality(String),
    `amount` Nullable(UInt64),
    `sold_amount` Nullable(UInt64),
    `bought_amount` Nullable(UInt64),
    `local_amount` Nullable(UInt64),
    `remote_amount` Nullable(UInt64),
    `success` Nullable(Bool),
    `local_origin` Nullable(Bool),
    `order_id` Nullable(String),
    `msg` String,
    `height_time` DateTime64(6)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (height, chain_id, address, reference, event_type)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: genesis
CREATE TABLE chain_1.genesis (
    `height` UInt64,
    `data` String,
    `fetched_at` DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree
ORDER BY height
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: orders
CREATE TABLE chain_1.orders (
    `order_id` String,
    `height` UInt64,
    `height_time` DateTime64(6),
    `committee` UInt64),
    `amount_for_sale` UInt64),
    `requested_amount` UInt64),
    `seller_address` String,
    `buyer_address` Nullable(String),
    `deadline` Nullable(UInt64)),
    `status` LowCardinality(String)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (order_id, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: params
CREATE TABLE chain_1.params (
    `height` UInt64),
    `height_time` DateTime64(6),
    `block_size` UInt64),
    `protocol_version` String,
    `root_chain_id` UInt64),
    `retired` UInt64),
    `unstaking_blocks` UInt64),
    `max_pause_blocks` UInt64),
    `double_sign_slash_percentage` UInt64),
    `non_sign_slash_percentage` UInt64),
    `max_non_sign` UInt64),
    `non_sign_window` UInt64),
    `max_committees` UInt64),
    `max_committee_size` UInt64),
    `early_withdrawal_penalty` UInt64),
    `delegate_unstaking_blocks` UInt64),
    `minimum_order_size` UInt64),
    `stake_percent_for_subsidized` UInt64),
    `max_slash_per_committee` UInt64),
    `delegate_reward_percentage` UInt64),
    `buy_deadline_blocks` UInt64),
    `lock_order_fee_multiplier` UInt64),
    `send_fee` UInt64),
    `stake_fee` UInt64),
    `edit_stake_fee` UInt64),
    `unstake_fee` UInt64),
    `pause_fee` UInt64),
    `unpause_fee` UInt64),
    `change_parameter_fee` UInt64),
    `dao_transfer_fee` UInt64),
    `certificate_results_fee` UInt64),
    `subsidy_fee` UInt64),
    `create_order_fee` UInt64),
    `edit_order_fee` UInt64),
    `delete_order_fee` UInt64),
    `dex_limit_order_fee` UInt64),
    `dex_liquidity_deposit_fee` UInt64),
    `dex_liquidity_withdraw_fee` UInt64),
    `dao_reward_percentage` UInt64)
) ENGINE = ReplacingMergeTree(height)
ORDER BY height
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: poll_snapshots
CREATE TABLE chain_1.poll_snapshots (
    `proposal_hash` String,
    `height` UInt64),
    `proposal_url` String,
    `accounts_approve_tokens` UInt64),
    `accounts_reject_tokens` UInt64),
    `accounts_total_voted_tokens` UInt64),
    `accounts_total_tokens` UInt64),
    `accounts_approve_percentage` UInt64),
    `accounts_reject_percentage` UInt64),
    `accounts_voted_percentage` UInt64),
    `validators_approve_tokens` UInt64),
    `validators_reject_tokens` UInt64),
    `validators_total_voted_tokens` UInt64),
    `validators_total_tokens` UInt64),
    `validators_approve_percentage` UInt64),
    `validators_reject_percentage` UInt64),
    `validators_voted_percentage` UInt64),
    `height_time` DateTime64(6))
) ENGINE = ReplacingMergeTree(height)
ORDER BY (proposal_hash, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: pool_points_by_holder
CREATE TABLE chain_1.pool_points_by_holder (
    `address` String,
    `pool_id` UInt64),
    `height` UInt64,
    `height_time` DateTime64(6),
    `committee` UInt64),
    `points` UInt64),
    `liquidity_pool_points` UInt64),
    `liquidity_pool_id` UInt64)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (address, pool_id, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: pools
CREATE TABLE chain_1.pools (
    `pool_id` UInt64,
    `height` UInt64,
    `chain_id` UInt64,
    `amount` UInt64,
    `total_points` UInt64,
    `lp_count` UInt32,
    `height_time` DateTime64(6),
    `liquidity_pool_id` UInt64,
    `holding_pool_id` UInt64,
    `escrow_pool_id` UInt64,
    `reward_pool_id` UInt64
) ENGINE = ReplacingMergeTree(height)
ORDER BY (pool_id, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: txs
CREATE TABLE chain_1.txs (
    `height` UInt64,
    `tx_hash` String,
    `time` DateTime64(6),
    `height_time` DateTime64(6),
    `message_type` LowCardinality(String),
    `signer` String,
    `counterparty` Nullable(String),
    `amount` Nullable(UInt64),
    `fee` UInt64,
    `validator_address` Nullable(String),
    `commission` Nullable(Float64),
    `chain_id` Nullable(UInt64),
    `sell_amount` Nullable(UInt64),
    `buy_amount` Nullable(UInt64),
    `liquidity_amount` Nullable(UInt64),
    `order_id` Nullable(String),
    `price` Nullable(Float64),
    `param_key` Nullable(String),
    `param_value` Nullable(String),
    `committee_id` Nullable(UInt64),
    `recipient` Nullable(String),
    `msg` String,
    `public_key` Nullable(String),
    `signature` Nullable(String)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (height, tx_hash)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: validator_double_signing_info
CREATE TABLE chain_1.validator_double_signing_info (
    `address` String,
    `evidence_count` UInt64),
    `first_evidence_height` UInt64),
    `last_evidence_height` UInt64),
    `height` UInt64),
    `height_time` DateTime64(3)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (address, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: validator_signing_info
CREATE TABLE chain_1.validator_signing_info (
    `address` String,
    `missed_blocks_count` UInt64),
    `missed_blocks_window` UInt64),
    `last_signed_height` UInt64),
    `start_height` UInt64),
    `height` UInt64),
    `height_time` DateTime64(3)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (address, height)
SETTINGS index_granularity = 8192;


-- Database: chain_1
-- Table: validators
CREATE TABLE chain_1.validators (
    `address` String,
    `public_key` String,
    `net_address` String,
    `staked_amount` UInt64),
    `committees` Array(UInt64),
    `max_paused_height` UInt64),
    `unstaking_height` UInt64),
    `output` String,
    `delegate` UInt8 DEFAULT 0,
    `compound` UInt8 DEFAULT 0,
    `status` String,
    `height` UInt64),
    `height_time` DateTime64(3)
) ENGINE = ReplacingMergeTree(height)
ORDER BY (address, height)
SETTINGS index_granularity = 8192;


