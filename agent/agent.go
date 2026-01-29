// ReAct (Reason + Act) loop implementation.
//
// This is THE canonical implementation of the ReAct pattern.
// All agent execution goes through this module.
//
// Information Hiding:
// - ReAct loop internals hidden
// - LLM communication hidden
// - Tool execution coordination hidden
// - Memory management hidden

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/richinex/davingo/model"
	jsonutil "github.com/richinex/davingo/internal/json"
	"github.com/richinex/davingo/llm"
	"github.com/richinex/davingo/storage"
	"github.com/richinex/davingo/tools"
)

// Agent executes tasks using the ReAct pattern.
// Following Dave's naming advice: just agent.Agent, not agent.ReactAgent.
type Agent struct {
	config       Config
	llmClient    *llm.Client
	toolRegistry *tools.Registry
	toolExecutor *tools.Executor
	storage      storage.MemoryStorage
	sessionID    string
	verbose      bool
}

// New creates a new agent with the given configuration and provider.
func New(config Config, provider llm.Provider) *Agent {
	registry := tools.NewRegistry()
	for _, tool := range config.Tools {
		_ = registry.Register(tool) // Ignore duplicate errors - caller's responsibility
	}

	return &Agent{
		config:       config,
		llmClient:    llm.NewClient(provider),
		toolRegistry: registry,
		toolExecutor: tools.NewDefaultExecutor(),
		verbose:      false,
	}
}

// WithToolConfig overrides the tool execution configuration.
func (a *Agent) WithToolConfig(config tools.ToolConfig) *Agent {
	a.toolExecutor = tools.NewExecutor(config)
	return a
}

// Name returns the agent's name.
func (a *Agent) Name() string {
	return a.config.Name
}

// Description returns the agent's description.
func (a *Agent) Description() string {
	return a.config.Description
}

// WithStorage enables memory persistence.
func (a *Agent) WithStorage(store storage.MemoryStorage, sessionID string) *Agent {
	a.storage = store
	a.sessionID = sessionID
	return a
}

// Verbose enables verbose output (shows LLM reasoning).
func (a *Agent) Verbose(enabled bool) *Agent {
	a.verbose = enabled
	return a
}

// Quiet disables verbose output.
func (a *Agent) Quiet() *Agent {
	a.verbose = false
	return a
}

// Execute runs a task with the given maximum iterations.
func (a *Agent) Execute(ctx context.Context, task string, maxIterations int) Response {
	return a.ExecuteWithHistory(ctx, task, nil, maxIterations)
}

// ExecuteWithHistory runs a task with conversation history.
func (a *Agent) ExecuteWithHistory(ctx context.Context, task string, history []llm.ChatMessage, maxIterations int) Response {
	return a.executeFull(ctx, task, history, nil, maxIterations)
}

// ExecuteWithContext runs a task with additional context data.
func (a *Agent) ExecuteWithContext(ctx context.Context, task string, contextData json.RawMessage, maxIterations int) Response {
	return a.executeFull(ctx, task, nil, contextData, maxIterations)
}

