// Supervisor - Multi-Agent Orchestration.
//
// "Agent of agents" that orchestrates multiple specialized agents.
// Uses LLM to decompose complex multi-step tasks.
// Can invoke agents multiple times ("return ticket" pattern).
//
// Information Hiding:
// - Task decomposition logic hidden
// - Sub-goal tracking hidden
// - Agent invocation coordination hidden
// - Memory management hidden

package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/richinex/davingo/agent"
	"github.com/richinex/davingo/model"
	jsonutil "github.com/richinex/davingo/internal/json"
	"github.com/richinex/davingo/llm"
	"github.com/richinex/davingo/storage"
)

// SubGoalDeclaration is a sub-goal declared during task planning.
type subGoalDeclaration struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// supervisorDecision is returned by LLM for next action.
type supervisorDecision struct {
	Thought       string                `json:"thought"`
	SubGoals      []subGoalDeclaration  `json:"sub_goals,omitempty"`
	AgentToInvoke *string               `json:"agent_to_invoke,omitempty"`
	AgentTask     *string               `json:"agent_task,omitempty"`
	SubGoalID     *string               `json:"sub_goal_id,omitempty"`
	IsFinal       bool                  `json:"is_final"`
	FinalAnswer   *string               `json:"final_answer,omitempty"`
}

// subGoalStatus represents the status of a sub-goal.
type subGoalStatus int

const (
	subGoalPending subGoalStatus = iota
	subGoalInProgress
	subGoalCompleted
	subGoalFailed
)

// subGoal is a sub-goal identified by the supervisor.
type subGoal struct {
	ID            string
	Description   string
	Status        subGoalStatus
	AssignedAgent *string
	Result        *string
}

// taskProgress tracks progress across sub-goals.
// Uses a map for O(1) lookup and slice for maintaining insertion order.
type taskProgress struct {
	goalsByID      map[string]*subGoal // O(1) lookup
	order          []string            // Insertion order for display
	completedCount int
	failedCount    int
}

func newTaskProgress() *taskProgress {
	return &taskProgress{
		goalsByID: make(map[string]*subGoal),
		order:     []string{},
	}
}

func (p *taskProgress) addSubGoal(id, description string) {
	if _, exists := p.goalsByID[id]; exists {
		return // Already exists
	}
	goal := &subGoal{
		ID:          id,
		Description: description,
		Status:      subGoalPending,
	}
	p.goalsByID[id] = goal
	p.order = append(p.order, id)
}

func (p *taskProgress) markInProgress(id, agentName string) {
	if goal, exists := p.goalsByID[id]; exists {
		goal.Status = subGoalInProgress
		goal.AssignedAgent = &agentName
	}
}

func (p *taskProgress) markCompleted(id, result string) {
	if goal, exists := p.goalsByID[id]; exists {
		goal.Status = subGoalCompleted
		goal.Result = &result
		p.completedCount++
	}
}

func (p *taskProgress) markFailed(id, errMsg string) {
	if goal, exists := p.goalsByID[id]; exists {
		goal.Status = subGoalFailed
		goal.Result = &errMsg
		p.failedCount++
	}
}

func (p *taskProgress) hasGoal(id string) bool {
	_, exists := p.goalsByID[id]
	return exists
}

func (p *taskProgress) progressSummary() string {
	total := len(p.goalsByID)
	if total == 0 {
		return "Progress: 0/0 sub-goals completed (0%), 0 failed"
	}
	pct := (p.completedCount * 100) / total
	return fmt.Sprintf("Progress: %d/%d sub-goals completed (%d%%), %d failed",
		p.completedCount, total, pct, p.failedCount)
}

func (p *taskProgress) detailedStatus() string {
	total := len(p.goalsByID)
	status := fmt.Sprintf("\nTask Progress (%d/%d):\n", p.completedCount, total)
	for _, id := range p.order {
		g := p.goalsByID[id]
		var icon string
		switch g.Status {
		case subGoalPending:
			icon = "[ ]"
		case subGoalInProgress:
			icon = "[→]"
		case subGoalCompleted:
			icon = "[✓]"
		case subGoalFailed:
			icon = "[✗]"
		}
		status += fmt.Sprintf("  %s %s\n", icon, g.Description)
	}
	return status
}

