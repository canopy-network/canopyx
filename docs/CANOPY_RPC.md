Mention by @Andrew that these are probably the most important RPCs to index

Docs: https://github.com/canopy-network/canopy/blob/main/cmd/rpc/README.md

```go
const blocksPath = "/v1/query/blocks";
const blockByHashPath = "/v1/query/block-by-hash";
const blockByHeightPath = "/v1/query/block-by-height";
const txByHashPath = "/v1/query/tx-by-hash";
const txsBySender = "/v1/query/txs-by-sender";
const txsByRec = "/v1/query/txs-by-rec";
const txsByHeightPath = "/v1/query/txs-by-height";
const pendingPath = "/v1/query/pending";
const ecoParamsPath = "/v1/query/eco-params";
const validatorsPath = "/v1/query/validators";
const accountsPath = "/v1/query/accounts";
const poolPath = "/v1/query/pool";
const accountPath = "/v1/query/account";
const validatorPath = "/v1/query/validator";
const paramsPath = "/v1/query/params";
const supplyPath = "/v1/query/supply";
const ordersPath = "/v1/query/orders";
const dexPath = "/v1/query/dex"; // TODO ADD
const configPath = "/v1/admin/config";
```