// executeFull is the main execution method with all options.
func (a *Agent) executeFull(ctx context.Context, task string, history []llm.ChatMessage, contextData json.RawMessage, maxIterations int) Response {
	startTime := time.Now()
	var steps []model.Step
	var toolCalls []model.ToolCall
	var totalUsage llm.TokenUsage // Track cumulative token usage
	var llmCalls int              // Track number of LLM calls
	conversation := history
	var lastToolOutput string

	// Load relevant memories
	memoryContext := a.loadRelevantMemories(ctx, 3)

	// Build memory section
	memorySection := ""
	if memoryContext != "" {
		memorySection = "\n\n" + memoryContext + "\n"
	}

	// Build context section
	contextSection := ""
	if len(contextData) > 0 {
		contextSection = fmt.Sprintf("\n\nCONTEXT DATA:\n```json\n%s\n```", string(contextData))
	}

	// Add system prompt if starting fresh
	if len(conversation) == 0 {
		systemPrompt := fmt.Sprintf(
			`%s

Available Tools:
%s%s%s

You have a maximum of %d iterations.
Respond in this JSON format:
{
  "thought": "your reasoning",
  "action": {"tool": "name", "input": {...}},
  "is_final": false,
  "final_answer": null
}

When complete: is_final=true, action=null, provide final_answer.`,
			a.config.SystemPrompt,
			a.toolRegistry.Description(),
			contextSection,
			memorySection,
			maxIterations,
		)

		conversation = append(conversation, llm.ChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	conversation = append(conversation, llm.ChatMessage{
		Role:    "user",
		Content: fmt.Sprintf("Task: %s", task),
	})

	// ReAct loop
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Check context cancellation at top of loop
		if ctx.Err() != nil {
			return NewFailureResponse(
				fmt.Sprintf("execution cancelled: %v", ctx.Err()),
				steps,
				uint64(time.Since(startTime).Milliseconds()),
			)
		}

		remaining := maxIterations - iteration

		// Think: get next action from LLM
		decision, usage, err := a.think(ctx, conversation)
		if err != nil {
			return NewFailureResponse(
				fmt.Sprintf("Failed to reason: %v", err),
				steps,
				uint64(time.Since(startTime).Milliseconds()),
			)
		}

		// Track LLM call and accumulate token usage
		llmCalls++
		if usage != nil {
			totalUsage.PromptTokens += usage.PromptTokens
			totalUsage.CompletionTokens += usage.CompletionTokens
			totalUsage.TotalTokens += usage.TotalTokens
		}

		// Check if complete
		if decision.IsFinal {
			result := a.getFinalResult(decision, lastToolOutput)

			// Store episodic memory
			a.storeEpisodicMemory(ctx, task, result)

			steps = append(steps, model.Step{
				Iteration:   iteration,
				Thought:     decision.Thought,
				Action:      nil,
				Observation: &result,
			})

			return NewSuccessResponse(
				result,
				steps,
				toolCalls,
				uint64(time.Since(startTime).Milliseconds()),
				a.config.Name,
				&totalUsage,
				llmCalls,
			)
		}

		// Act: execute tool
		if decision.Action != nil {
			observation, toolCall, err := a.executeTool(ctx, decision.Action)

			if toolCall != nil {
				toolCalls = append(toolCalls, *toolCall)
			}

			if err == nil {
				lastToolOutput = observation
			}

			// Add to conversation
			assistantMsg := map[string]interface{}{
				"thought": decision.Thought,
				"action": map[string]interface{}{
					"tool":  decision.Action.Tool,
					"input": decision.Action.Input,
				},
				"is_final": false,
			}
			msgJSON, err := json.Marshal(assistantMsg)
			if err != nil {
				// Fallback if marshal fails (should not happen with simple types)
				msgJSON = []byte(fmt.Sprintf(`{"thought": %q}`, decision.Thought))
			}
			conversation = append(conversation, llm.ChatMessage{
				Role:    "assistant",
				Content: string(msgJSON),
			})

			urgency := ""
			if remaining <= 2 {
				urgency = fmt.Sprintf("\n\nWARNING: Only %d iterations remaining!", remaining-1)
			}

			observationMsg := observation
			if err != nil {
				observationMsg = fmt.Sprintf("Tool failed: %v", err)
			}

			conversation = append(conversation, llm.ChatMessage{
				Role: "user",
				Content: fmt.Sprintf(
					"Observation: %s%s\n\nIs the task complete? If yes, set is_final=true.",
					observationMsg, urgency,
				),
			})

			actionName := decision.Action.Tool
			steps = append(steps, model.Step{
				Iteration:   iteration,
				Thought:     decision.Thought,
				Action:      &actionName,
				Observation: &observationMsg,
			})
		} else {
			// No action - might be implicit completion
			if a.hasPriorProgress(steps) {
				result := a.getImplicitResult(decision, lastToolOutput, steps)

				a.storeEpisodicMemory(ctx, task, result)

				return NewSuccessResponse(
					result,
					steps,
					toolCalls,
					uint64(time.Since(startTime).Milliseconds()),
					a.config.Name,
					&totalUsage,
					llmCalls,
				)
			}

			observation := "No action specified"
			steps = append(steps, model.Step{
				Iteration:   iteration,
				Thought:     decision.Thought,
				Action:      nil,
				Observation: &observation,
			})
		}
	}

	// Max iterations reached
	a.storeEpisodicMemory(ctx, task, fmt.Sprintf("Timeout after %d iterations", maxIterations))

	return NewTimeoutResponse(
		steps,
		toolCalls,
		uint64(time.Since(startTime).Milliseconds()),
		&totalUsage,
		llmCalls,
	)
}

// think asks the LLM for the next action.
// Uses streaming when verbose mode is enabled to show tokens in real-time.
// Returns the decision and token usage (usage may be nil for streaming).
func (a *Agent) think(ctx context.Context, conversation []llm.ChatMessage) (Decision, *llm.TokenUsage, error) {
	var response string
	var err error
	var usage *llm.TokenUsage

	if a.verbose {
		// Use streaming to show tokens in real-time
		response, usage, err = a.thinkWithStreaming(ctx, conversation)
	} else {
		// Use regular completion with token tracking
		response, usage, err = a.llmClient.ChatWithUsage(ctx, conversation)
	}

	if err != nil {
		return Decision{}, nil, fmt.Errorf("LLM chat failed: %w", err)
	}

	// Extract JSON from response
	var decision Decision
	extracted, err := jsonutil.ExtractJSON(response)
	if err != nil {
		// Could not extract JSON - treat as a thought without action
		return Decision{
			Thought: response,
			IsFinal: false,
		}, usage, nil
	}

	if err := json.Unmarshal([]byte(extracted), &decision); err != nil {
		return Decision{
			Thought: response,
			IsFinal: false,
		}, usage, nil
	}

	return decision, usage, nil
}

// streamResult holds the result of a streaming call.
type streamResult struct {
	usage *llm.TokenUsage
	err   error
}

// thinkWithStreaming uses streaming to show tokens in real-time (verbose mode).
func (a *Agent) thinkWithStreaming(ctx context.Context, conversation []llm.ChatMessage) (string, *llm.TokenUsage, error) {
	chunks := make(chan string, 100)

	// Start streaming in goroutine
	resultCh := make(chan streamResult, 1)
	go func() {
		defer close(chunks)
		usage, err := a.llmClient.StreamChat(ctx, conversation, chunks)
		resultCh <- streamResult{usage: usage, err: err}
	}()

	// Collect response while printing tokens
	var response strings.Builder
	printedHeader := false

	for chunk := range chunks {
		if !printedHeader {
			fmt.Printf("\n[%s] ", a.config.Name)
			printedHeader = true
		}
		fmt.Print(chunk)
		os.Stdout.Sync() // Flush to show tokens immediately
		response.WriteString(chunk)
	}

	if printedHeader {
		fmt.Print("\n\n")
	}

	// Wait for stream to complete and check result
	result := <-resultCh
	if result.err != nil {
		return "", nil, result.err
	}

	return response.String(), result.usage, nil
}

// executeTool runs a tool and returns the observation.
func (a *Agent) executeTool(ctx context.Context, action *Action) (string, *model.ToolCall, error) {
	tool, exists := a.toolRegistry.Get(action.Tool)
	if !exists {
		return "", nil, fmt.Errorf("tool '%s' not found", action.Tool)
	}

	startTime := time.Now()
	inputJSON, err := json.Marshal(action.Input)
	if err != nil {
		inputJSON = []byte("{}")
	}
	inputSize := len(inputJSON)

	result, err := a.toolExecutor.Execute(ctx, tool, action.Input)
	if err != nil {
		return "", nil, fmt.Errorf("tool %q failed: %w", action.Tool, err)
	}

	toolCall := &model.ToolCall{
		Name:       action.Tool,
		InputSize:  inputSize,
		OutputSize: len(result.Output),
		DurationMs: uint64(time.Since(startTime).Milliseconds()),
		Success:    result.Success(),
	}

	if result.Success() {
		return result.Output, toolCall, nil
	}

	return "", toolCall, result.Error
}

// Memory helpers

func (a *Agent) storeEpisodicMemory(ctx context.Context, task, result string) {
	if a.storage == nil || a.sessionID == "" {
		return
	}

	resultPreview := result
	if len(result) > 150 {
		resultPreview = result[:150] + "..."
	}

	entry := storage.NewMemoryEntry(a.sessionID, storage.MemoryEpisodic, fmt.Sprintf("Task: %s | Result: %s", task, resultPreview)).
		WithAgent(a.config.Name)

	_ = a.storage.StoreMemory(ctx, entry) // Best-effort memory storage
}

func (a *Agent) loadRelevantMemories(ctx context.Context, limit int) string {
	if a.storage == nil || a.sessionID == "" {
		return ""
	}

	memType := storage.MemoryEpisodic
	memories, err := a.storage.QueryMemories(ctx, a.sessionID, &memType, limit)
	if err != nil || len(memories) == 0 {
		return ""
	}

	var lines []string
	for _, m := range memories {
		lines = append(lines, fmt.Sprintf("- %s", m.Content))
	}

	return fmt.Sprintf("Relevant past experiences:\n%s", strings.Join(lines, "\n"))
}

// Result helpers

func (a *Agent) getFinalResult(decision Decision, lastToolOutput string) string {
	if a.config.ReturnToolOutput && lastToolOutput != "" {
		return lastToolOutput
	}
	if decision.FinalAnswer != nil {
		return *decision.FinalAnswer
	}
	return "Task completed"
}

func (a *Agent) getImplicitResult(decision Decision, lastToolOutput string, steps []model.Step) string {
	if a.config.ReturnToolOutput && lastToolOutput != "" {
		return lastToolOutput
	}
	if decision.Thought != "" {
		return decision.Thought
	}
	if len(steps) > 0 && steps[len(steps)-1].Observation != nil {
		return *steps[len(steps)-1].Observation
	}
	return "Task completed"
}

func (a *Agent) hasPriorProgress(steps []model.Step) bool {
	if len(steps) == 0 {
		return false
	}
	for _, s := range steps {
		if s.Observation != nil {
			return true
		}
	}
	return false
}