// SupervisorConfig holds configuration for the supervisor.
type SupervisorConfig struct {
	MaxSubGoals   int
	MaxIterations int
	// LargeResultThreshold is the byte size above which results are stored
	// in ResultStore instead of being passed in conversation. Default: 2KB.
	LargeResultThreshold int
}

// DefaultSupervisorConfig returns default supervisor configuration.
func DefaultSupervisorConfig() SupervisorConfig {
	return SupervisorConfig{
		MaxSubGoals:          10,
		MaxIterations:        10,
		LargeResultThreshold: 1024, // 1KB - results larger than this go to ResultStore
	}
}

// Supervisor orchestrates multiple specialized agents.
// Not safe for concurrent use - use separate instances for concurrent orchestrations.
type Supervisor struct {
	agents             map[string]*agent.Agent
	llmClient          *llm.Client
	config             SupervisorConfig
	handoffCoordinator *Coordinator
	storage            storage.MemoryStorage
	resultStore        *storage.ResultStore
	sessionID          string
	verbose            bool
}

// NewSupervisor creates a new supervisor with the given agents and LLM client.
func NewSupervisor(agents []*agent.Agent, llmClient *llm.Client, config SupervisorConfig) *Supervisor {
	agentMap := make(map[string]*agent.Agent)
	for _, a := range agents {
		agentMap[a.Name()] = a
	}

	return &Supervisor{
		agents:    agentMap,
		llmClient: llmClient,
		config:    config,
		verbose:   false,
	}
}

// WithHandoffValidation enables handoff validation with a configured coordinator.
func (s *Supervisor) WithHandoffValidation(coordinator *Coordinator) *Supervisor {
	s.handoffCoordinator = coordinator
	return s
}

// WithStorage enables memory persistence with storage backend.
func (s *Supervisor) WithStorage(store storage.MemoryStorage, sessionID string) *Supervisor {
	s.storage = store
	s.sessionID = sessionID
	return s
}

// WithResultStore enables large result storage to prevent context bloat.
// When agent results exceed config.LargeResultThreshold, they are stored
// in the ResultStore and only a summary/reference is passed in conversation.
func (s *Supervisor) WithResultStore(store *storage.ResultStore) *Supervisor {
	s.resultStore = store
	return s
}

// Verbose enables verbose output (shows LLM reasoning).
func (s *Supervisor) Verbose(enabled bool) *Supervisor {
	s.verbose = enabled
	return s
}

// Quiet disables verbose output.
func (s *Supervisor) Quiet() *Supervisor {
	s.verbose = false
	return s
}

