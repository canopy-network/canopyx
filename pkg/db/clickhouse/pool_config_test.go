package clickhouse

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetPoolConfigForComponent(t *testing.T) {
	tests := []struct {
		name      string
		component string
		wantOpen  int
		wantIdle  int
		wantLife  time.Duration
	}{
		{
			name:      "indexer_admin",
			component: "indexer_admin",
			wantOpen:  15,
			wantIdle:  5,
			wantLife:  5 * time.Minute,
		},
		{
			name:      "indexer_chain",
			component: "indexer_chain",
			wantOpen:  40,
			wantIdle:  15,
			wantLife:  5 * time.Minute,
		},
		{
			name:      "admin",
			component: "admin",
			wantOpen:  10,
			wantIdle:  3,
			wantLife:  5 * time.Minute,
		},
		{
			name:      "admin_chain",
			component: "admin_chain",
			wantOpen:  5,
			wantIdle:  2,
			wantLife:  5 * time.Minute,
		},
		{
			name:      "controller",
			component: "controller",
			wantOpen:  10,
			wantIdle:  3,
			wantLife:  5 * time.Minute,
		},
		{
			name:      "crosschain",
			component: "crosschain",
			wantOpen:  10,
			wantIdle:  3,
			wantLife:  5 * time.Minute,
		},
		{
			name:      "unknown_component_uses_defaults",
			component: "unknown",
			wantOpen:  75,
			wantIdle:  75,
			wantLife:  5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := GetPoolConfigForComponent(tt.component)
			assert.Equal(t, tt.wantOpen, config.MaxOpenConns, "MaxOpenConns mismatch")
			assert.Equal(t, tt.wantIdle, config.MaxIdleConns, "MaxIdleConns mismatch")
			assert.Equal(t, tt.wantLife, config.ConnMaxLifetime, "ConnMaxLifetime mismatch")
			assert.Equal(t, tt.component, config.Component, "Component name mismatch")
		})
	}
}

func TestGetPoolConfigForComponent_DeterministicValues(t *testing.T) {
	// Verify that known components return fixed values regardless of env vars
	os.Setenv("CLICKHOUSE_INDEXER_ADMIN_MAX_OPEN", "999")
	os.Setenv("CLICKHOUSE_INDEXER_ADMIN_MAX_IDLE", "999")
	os.Setenv("CLICKHOUSE_CONN_MAX_LIFETIME", "99h")
	defer func() {
		os.Unsetenv("CLICKHOUSE_INDEXER_ADMIN_MAX_OPEN")
		os.Unsetenv("CLICKHOUSE_INDEXER_ADMIN_MAX_IDLE")
		os.Unsetenv("CLICKHOUSE_CONN_MAX_LIFETIME")
	}()

	config := GetPoolConfigForComponent("indexer_admin")
	assert.Equal(t, 15, config.MaxOpenConns, "Should ignore env and use fixed value")
	assert.Equal(t, 5, config.MaxIdleConns, "Should ignore env and use fixed value")
	assert.Equal(t, 5*time.Minute, config.ConnMaxLifetime, "Should ignore env and use fixed value")
}

func TestGetPoolConfigForComponent_EnforcesMaxIdleLEMaxOpen(t *testing.T) {
	// Test that unknown components with env overrides still enforce MaxIdle <= MaxOpen
	os.Setenv("CLICKHOUSE_MAX_OPEN_CONNS", "5")
	os.Setenv("CLICKHOUSE_MAX_IDLE_CONNS", "10")
	defer func() {
		os.Unsetenv("CLICKHOUSE_MAX_OPEN_CONNS")
		os.Unsetenv("CLICKHOUSE_MAX_IDLE_CONNS")
	}()

	config := GetPoolConfigForComponent("unknown_component")
	assert.Equal(t, 5, config.MaxOpenConns, "MaxOpenConns should be 5")
	assert.Equal(t, 5, config.MaxIdleConns, "MaxIdleConns should be capped at MaxOpenConns")
}

func TestGetPoolConfigForComponent_KnownComponentsIgnoreEnvLifetime(t *testing.T) {
	os.Setenv("CLICKHOUSE_CONN_MAX_LIFETIME", "invalid")
	defer os.Unsetenv("CLICKHOUSE_CONN_MAX_LIFETIME")

	config := GetPoolConfigForComponent("admin")
	assert.Equal(t, 5*time.Minute, config.ConnMaxLifetime, "Known components always use fixed 5 minute lifetime")
}

func TestParseConnMaxLifetimeFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     time.Duration
	}{
		{
			name:     "no_env_set",
			envValue: "",
			want:     0,
		},
		{
			name:     "valid_duration_minutes",
			envValue: "10m",
			want:     10 * time.Minute,
		},
		{
			name:     "valid_duration_seconds",
			envValue: "30s",
			want:     30 * time.Second,
		},
		{
			name:     "valid_duration_hours",
			envValue: "2h",
			want:     2 * time.Hour,
		},
		{
			name:     "invalid_duration_returns_zero",
			envValue: "invalid",
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("CLICKHOUSE_CONN_MAX_LIFETIME", tt.envValue)
				defer os.Unsetenv("CLICKHOUSE_CONN_MAX_LIFETIME")
			}

			got := parseConnMaxLifetimeFromEnv()
			assert.Equal(t, tt.want, got, "parseConnMaxLifetimeFromEnv mismatch")
		})
	}
}

func TestPoolConfig_ComponentNaming(t *testing.T) {
	// Verify that component names are correctly set
	tests := []string{
		"indexer_admin",
		"indexer_chain",
		"admin",
		"admin_chain",
		"controller",
		"crosschain",
	}

	for _, component := range tests {
		t.Run(component, func(t *testing.T) {
			config := GetPoolConfigForComponent(component)
			assert.Equal(t, component, config.Component, "Component name should match input")
		})
	}
}

func TestPoolConfig_ConnectionLimits(t *testing.T) {
	// Test that all component configs have reasonable limits
	components := []string{
		"indexer_admin",
		"indexer_chain",
		"admin",
		"admin_chain",
		"controller",
		"crosschain",
	}

	for _, component := range components {
		t.Run(component, func(t *testing.T) {
			config := GetPoolConfigForComponent(component)

			// MaxOpenConns should be positive
			assert.Greater(t, config.MaxOpenConns, 0, "MaxOpenConns must be positive")

			// MaxIdleConns should be positive
			assert.Greater(t, config.MaxIdleConns, 0, "MaxIdleConns must be positive")

			// MaxIdleConns should be <= MaxOpenConns
			assert.LessOrEqual(t, config.MaxIdleConns, config.MaxOpenConns,
				"MaxIdleConns must be <= MaxOpenConns")

			// ConnMaxLifetime should be positive
			assert.Greater(t, config.ConnMaxLifetime, time.Duration(0),
				"ConnMaxLifetime must be positive")
		})
	}
}

// Benchmark pool config creation
func BenchmarkGetPoolConfigForComponent(b *testing.B) {
	components := []string{
		"indexer_admin",
		"indexer_chain",
		"admin",
		"admin_chain",
		"controller",
		"crosschain",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		component := components[i%len(components)]
		_ = GetPoolConfigForComponent(component)
	}
}
