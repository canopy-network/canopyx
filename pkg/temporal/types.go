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
	ScheduleHeadScan         = "headscan"
	ScheduleGapScan          = "gapscan"
	SchedulePollSnapshot     = "pollsnapshot"
	ScheduleProposalSnapshot = "proposalsnapshot"
	ScheduleLPSnapshot       = "lpsnapshot"
	ScheduleGlobalCompaction = "global:compaction"
)

// Workflow ID patterns
const (
	WorkflowIDIndexBlock   = "index:%d"
	WorkflowIDScheduler    = "scheduler"
	WorkflowIDGapScheduler = "gap-scheduler:%d-%d"
	WorkflowIDReindexBlock = "reindex:%d:%s"
	WorkflowIDDeleteChain  = "delete-chain:%d"
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

// ChainNamespace returns the namespace name for a given chain ID (legacy format).
// e.g., chainID=1 -> "chain_1"
// Deprecated: Use ChainNamespaceWithUID for new chains to avoid namespace cache issues.
func (c NamespaceConfig) ChainNamespace(chainID uint64) string {
	return fmt.Sprintf("%s%d", c.ChainNamespacePrefix, chainID)
}

// ChainNamespaceWithUID returns the namespace name for a chain with unique identifier.
// Format: "chain-{chain_id}-{namespace_uid}" (e.g., "chain-5-abc123")
// This ensures a unique namespace name after chain deletion/recreation,
// avoiding Temporal's namespace cache issues.
func (c NamespaceConfig) ChainNamespaceWithUID(chainID uint64, namespaceUID string) string {
	if namespaceUID == "" {
		// Fallback to legacy format for backward compatibility
		return c.ChainNamespace(chainID)
	}
	return fmt.Sprintf("chain-%d-%s", chainID, namespaceUID)
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