// Orchestrate orchestrates a complex task across multiple specialized agents.
func (s *Supervisor) Orchestrate(ctx context.Context, task string, maxOrchestrationSteps int) Response {
	// Token tracking local to this orchestration
	tokenStats := &TokenStats{}

	// Store task initiation
	s.storeOrchestrationMemory(ctx, fmt.Sprintf("Started orchestration: %s", task), nil)

	var conversation []llm.ChatMessage
	var allSteps []Step
	agentResultsContext := make(map[string]interface{})
	progress := newTaskProgress()

	// Load prior context if available
	priorContext := s.loadPriorContext(ctx)

	agentDescriptions := make([]string, 0, len(s.agents))
	for _, a := range s.agents {
		agentDescriptions = append(agentDescriptions, fmt.Sprintf("- %s: %s", a.Name(), a.Description()))
	}

	priorContextSection := ""
	if priorContext != "" {
		priorContextSection = "\n\n" + priorContext + "\n"
	}

	systemPrompt := fmt.Sprintf(
		`You are a supervisor that coordinates multiple specialized agents to accomplish complex tasks.

Available Agents:
%s

IMPORTANT LIMITS:
- Maximum orchestration steps: %d
- Maximum sub-goals to declare: %d

Your role is to:
1. IN YOUR FIRST RESPONSE: Analyze the task and declare sub-goals upfront (max %d)
2. IN SUBSEQUENT RESPONSES: Invoke appropriate agents to accomplish each sub-goal
3. Track progress and combine results to provide a final answer

CRITICAL - Passing Data Between Agents:
- When an agent produces data that the next agent needs, you MUST include the complete data in the agent_task field
- The agent_task is the ONLY information the agent receives - make it complete

IMPORTANT - Using Agent Results Efficiently:
- When an agent returns a SUMMARY or ANALYSIS (structured text with headers, bullet points, explanations), that IS the final result - use it directly
- Do NOT re-read stored files when the agent has already provided a summary - the preview/result contains the answer
- Only read stored files when you need to: (1) search for specific details not in the summary, or (2) the agent returned raw/unprocessed data
- If an agent's result contains "File Summary:", "Content Overview:", or similar headers with bullet points, it's already processed - use it as the final answer

You MUST respond in this EXACT JSON format:
{
  "thought": "your reasoning about what to do next",
  "sub_goals": [{"id": "goal_1", "description": "..."}, ...] or null,
  "agent_to_invoke": "agent_name or null",
  "agent_task": "specific task description as a plain text STRING or null",
  "sub_goal_id": "which sub-goal this addresses or null",
  "is_final": false,
  "final_answer": null
}

CRITICAL FORMAT RULES:
- agent_task MUST be a plain string like "read the go.mod file", NOT an object
- All string values must be simple text, never nested JSON objects
- Do not wrap the JSON in markdown code blocks

Respond with valid JSON only. No extra text.%s`,
		strings.Join(agentDescriptions, "\n"),
		maxOrchestrationSteps,
		s.config.MaxSubGoals,
		s.config.MaxSubGoals,
		priorContextSection,
	)

	conversation = append(conversation, llm.ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	conversation = append(conversation, llm.ChatMessage{
		Role:    "user",
		Content: fmt.Sprintf("Task: %s", task),
	})

	for step := 0; step < maxOrchestrationSteps; step++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return NewFailureResponse(
				fmt.Sprintf("orchestration cancelled: %v", ctx.Err()),
				allSteps,
				buildMetadata(tokenStats),
				&CompletionStatus{
					Type:        StatusFailed,
					Error:       ctx.Err().Error(),
					Recoverable: true,
				},
			)
		}

		remainingSteps := maxOrchestrationSteps - step

		decision, err := s.decideNextAction(ctx, conversation, tokenStats)
		if err != nil {
			return NewFailureResponse(
				fmt.Sprintf("Supervisor decision failed: %v", err),
				allSteps,
				buildMetadata(tokenStats),
				&CompletionStatus{
					Type:        StatusFailed,
					Error:       fmt.Sprintf("Supervisor reasoning failed: %v", err),
					Recoverable: true,
				},
			)
		}

		// Handle sub-goal declaration
		if len(decision.SubGoals) > 0 {
			goalsToAdd := decision.SubGoals
			if len(goalsToAdd) > s.config.MaxSubGoals {
				goalsToAdd = goalsToAdd[:s.config.MaxSubGoals]
			}
			for _, decl := range goalsToAdd {
				progress.addSubGoal(decl.ID, decl.Description)
			}
		}

		// Check if task is complete
		if decision.IsFinal {
			finalAnswer := "Task completed without explicit answer"
			if decision.FinalAnswer != nil {
				finalAnswer = *decision.FinalAnswer
			}

			// Store completion in memory
			preview := finalAnswer
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			s.storeOrchestrationMemory(ctx, fmt.Sprintf("Orchestration completed: %s", preview), nil)

			allSteps = append(allSteps, model.Step{
				Iteration:   step,
				Thought:     decision.Thought,
				Observation: &finalAnswer,
			})

			return NewSuccessResponse(
				finalAnswer,
				allSteps,
				buildMetadata(tokenStats),
				&CompletionStatus{Type: StatusComplete},
			)
		}

		// Invoke agent if specified
		if decision.AgentToInvoke != nil && decision.AgentTask != nil {
			agentName := *decision.AgentToInvoke
			agentTask := *decision.AgentTask

			subGoalID := fmt.Sprintf("goal_%d", step)
			if decision.SubGoalID != nil {
				subGoalID = *decision.SubGoalID
			}

			if !progress.hasGoal(subGoalID) {
				progress.addSubGoal(subGoalID, agentTask)
			}

			progress.markInProgress(subGoalID, agentName)

			selectedAgent, exists := s.agents[agentName]
			if !exists {
				errorMsg := fmt.Sprintf("Agent '%s' not found", agentName)
				conversation = append(conversation, llm.ChatMessage{
					Role:    "user",
					Content: fmt.Sprintf("Error: %s", errorMsg),
				})
				allSteps = append(allSteps, model.Step{
					Iteration:   step,
					Thought:     decision.Thought,
					Action:      &agentName,
					Observation: &errorMsg,
				})
				continue
			}

			// Build context from previous agent results
			var contextData json.RawMessage
			if len(agentResultsContext) > 0 {
				contextData, _ = json.Marshal(agentResultsContext)
			}

			// Propagate verbose setting to agent
			selectedAgent.Verbose(s.verbose)

			agentResponse := selectedAgent.ExecuteWithContext(ctx, agentTask, contextData, s.config.MaxIterations)

			var resultSummary string
			switch agentResponse.Type {
			case agent.ResponseSuccess:
				// Aggregate agent token usage and LLM calls
				if agentResponse.Metadata.TokenUsage != nil {
					tokenStats.AddUsage(agentResponse.Metadata.TokenUsage)
				}
				tokenStats.LLMCalls += agentResponse.Metadata.LLMCalls

				// Process result - store in ResultStore if large
				processedResult := s.processAgentResult(ctx, agentName, subGoalID, agentResponse.Result, tokenStats)
				progress.markCompleted(subGoalID, processedResult)

				// Store agent execution result
				preview := processedResult
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				s.storeOrchestrationMemory(ctx, fmt.Sprintf(
					"Agent '%s' completed sub-goal '%s': %s",
					agentName, subGoalID, preview,
				), &agentName)

				// Add to context for future agents
				var resultValue interface{}
				if err := json.Unmarshal([]byte(processedResult), &resultValue); err != nil {
					resultValue = processedResult
				}
				agentResultsContext[agentName+"_output"] = resultValue

				resultSummary = fmt.Sprintf("SUCCESS: %s", processedResult)

			case agent.ResponseFailure:
				// Still count LLM calls from failed agents
				tokenStats.LLMCalls += agentResponse.Metadata.LLMCalls
				if agentResponse.Metadata.TokenUsage != nil {
					tokenStats.AddUsage(agentResponse.Metadata.TokenUsage)
				}
				progress.markFailed(subGoalID, agentResponse.Error)
				resultSummary = fmt.Sprintf("FAILED: %s", agentResponse.Error)

			case agent.ResponseTimeout:
				// Still count LLM calls from timed-out agents
				tokenStats.LLMCalls += agentResponse.Metadata.LLMCalls
				if agentResponse.Metadata.TokenUsage != nil {
					tokenStats.AddUsage(agentResponse.Metadata.TokenUsage)
				}
				progress.markFailed(subGoalID, agentResponse.PartialResult)
				resultSummary = fmt.Sprintf("TIMEOUT: %s", agentResponse.PartialResult)
			}

			// Update conversation
			assistantJSON, err := json.Marshal(supervisorDecision{
				Thought:       decision.Thought,
				AgentToInvoke: &agentName,
				AgentTask:     &agentTask,
				SubGoalID:     &subGoalID,
				IsFinal:       false,
			})
			if err != nil {
				assistantJSON = []byte(fmt.Sprintf(`{"thought": %q}`, decision.Thought))
			}
			conversation = append(conversation, llm.ChatMessage{
				Role:    "assistant",
				Content: string(assistantJSON),
			})

			urgencyMsg := fmt.Sprintf("\n\nYou have %d orchestration steps remaining.", remainingSteps-1)
			if remainingSteps-1 <= 2 {
				urgencyMsg = fmt.Sprintf("\n\nWARNING: Only %d orchestration steps remaining!", remainingSteps-1)
			}

			conversation = append(conversation, llm.ChatMessage{
				Role: "user",
				Content: fmt.Sprintf(
					"Agent '%s' completed the task.\nResult: %s%s\n%s\n\nIf all sub-goals are complete, set is_final=true and provide the final_answer.",
					agentName, resultSummary, urgencyMsg, progress.detailedStatus(),
				),
			})

			action := fmt.Sprintf("%s:%s", agentName, agentTask)
			allSteps = append(allSteps, model.Step{
				Iteration:   step,
				Thought:     decision.Thought,
				Action:      &action,
				Observation: &resultSummary,
			})
		} else {
			// No agent invoked
			warning := "Supervisor must either invoke an agent or mark task as final"
			conversation = append(conversation, llm.ChatMessage{
				Role:    "user",
				Content: fmt.Sprintf("%s\nPlease either invoke an agent or set is_final=true", warning),
			})
			allSteps = append(allSteps, model.Step{
				Iteration:   step,
				Thought:     decision.Thought,
				Observation: &warning,
			})
		}
	}

	// Max orchestration steps reached
	partialResult := fmt.Sprintf(
		"Supervisor reached max orchestration steps. %s",
		progress.progressSummary(),
	)

	return NewTimeoutResponse(
		partialResult,
		allSteps,
		buildMetadata(tokenStats),
		&CompletionStatus{
			Type:      StatusPartial,
			NextSteps: []string{"Increase max_orchestration_steps"},
		},
	)
}

