// Spawn Agent Tool - True RLM Implementation.
//
// Enables recursive sub-agent spawning per Zhang's RLM architecture:
// - Any agent can spawn sub-agents via this tool
// - Sub-agents can spawn their own sub-agents (recursive to any depth)
// - Child agents execute, return answer, then die
// - Parent never sees raw content, only answers
//
// This implements "symbolic recursion" - the spawn call can be embedded
// in code (for-loop, parallel map) within a REPL.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/richinex/ariadne/llm"
)

// SpawnMetrics tracks RLM execution statistics.
type SpawnMetrics struct {
	LLMCalls      atomic.Int64 // Total LLM API calls
	ToolCalls     atomic.Int64 // Total tool executions
	SubAgents     atomic.Int64 // Total sub-agents spawned
	MaxDepthUsed  atomic.Int64 // Deepest recursion level reached
	TotalDuration atomic.Int64 // Total execution time (nanoseconds)
}

// Add adds another metrics instance to this one.
func (m *SpawnMetrics) Add(other *SpawnMetrics) {
	m.LLMCalls.Add(other.LLMCalls.Load())
	m.ToolCalls.Add(other.ToolCalls.Load())
	m.SubAgents.Add(other.SubAgents.Load())
	// Update max depth if other is deeper
	for {
		current := m.MaxDepthUsed.Load()
		otherDepth := other.MaxDepthUsed.Load()
		if otherDepth <= current {
			break
		}
		if m.MaxDepthUsed.CompareAndSwap(current, otherDepth) {
			break
		}
	}
}

// String returns a human-readable summary.
func (m *SpawnMetrics) String() string {
	duration := time.Duration(m.TotalDuration.Load())
	return fmt.Sprintf(
		"LLM calls: %d | Tool calls: %d | Sub-agents: %d | Max depth: %d | Duration: %s",
		m.LLMCalls.Load(),
		m.ToolCalls.Load(),
		m.SubAgents.Load(),
		m.MaxDepthUsed.Load(),
		duration.Round(time.Millisecond),
	)
}

// Global metrics for the current RLM session
var (
	globalMetrics   = &SpawnMetrics{}
	globalMetricsMu sync.Mutex
)

// ResetMetrics resets global metrics for a new RLM session.
func ResetMetrics() *SpawnMetrics {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()
	globalMetrics = &SpawnMetrics{}
	return globalMetrics
}

// GetMetrics returns the current global metrics.
func GetMetrics() *SpawnMetrics {
	return globalMetrics
}

// SpawnConfig holds configuration for spawned agents.
type SpawnConfig struct {
	MaxDepth     int           // Maximum recursion depth (default: 5)
	MaxIterations int           // Max iterations per agent (default: 10)
	Timeout      time.Duration // Timeout per agent (default: 2 min)
}

// DefaultSpawnConfig returns default spawn configuration.
func DefaultSpawnConfig() SpawnConfig {
	return SpawnConfig{
		MaxDepth:      5,
		MaxIterations: 10,
		Timeout:       2 * time.Minute,
	}
}

// SpawnAgentTool enables recursive sub-agent spawning.
// Implements the RLM pattern where any agent can spawn sub-agents.
type SpawnAgentTool struct {
	provider   llm.Provider
	config     SpawnConfig
	toolConfig ToolConfig
	depth      int  // Current recursion depth
	verbose    bool // Print debug output
	metrics    *SpawnMetrics // Metrics tracking

	// Tools available to spawned agents (includes this tool for recursion)
	availableTools []Tool
}

// NewSpawnAgentTool creates a spawn tool at depth 0 (root).
func NewSpawnAgentTool(provider llm.Provider, config SpawnConfig, toolConfig ToolConfig) *SpawnAgentTool {
	return &SpawnAgentTool{
		provider:   provider,
		config:     config,
		toolConfig: toolConfig,
		depth:      0,
		metrics:    globalMetrics,
	}
}

// WithTools sets the tools available to spawned agents.
func (t *SpawnAgentTool) WithTools(tools []Tool) *SpawnAgentTool {
	t.availableTools = tools
	return t
}

// Verbose enables debug output for sub-agents.
func (t *SpawnAgentTool) Verbose(v bool) *SpawnAgentTool {
	t.verbose = v
	return t
}

// atDepth creates a child spawn tool at deeper recursion level.
func (t *SpawnAgentTool) atDepth(depth int) *SpawnAgentTool {
	child := &SpawnAgentTool{
		provider:       t.provider,
		config:         t.config,
		toolConfig:     t.toolConfig,
		depth:          depth,
		verbose:        t.verbose,
		metrics:        t.metrics,
		availableTools: t.availableTools,
	}
	return child
}

