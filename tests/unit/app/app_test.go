package controller_test

import (
	"testing"
	"time"

	"github.com/canopy-network/canopyx/app/controller"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestDesiredReplicasRespectsThresholds(t *testing.T) {
	app := &controller.App{Logger: zaptest.NewLogger(t)}

	t.Run("returns min when below low watermark", func(t *testing.T) {
		ch := &controller.Chain{ID: "chain-low", MinReplicas: 1, MaxReplicas: 5}
		stats := controller.QueueStats{PendingWorkflowTasks: 3, PendingActivityTasks: 1}
		require.Equal(t, int32(1), app.DesiredReplicas(ch, stats))
		require.Equal(t, int32(1), ch.Hysteresis.LastDecisionReplicas)
	})

	t.Run("returns max when above high watermark", func(t *testing.T) {
		ch := &controller.Chain{ID: "chain-high", MinReplicas: 2, MaxReplicas: 6}
		stats := controller.QueueStats{PendingWorkflowTasks: controller.BacklogHighWatermark + 5, PendingActivityTasks: 10}
		require.Equal(t, int32(6), app.DesiredReplicas(ch, stats))
		require.Equal(t, int32(6), ch.Hysteresis.LastDecisionReplicas)
	})

	t.Run("interpolates between bounds", func(t *testing.T) {
		ch := &controller.Chain{ID: "chain-mixed", MinReplicas: 1, MaxReplicas: 5}
		stats := controller.QueueStats{PendingWorkflowTasks: 320, PendingActivityTasks: 90}
		require.Equal(t, int32(3), app.DesiredReplicas(ch, stats))
		require.Equal(t, int32(3), ch.Hysteresis.LastDecisionReplicas)
	})
}

func TestDesiredReplicasHonoursCooldown(t *testing.T) {
	app := &controller.App{Logger: zaptest.NewLogger(t)}

	ch := &controller.Chain{
		ID:          "chain-cooldown",
		MinReplicas: 1,
		MaxReplicas: 6,
		Hysteresis: controller.ScaleState{
			LastDecisionReplicas: 3,
			LastChangeTime:       time.Now().Add(-controller.ScaleCooldown / 2),
		},
	}
	stats := controller.QueueStats{PendingWorkflowTasks: controller.BacklogHighWatermark + 50}

	require.Equal(t, int32(3), app.DesiredReplicas(ch, stats), "should hold steady during cooldown")
	require.Equal(t, int32(3), ch.Hysteresis.LastDecisionReplicas)

	ch.Hysteresis.LastChangeTime = time.Now().Add(-2 * controller.ScaleCooldown)
	require.Equal(t, int32(6), app.DesiredReplicas(ch, stats), "should update after cooldown elapses")
	require.Equal(t, int32(6), ch.Hysteresis.LastDecisionReplicas)
}
