curl -X POST http://localhost:50002/v1/query/cert-by-height -d '{"height": 1}'
GET FROM HERE THE CHAIN ID

GET FROM STATE THE ROOT CHAIN ID (FATHER)

// turn of pool points at dex-batch / next-dex-batch

dex-orders

// query to get order=1 FINAL
order=1 height=2 state=future success=true amount=0
order=1 height=4 state=locked success=true amount=0
// by event type
// order=1 height=10 state=complete success=true|false amount=1000 (only on success)`


// LiquidityPoolAddend = uint64(2 * )

var (
MaxChainId          = uint64(math.MaxUint16 / 4)
HoldingPoolAddend   = uint64(1 * math.MaxUint16 / 4)
LiquidityPoolAddend = uint64(2 * math.MaxUint16 / 4)
Unused1PoolAddend   = uint64(3 * math.MaxUint16 / 4)
EscrowPoolAddend    = uint64(4 * math.MaxUint16 / 4)
Unused2PoolAddend   = uint64(5 * math.MaxUint16 / 4)
Unused3PoolAddend   = uint64(6 * math.MaxUint16 / 4)
Unused4PoolAddend   = uint64(7 * math.MaxUint16 / 4)
)

// POOLS

id=1 totalPoolPoints=xxx \
liquidity_pool_id=uint64(2 * math.MaxUint16 / 4) + (chain.id) \
holding_pool_id=uint64(1 * math.MaxUint16 / 4) + (chain.id) \
escrow_pool_id=uint64(1 * math.MaxUint16 / 4) + (chain.id) \
reward_pool_id=chain.id

// DEX POOL POINT 

address=abc height=2 points=1000 pool_id=pool.id \
    
    liquidity_pool_points=(....) // come from a new event <--
    liquidity_pool_id=uint64(2 * math.MaxUint16 / 4) + (chain.id) \

    holding_pool_points=(....) // come from a new event <-- 
    holding_pool_id=uint64(1 * math.MaxUint16 / 4) + (chain.id) \

    escrow_pool_points=(....) // come from a new event <--
    escrow_pool_id=uint64(1 * math.MaxUint16 / 4) + (chain.id) \

    reward_pool_points=(....) \ 
    reward_pool_id=chain.id