// decideNextAction asks the supervisor LLM to decide the next action.
// Uses streaming when verbose mode is enabled to show tokens in real-time.
func (s *Supervisor) decideNextAction(ctx context.Context, conversation []llm.ChatMessage, tokenStats *TokenStats) (supervisorDecision, error) {
	var response string
	var err error
	var usage *llm.TokenUsage

	if s.verbose {
		response, usage, err = s.decideWithStreaming(ctx, conversation)
	} else {
		response, usage, err = s.llmClient.ChatWithUsage(ctx, conversation)
	}

	if err != nil {
		return supervisorDecision{}, fmt.Errorf("LLM chat failed: %w", err)
	}

	// Track token usage
	tokenStats.LLMCalls++
	tokenStats.AddUsage(usage)

	extracted, err := jsonutil.ExtractJSON(response)
	if err != nil {
		// Could not extract JSON - treat as a thought without action
		return supervisorDecision{
			Thought: response,
			IsFinal: false,
		}, nil
	}

	var decision supervisorDecision
	if err := json.Unmarshal([]byte(extracted), &decision); err != nil {
		return supervisorDecision{
			Thought: response,
			IsFinal: false,
		}, nil
	}

	return decision, nil
}

// streamResult holds the result of a streaming call.
type streamResult struct {
	usage *llm.TokenUsage
	err   error
}

