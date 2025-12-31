- [x] committees table fields whith wrong values base on rpc mocks subsidized=1 -> subsidized=2, retired=0 -> retired=1
- [x] dex_deposits - is repeating the data for pending and locked even when they are not changed.
- [x] dex_orders - is repeating the data for pending and locked even when they are not changed.
- [x] dex_prices - is repeating the data for pending and locked even when they are not changed.
- [x] dex_withdrawals - is repeating the data for pending and locked even when they are not changed.
- [x] events - remove block_hash
- [x] params - is repeating the data for pending and locked even when they are not changed.
- [x] poll snapshot is not working, but we have mock data - unable to find workflow type: PollSnapshotWorkflow. Supported types: [HeadScanWorkflow, GapScanWorkflow, SchedulerWorkflow, ReindexSchedulerWorkflow, CleanupStagingWorkflow]
- [x] proposal snapshot is not working, but we have mock data - unable to find workflow type: ProposalSnapshotWorkflow. Supported types: [HeadScanWorkflow, GapScanWorkflow, SchedulerWorkflow, ReindexSchedulerWorkflow, CleanupStagingWorkflow]
- pool points by holder:
  - [x] liquidity pool points is - right now that is the amount of token in the pools and should be totalPoints
  - [x] when liquidity pool point became totalPoints that field should be evaluate to understand if the entry change to save the snapshot, basically it will force us to save every holder when the totalPoints change.
- txs:
  - [x] signer should be always there - we have a lot of types that are not handling them. Every single tx has a signer.
  - [x] stake type has counterparty with the publickey which is wrong should have the address.
  - [x] dexLiquidityWithdraw type has `liquidity_amount` with a value, but is wrong, we need a new column called `liquidity_percent` and that should handle the value it handle right now
  - [x] createOrder type has missing the order_id which is first 20 bytes of the tx_hash
  - [x] editOrder and deleteOrder SHOULD have chain_id
  - [x] changeParam has missing the param_value
  - [x] rename counterparty to recipient, anyone that use that one like daoTransfer should keep using, and anyone with counterparty should change to recipient.
  - [x] deleteOrder has missing signer field
  - [x] editOrder has missing signer field
  - [x] certificateResults has the publickey instead of the address in recipient field
- [???] Proposals: Explode proposal column which right now is a string of a json which look like the sample below as individual columns:
  ```json
  {
      "msg": {
        "endHeight": 50,
        "parameterKey": "send",
        "parameterSpace": "fee",
        "parameterValue": "15",
        "signer": "56475aa75463474c0285df5dbf2bcab73da65135",
        "startHeight": 5
      },
      "type": "changeParameter"
    }
  ```
- [x] rename validator SigningInfo to NonSigningInfo, since that is what it is.
- [x] non-signing info has a trash property `StartHeight`
- [x] non-signing info has a trash property `MissedBlocksWindow`
- [x] non-signing and double signing we need to iter current vs previous and again previous vs current to write the "reset|reboot" – when, on iterating the previous, we do not found on the current, we need to create a record with all in zeros as a way to track the reset. 
- [X] orders should NOT SEND the chain id, should always use 0 to get all the orders across all the chains on the "source_chain" which is the indexed chainID
