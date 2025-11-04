[] Review:

Event Types from Canopy (9 total):
- Validator: EventReward, EventSlash, EventAutoPause, EventAutoBeginUnstaking,
  EventFinishUnstaking
- DEX/Trading: EventDexSwap, EventDexLiquidityDeposit, EventDexLiquidityWithdraw,
  EventOrderBookSwap

Activities that SHOULD use events but DON'T:
1. IndexAccounts - Should query: EventReward, EventSlash, EventFinishUnstaking
2. IndexValidators - Should query: All 5 validator events + EventReward, EventSlash
3. IndexOrders - Should query: EventOrderBookSwap
4. IndexPools - Should query: EventDexLiquidityDeposit, EventDexLiquidityWithdraw,
   EventDexSwap

---

[] Integration Tests (db layer)
[] Unit Tests (canopy rpc, admin api, workflows, activities)
