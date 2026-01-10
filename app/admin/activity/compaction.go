package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/pkg/db/global"
	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"
)

// CompactGlobalTable optimizes a single global table using OPTIMIZE TABLE FINAL.
// This forces a complete merge of all parts and removes deduplicated records,
// keeping only the latest state per (chain_id, entity_id) combination.
func (ac *Context) CompactGlobalTable(ctx context.Context, input types.ActivityCompactGlobalTableInput) (types.ActivityCompactGlobalTableOutput, error) {
	startTime := time.Now()

	ac.Logger.Info("Starting table compaction",
		zap.String("table", input.TableName))

	// Execute OPTIMIZE TABLE FINAL using direct SQL
	query := fmt.Sprintf(`OPTIMIZE TABLE "%s"."%s" FINAL`, ac.GlobalDB.DatabaseName(), input.TableName)
	err := ac.GlobalDB.Exec(ctx, query)
	duration := time.Since(startTime)

	if err != nil {
		ac.Logger.Error("Table compaction failed",
			zap.String("table", input.TableName),
			zap.Duration("duration", duration),
			zap.Error(err))
		return types.ActivityCompactGlobalTableOutput{
			TableName: input.TableName,
			Success:   false,
			Duration:  duration,
			Error:     err.Error(),
		}, nil // Don't fail the workflow, just mark as failed in output
	}

	ac.Logger.Info("Table compaction completed",
		zap.String("table", input.TableName),
		zap.Duration("duration", duration))

	return types.ActivityCompactGlobalTableOutput{
		TableName: input.TableName,
		Success:   true,
		Duration:  duration,
		Error:     "",
	}, nil
}

// CompactAllGlobalTables compacts all global tables sequentially.
// Returns a summary with per-table results.
func (ac *Context) CompactAllGlobalTables(ctx context.Context) (types.ActivityCompactAllGlobalTablesOutput, error) {
	startTime := time.Now()
	tables := global.GetAllCompactableTables()

	ac.Logger.Info("Starting compaction of all global tables",
		zap.Int("table_count", len(tables)))

	results := make([]types.ActivityCompactGlobalTableOutput, 0, len(tables))
	successCount := 0
	failureCount := 0

	// Compact each table sequentially
	for i, table := range tables {
		// Report heartbeat before each table compaction (activity can run up to 30 minutes)
		activity.RecordHeartbeat(ctx, fmt.Sprintf("compacting_table_%d_of_%d_%s", i+1, len(tables), table.TableName))

		result, err := ac.CompactGlobalTable(ctx, types.ActivityCompactGlobalTableInput{
			TableName: table.TableName,
		})
		if err != nil {
			// Log error but continue with other tables
			ac.Logger.Error("Error during table compaction",
				zap.String("table", table.TableName),
				zap.Error(err))
			continue
		}
		results = append(results, result)

		if result.Success {
			successCount++
		} else {
			failureCount++
		}

		// Report heartbeat after each table completes
		activity.RecordHeartbeat(ctx, fmt.Sprintf("completed_table_%d_of_%d_%s", i+1, len(tables), table.TableName))
	}

	totalDuration := time.Since(startTime)

	ac.Logger.Info("Completed compaction of all global tables",
		zap.Int("total_tables", len(tables)),
		zap.Int("success_count", successCount),
		zap.Int("failure_count", failureCount),
		zap.Duration("total_duration", totalDuration))

	return types.ActivityCompactAllGlobalTablesOutput{
		TotalTables:   len(tables),
		SuccessCount:  successCount,
		FailureCount:  failureCount,
		TotalDuration: totalDuration,
		TableResults:  results,
	}, nil
}

// LogCompactionSummary logs a detailed summary of the compaction results.
// This is a local activity (fast, no retries) for logging purposes.
func (ac *Context) LogCompactionSummary(_ context.Context, output types.ActivityCompactAllGlobalTablesOutput) error {
	ac.Logger.Info("=== Global Table Compaction Summary ===",
		zap.Int("total_tables", output.TotalTables),
		zap.Int("success_count", output.SuccessCount),
		zap.Int("failure_count", output.FailureCount),
		zap.Duration("total_duration", output.TotalDuration),
		zap.String("average_per_table", fmt.Sprintf("%.2fs", output.TotalDuration.Seconds()/float64(output.TotalTables))))

	// Log each table's result
	for _, result := range output.TableResults {
		if result.Success {
			ac.Logger.Info("✓ Table compacted successfully",
				zap.String("table", result.TableName),
				zap.Duration("duration", result.Duration))
		} else {
			ac.Logger.Error("✗ Table compaction failed",
				zap.String("table", result.TableName),
				zap.Duration("duration", result.Duration),
				zap.String("error", result.Error))
		}
	}

	return nil
}