// decideWithStreaming uses streaming to show tokens in real-time (verbose mode).
func (s *Supervisor) decideWithStreaming(ctx context.Context, conversation []llm.ChatMessage) (string, *llm.TokenUsage, error) {
	chunks := make(chan string, 100)

	// Start streaming in goroutine
	resultCh := make(chan streamResult, 1)
	go func() {
		defer close(chunks)
		usage, err := s.llmClient.StreamChat(ctx, conversation, chunks)
		resultCh <- streamResult{usage: usage, err: err}
	}()

	// Collect response while printing tokens
	var response strings.Builder
	printedHeader := false

	for chunk := range chunks {
		if !printedHeader {
			fmt.Printf("\n[supervisor] ")
			printedHeader = true
		}
		fmt.Print(chunk)
		os.Stdout.Sync()
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

// storeOrchestrationMemory stores an orchestration memory entry.
func (s *Supervisor) storeOrchestrationMemory(ctx context.Context, content string, agentName *string) {
	if s.storage == nil || s.sessionID == "" {
		return
	}

	entry := storage.NewMemoryEntry(s.sessionID, storage.MemoryOrchestration, content)

	if agentName != nil {
		entry = entry.WithAgent(*agentName)
	}

	_ = s.storage.StoreMemory(ctx, entry) // Best-effort memory storage
}

// loadPriorContext loads prior orchestration context for this session.
func (s *Supervisor) loadPriorContext(ctx context.Context) string {
	if s.storage == nil || s.sessionID == "" {
		return ""
	}

	memType := storage.MemoryOrchestration
	memories, err := s.storage.QueryMemories(ctx, s.sessionID, &memType, 5)
	if err != nil || len(memories) == 0 {
		return ""
	}

	lines := make([]string, 0, len(memories))
	for _, m := range memories {
		lines = append(lines, fmt.Sprintf("- %s", m.Content))
	}

	return fmt.Sprintf("Prior orchestration context:\n%s", strings.Join(lines, "\n"))
}

// AgentNames returns the names of all registered agents.
func (s *Supervisor) AgentNames() []string {
	names := make([]string, 0, len(s.agents))
	for name := range s.agents {
		names = append(names, name)
	}
	return names
}

// buildMetadata creates metadata with token stats.
func buildMetadata(stats *TokenStats) *Metadata {
	return &Metadata{
		TokenStats: stats,
	}
}

// storedResultRef represents a reference to a large result stored in ResultStore.
type storedResultRef struct {
	Key       string `json:"result_key"`
	FilePath  string `json:"file_path"`
	Hash      string `json:"content_hash"`
	LineCount int    `json:"line_count"`
	ByteSize  int    `json:"byte_size"`
	Summary   string `json:"summary"`
}

// processAgentResult checks if result is large and stores it in ResultStore if so.
// Returns the result to pass to supervisor (either original or compact reference).
func (s *Supervisor) processAgentResult(ctx context.Context, agentName, subGoalID, result string, tokenStats *TokenStats) string {
	// If no ResultStore or result is small, return as-is
	if s.resultStore == nil || len(result) <= s.config.LargeResultThreshold {
		return result
	}

	// Store large result
	key := storage.ResultKey{
		SessionID: s.sessionID,
		Key:       fmt.Sprintf("%s/%s", agentName, subGoalID),
	}

	meta, err := s.resultStore.Store(ctx, key, result, storage.DefaultStoreOptions())
	if err != nil {
		// If storage fails, truncate result instead
		return s.truncateResult(result)
	}

	// File path for direct access (e.g., with ripgrep or cat)
	filePath := fmt.Sprintf(".davingo/results/%s", meta.ContentHash)

	// Build compact reference with file path for tool access
	ref := storedResultRef{
		Key:       key.Key,
		FilePath:  filePath,
		Hash:      meta.ContentHash,
		LineCount: meta.LineCount,
		ByteSize:  meta.ByteSize,
		Summary:   meta.Summary,
	}

	refJSON, err := json.Marshal(ref)
	if err != nil {
		refJSON = []byte(fmt.Sprintf(`{"file_path": %q}`, filePath))
	}

	referenceStr := fmt.Sprintf("[Large result stored - %d bytes, %d lines]\nFile: %s (use ripgrep/cat to access)\nReference: %s\nPreview:\n%s",
		meta.ByteSize, meta.LineCount, filePath, string(refJSON), meta.Summary)

	// Track actual bytes saved (original size - actual reference size)
	tokenStats.BytesSaved += len(result) - len(referenceStr)
	tokenStats.ResultsStored++

	return referenceStr
}

// truncateResult truncates a result to fit within threshold.
func (s *Supervisor) truncateResult(result string) string {
	threshold := s.config.LargeResultThreshold
	if len(result) <= threshold {
		return result
	}

	// Show first and last portions
	halfSize := threshold / 2
	return fmt.Sprintf("%s\n\n... [%d bytes truncated] ...\n\n%s",
		result[:halfSize], len(result)-threshold, result[len(result)-halfSize:])
}
