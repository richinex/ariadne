package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// Ignore known background goroutines from dependencies
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
}

func TestParallelSpawnToolValidation(t *testing.T) {
	tool := &ParallelSpawnTool{}

	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "empty tasks array",
			args:    `{"tasks":[]}`,
			wantErr: true,
		},
		{
			name:    "empty task in array",
			args:    `{"tasks":[{"task":""}]}`,
			wantErr: true,
		},
		{
			name:    "valid single task",
			args:    `{"tasks":[{"task":"do something"}]}`,
			wantErr: false,
		},
		{
			name:    "valid multiple tasks",
			args:    `{"tasks":[{"task":"task1"},{"task":"task2","context":"ctx"}]}`,
			wantErr: false,
		},
		{
			name:    "invalid json",
			args:    `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.Validate(json.RawMessage(tt.args))
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSpawnAgentToolValidation(t *testing.T) {
	tool := &SpawnAgentTool{}

	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "empty task",
			args:    `{"task":""}`,
			wantErr: true,
		},
		{
			name:    "valid task",
			args:    `{"task":"analyze this"}`,
			wantErr: false,
		},
		{
			name:    "valid task with context",
			args:    `{"task":"analyze","context":"some context"}`,
			wantErr: false,
		},
		{
			name:    "invalid json",
			args:    `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.Validate(json.RawMessage(tt.args))
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSpawnMetricsAdd(t *testing.T) {
	m1 := &SpawnMetrics{}
	m1.LLMCalls.Store(5)
	m1.ToolCalls.Store(10)
	m1.SubAgents.Store(3)
	m1.MaxDepthUsed.Store(2)

	m2 := &SpawnMetrics{}
	m2.LLMCalls.Store(3)
	m2.ToolCalls.Store(5)
	m2.SubAgents.Store(2)
	m2.MaxDepthUsed.Store(4) // Higher depth

	m1.Add(m2)

	if m1.LLMCalls.Load() != 8 {
		t.Errorf("LLMCalls = %d, want 8", m1.LLMCalls.Load())
	}
	if m1.ToolCalls.Load() != 15 {
		t.Errorf("ToolCalls = %d, want 15", m1.ToolCalls.Load())
	}
	if m1.SubAgents.Load() != 5 {
		t.Errorf("SubAgents = %d, want 5", m1.SubAgents.Load())
	}
	if m1.MaxDepthUsed.Load() != 4 {
		t.Errorf("MaxDepthUsed = %d, want 4", m1.MaxDepthUsed.Load())
	}
}

func TestSpawnMetricsString(t *testing.T) {
	m := &SpawnMetrics{}
	m.LLMCalls.Store(10)
	m.ToolCalls.Store(20)
	m.SubAgents.Store(5)
	m.MaxDepthUsed.Store(3)
	m.TotalDuration.Store(int64(time.Second))

	s := m.String()

	if s == "" {
		t.Error("String() returned empty")
	}
	// Should contain key metrics
	if !contains(s, "LLM calls: 10") {
		t.Errorf("String() missing LLM calls, got: %s", s)
	}
	if !contains(s, "Tool calls: 20") {
		t.Errorf("String() missing Tool calls, got: %s", s)
	}
}

func TestResetMetrics(t *testing.T) {
	// Set some values
	globalMetrics.LLMCalls.Store(100)
	globalMetrics.ToolCalls.Store(200)

	// Reset
	m := ResetMetrics()

	if m.LLMCalls.Load() != 0 {
		t.Errorf("LLMCalls after reset = %d, want 0", m.LLMCalls.Load())
	}
	if m.ToolCalls.Load() != 0 {
		t.Errorf("ToolCalls after reset = %d, want 0", m.ToolCalls.Load())
	}
}

func TestDefaultSpawnConfig(t *testing.T) {
	cfg := DefaultSpawnConfig()

	if cfg.MaxDepth != 5 {
		t.Errorf("MaxDepth = %d, want 5", cfg.MaxDepth)
	}
	if cfg.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", cfg.MaxIterations)
	}
	if cfg.Timeout != 2*time.Minute {
		t.Errorf("Timeout = %v, want 2m", cfg.Timeout)
	}
}

func TestSpawnAgentToolMaxDepthReached(t *testing.T) {
	tool := &SpawnAgentTool{
		depth: 5,
		config: SpawnConfig{
			MaxDepth: 5,
		},
	}

	args := json.RawMessage(`{"task":"test task"}`)
	result, err := tool.Execute(context.Background(), args)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success() {
		t.Error("Expected failure when max depth reached")
	}
	// FailureResultf sets Error, not Output
	if result.Error == nil || !contains(result.Error.Error(), "maximum recursion depth") {
		errMsg := ""
		if result.Error != nil {
			errMsg = result.Error.Error()
		}
		t.Errorf("Expected depth error message, got: %s", errMsg)
	}
}

func TestParallelSpawnCancellation(t *testing.T) {
	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	tool := &ParallelSpawnTool{
		spawnTool: &SpawnAgentTool{
			config: SpawnConfig{
				MaxDepth:      5,
				MaxIterations: 10,
				Timeout:       time.Second,
			},
		},
	}

	args := json.RawMessage(`{"tasks":[{"task":"task1"},{"task":"task2"}]}`)
	result, err := tool.Execute(ctx, args)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	// Should fail due to context cancellation
	if result.Success() {
		t.Error("Expected failure on cancelled context")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