func (t *SpawnAgentTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name: "spawn",
		Description: `Spawn a sub-agent to handle a specific task. The sub-agent executes independently,
returns only the answer (not raw content), and terminates. Use this for:
- Reading and summarizing specific documents/sections
- Performing searches across files
- Any subtask that would otherwise bloat your context

The sub-agent has the same capabilities as you, including spawning its own sub-agents.
Use parallel spawning (multiple spawn calls) for independent tasks.`,
		Parameters: []ToolParameter{
			{Name: "task", ParamType: "string", Description: "The specific task for the sub-agent to complete", Required: true},
			{Name: "context", ParamType: "string", Description: "Any context the sub-agent needs (file paths, data, etc.)", Required: false},
		},
	}
}

type spawnArgs struct {
	Task    string `json:"task"`
	Context string `json:"context"`
}

func (t *SpawnAgentTool) Validate(args json.RawMessage) error {
	var a spawnArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Task == "" {
		return fmt.Errorf("task cannot be empty")
	}
	return nil
}

func (t *SpawnAgentTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if err := t.Validate(args); err != nil {
		return FailureResult(err), nil
	}

	var a spawnArgs
	_ = json.Unmarshal(args, &a) // validated above

	// Check recursion depth
	if t.depth >= t.config.MaxDepth {
		return FailureResultf("maximum recursion depth (%d) reached", t.config.MaxDepth), nil
	}

	// Create timeout context for this agent
	ctx, cancel := context.WithTimeout(ctx, t.config.Timeout)
	defer cancel()

	// Build the sub-agent prompt
	prompt := a.Task
	if a.Context != "" {
		prompt = fmt.Sprintf("%s\n\nContext:\n%s", a.Task, a.Context)
	}

	// Run the sub-agent
	result, err := t.runSubAgent(ctx, prompt)
	if err != nil {
		return FailureResult(fmt.Errorf("sub-agent failed: %w", err)), nil
	}

	return SuccessResult(result), nil
}

// runSubAgent creates and executes a sub-agent.
func (t *SpawnAgentTool) runSubAgent(ctx context.Context, task string) (string, error) {
	// Track metrics
	if t.metrics != nil {
		t.metrics.SubAgents.Add(1)
		// Update max depth if this is deeper
		for {
			current := t.metrics.MaxDepthUsed.Load()
			newDepth := int64(t.depth + 1)
			if newDepth <= current {
				break
			}
			if t.metrics.MaxDepthUsed.CompareAndSwap(current, newDepth) {
				break
			}
		}
	}

	// Build tool set for sub-agent (includes spawn tool at deeper depth)
	childSpawn := t.atDepth(t.depth + 1)
	tools := append([]Tool{childSpawn}, t.availableTools...)

	// Build tool map for lookup
	toolMap := make(map[string]Tool)
	for _, tool := range tools {
		toolMap[tool.Metadata().Name] = tool
	}

	// Build conversation
	systemPrompt := fmt.Sprintf(`You are a focused sub-agent (depth %d/%d). Your job is to complete ONE specific task.

IMPORTANT: You MUST use tools to complete tasks. Do not just respond with text.

Available tools: %s

AUTOMATIC STORAGE:
- ripgrep auto-stores files with matches (use DSA tools after)
- read_file also stores files for DSA search
- Use search_stored/get_lines/list_stored to query stored content

Instructions:
1. Use the appropriate tool to complete your task
2. After getting results, summarize the answer concisely
3. Return a clear, direct answer (not raw data)

Example: If asked "what does file.go do?", use read_file to read it, then explain its purpose.`, t.depth+1, t.config.MaxDepth, toolNames(tools))

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task},
	}

	// Create executor for tool calls
	executor := NewExecutor(t.toolConfig)

	// Run ReAct loop
	for i := 0; i < t.config.MaxIterations; i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		if t.verbose {
			fmt.Printf("  [sub:%d:%d] Processing...\n", t.depth+1, i)
		}

		// Call LLM
		response, err := t.provider.ChatWithTools(ctx, messages, convertToLLMTools(tools))
		if t.metrics != nil {
			t.metrics.LLMCalls.Add(1)
		}
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		if t.verbose && response.Content != "" {
			content := response.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			fmt.Printf("  [sub:%d:%d] %s\n", t.depth+1, i, content)
		}

		// Check if there are tool calls
		if len(response.ToolCalls) == 0 {
			// No tool calls - this is the final answer
			if response.Content == "" {
				if t.verbose {
					fmt.Printf("  [sub:%d:%d] Empty response, no tool calls\n", t.depth+1, i)
				}
				return "(sub-agent returned empty response)", nil
			}
			return response.Content, nil
		}

		if t.verbose {
			for _, tc := range response.ToolCalls {
				args := string(tc.Arguments)
				if len(args) > 100 {
					args = args[:100] + "..."
				}
				fmt.Printf("  [sub:%d:%d] Calling: %s(%s)\n", t.depth+1, i, tc.Name, args)
			}
		}

		// Add assistant message with tool calls
		messages = append(messages, llm.ChatMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})

		// Execute tool calls
		for _, tc := range response.ToolCalls {
			tool, exists := toolMap[tc.Name]
			if !exists {
				messages = append(messages, llm.ChatMessage{
					Role:       "tool",
					Content:    fmt.Sprintf("Error: tool '%s' not found", tc.Name),
					ToolCallID: tc.ID,
				})
				continue
			}

			result, err := executor.Execute(ctx, tool, tc.Arguments)
			if t.metrics != nil {
				t.metrics.ToolCalls.Add(1)
			}
			if err != nil {
				messages = append(messages, llm.ChatMessage{
					Role:       "tool",
					Content:    fmt.Sprintf("Error: %v", err),
					ToolCallID: tc.ID,
				})
				continue
			}

			output := result.Output
			if output == "" {
				output = "(empty result)"
			}
			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    output,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("sub-agent reached max iterations without completing")
}

