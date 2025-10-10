package controller

import "time"

// Chain describes desired chain state from metadata DB.
type Chain struct {
	ID          string
	Image       string
	Paused      bool
	Deleted     bool
	MinReplicas int32
	MaxReplicas int32
	Replicas    int32
	TaskQueue   string
	Queue       QueueStats
	Hysteresis  ScaleState
}

type QueueStats struct {
	PendingWorkflowTasks int64
	PendingActivityTasks int64
	PollerCount          int
}

type ScaleState struct {
	LastDecisionReplicas int32
	LastChangeTime       time.Time
}
