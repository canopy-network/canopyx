We need queue blocks to be indexed base on priority.
Blocks has 20 seconds delay each.
We can assume the block time to calculate to which priority the block belongs.
Priority should be like:
  1. Ultra High = 5 -> Live blocks (upcoming blocks from the next headscan)
  2. High = 4 -> 24hs blocks
  3. Medium = 3 -> 24hs - 48hs blocks
  4. Low = 2 -> 48hs - 72hs blocks
  5. Ultra Low = 1 -> older than 72hs blocks

Im thinking on few ideas:
1. Trigger a workflow for each priority, that will receive the range and schedule the blocks in the fast way possible.
2. Use a goroutine that monitor headscan instead of a workflow that will use priority channels to publish blocks that will then be published into temporal workflows.

I have doubts about both approaches due to the massive amount of missing blocks, currently mainnet has 700k blocks missing if we start indexing. But the reallity is that we need
to be as fast as possible without BURN temporal QPS/RPS scheduling. Since temporal has gRpc rate limits and I think but not sure, task queue has limits too (confirm this please)

Today testing a solution that spawn goroutines (current implementation) it start crashing due to the amount of goroutines.

SchedulerWorkflow will be a long running workflow so we could/will need to use the keep alive mechanism from temporal otherwise it will kill the workflow.
