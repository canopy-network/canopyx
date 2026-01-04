package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/client"
)

// NamespaceType distinguishes between admin and chain namespaces
type NamespaceType string

const (
	NamespaceAdmin NamespaceType = "admin"
	NamespaceChain NamespaceType = "chain"
)

// Default namespace names
const (
	DefaultAdminNamespace  = "canopyx"
	DefaultChainNamePrefix = "chain_"
)

// Queue names (simplified - namespace provides chain context)
const (
	QueueLive        = "live"
	QueueHistorical  = "historical"
	QueueOps         = "ops"
	QueueReindex     = "reindex"
	QueueMaintenance = "maintenance"
)

// Schedule IDs (simplified - namespace provides chain context)
const (
	ScheduleHeadScan          = "headscan"
	ScheduleGapScan           = "gapscan"
	SchedulePollSnapshot      = "pollsnapshot"
	ScheduleProposalSnapshot  = "proposalsnapshot"
	ScheduleLPSnapshot        = "lpsnapshot"
	ScheduleCleanupStaging    = "cleanupstaging"
	ScheduleCrossChainCompact = "crosschain:compaction"
)

// Workflow ID patterns
const (
	WorkflowIDIndexBlock     = "index:%d"
	WorkflowIDScheduler      = "scheduler"
	WorkflowIDGapScheduler   = "gap-scheduler:%d-%d"
	WorkflowIDCleanupStaging = "cleanup:%d"
	WorkflowIDReindexBlock   = "reindex:%d:%s"
	WorkflowIDDeleteChain    = "delete-chain:%d"
)

// NamespaceConfig holds configuration for namespace naming conventions
type NamespaceConfig struct {
	AdminNamespace       string // "canopyx"
	ChainNamespacePrefix string // "chain_"
}

// DefaultNamespaceConfig returns production defaults
func DefaultNamespaceConfig() NamespaceConfig {
	return NamespaceConfig{
		AdminNamespace:       DefaultAdminNamespace,
		ChainNamespacePrefix: DefaultChainNamePrefix,
	}
}

// ChainNamespace returns the namespace name for a given chain ID
// e.g., chainID=1 -> "chain_1"
func (c NamespaceConfig) ChainNamespace(chainID uint64) string {
	return fmt.Sprintf("%s%d", c.ChainNamespacePrefix, chainID)
}

// Schedule spec helper functions (shared across client types)

// TwoSecondSpec returns a schedule spec for HeadScan workflow (5 seconds).
// With 20s block time = 4 checks per block, max 5s delay for new blocks.
func TwoSecondSpec() client.ScheduleSpec {
	return GetScheduleSpec(5 * time.Second)
}

// FiveMinuteSpec returns a schedule spec for five minutes.
// Captures governance snapshots (polls and proposals) at 5-minute intervals.
func FiveMinuteSpec() client.ScheduleSpec {
	return GetScheduleSpec(5 * time.Minute)
}

// OneHourSpec returns a schedule spec for one hour.
func OneHourSpec() client.ScheduleSpec {
	return GetScheduleSpec(time.Hour)
}

// GetScheduleSpec returns a schedule spec for the given interval.
func GetScheduleSpec(interval time.Duration) client.ScheduleSpec {
	return client.ScheduleSpec{Intervals: []client.ScheduleIntervalSpec{{Every: interval}}}
}