// toolNames returns comma-separated list of tool names.
func toolNames(tools []Tool) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Metadata().Name
	}
	return strings.Join(names, ", ")
}

// convertToLLMTools converts Tool slice to LLM tool definitions.
func convertToLLMTools(tools []Tool) []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, len(tools))
	for i, t := range tools {
		meta := t.Metadata()
		params := make(map[string]interface{})
		required := []string{}
		for _, p := range meta.Parameters {
			params[p.Name] = map[string]interface{}{
				"type":        p.ParamType,
				"description": p.Description,
			}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		defs[i] = llm.ToolDefinition{
			Name:        meta.Name,
			Description: meta.Description,
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": params,
				"required":   required,
			},
		}
	}
	return defs
}

// ParallelSpawnTool enables parallel sub-agent spawning.
// This is a convenience wrapper for spawning multiple sub-agents concurrently.
type ParallelSpawnTool struct {
	spawnTool *SpawnAgentTool
}

// NewParallelSpawnTool creates a parallel spawn tool.
func NewParallelSpawnTool(spawnTool *SpawnAgentTool) *ParallelSpawnTool {
	return &ParallelSpawnTool{
		spawnTool: spawnTool,
	}
}

func (t *ParallelSpawnTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name: "parallel_spawn",
		Description: `Spawn multiple sub-agents in parallel. Each sub-agent executes independently and
returns its answer. Use this when you have multiple independent tasks that can run concurrently.
Returns a JSON object mapping task indices to their results.`,
		Parameters: []ToolParameter{
			{Name: "tasks", ParamType: "array", Description: "Array of task objects, each with 'task' and optional 'context' fields", Required: true},
		},
	}
}

type parallelSpawnArgs struct {
	Tasks []spawnArgs `json:"tasks"`
}

func (t *ParallelSpawnTool) Validate(args json.RawMessage) error {
	var a parallelSpawnArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if len(a.Tasks) == 0 {
		return fmt.Errorf("tasks array cannot be empty")
	}
	for i, task := range a.Tasks {
		if task.Task == "" {
			return fmt.Errorf("task at index %d is empty", i)
		}
	}
	return nil
}

func (t *ParallelSpawnTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if err := t.Validate(args); err != nil {
		return FailureResult(err), nil
	}

	var a parallelSpawnArgs
	_ = json.Unmarshal(args, &a) // validated above

	type taskResult struct {
		index  int
		result string
		err    error
	}

	// Buffered channel prevents goroutine leaks
	results := make(chan taskResult, len(a.Tasks))

	// Launch all tasks
	for i, task := range a.Tasks {
		go func(idx int, taskArg spawnArgs) {
			// Check context before starting
			if ctx.Err() != nil {
				results <- taskResult{index: idx, err: ctx.Err()}
				return
			}

			taskArgs, err := json.Marshal(taskArg)
			if err != nil {
				results <- taskResult{index: idx, err: fmt.Errorf("failed to marshal task: %w", err)}
				return
			}

			res, err := t.spawnTool.Execute(ctx, taskArgs)
			if err != nil {
				results <- taskResult{index: idx, err: err}
			} else if !res.Success() {
				results <- taskResult{index: idx, err: fmt.Errorf("%s", res.Output)}
			} else {
				results <- taskResult{index: idx, result: res.Output}
			}
		}(i, task)
	}

	// Collect exactly len(a.Tasks) results
	type TaskOutput struct {
		Result string `json:"result,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	output := make(map[string]TaskOutput)

	for i := 0; i < len(a.Tasks); i++ {
		select {
		case <-ctx.Done():
			// Drain remaining results to prevent goroutine leaks
			for j := i; j < len(a.Tasks); j++ {
				<-results
			}
			return FailureResult(ctx.Err()), nil
		case r := <-results:
			key := fmt.Sprintf("task_%d", r.index)
			if r.err != nil {
				output[key] = TaskOutput{Error: r.err.Error()}
			} else {
				output[key] = TaskOutput{Result: r.result}
			}
		}
	}

	resultJSON, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return FailureResult(err), nil
	}

	return SuccessResult(string(resultJSON)), nil
}
