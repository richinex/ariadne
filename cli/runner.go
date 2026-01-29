// Command execution for CLI commands.
//
// Information Hiding:
// - Command dispatch logic hidden
// - Agent/orchestration setup hidden
// - Output formatting hidden

package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/richinex/ariadne/agent"
	"github.com/richinex/ariadne/model"
	"github.com/richinex/ariadne/config"
	"github.com/richinex/ariadne/llm"
	"github.com/richinex/ariadne/mcp"
	"github.com/richinex/ariadne/orchestration"
	"github.com/richinex/ariadne/storage"
	"github.com/richinex/ariadne/tools"
)

// Options holds CLI execution options.
type Options struct {
	Provider    string
	MaxIter     int
	ToolRetries uint32
	Verbose     bool
}

// DefaultOptions returns default CLI options.
func DefaultOptions() Options {
	return Options{
		MaxIter:     10,
		ToolRetries: 3,
		Verbose:     false,
	}
}

// RunTask executes a single task with an agent.
func RunTask(ctx context.Context, task, agentName, systemPrompt string, opts Options) error {
	provider, err := createProvider(opts.Provider)
	if err != nil {
		return err
	}

	// Create ResultStore for RLM pattern
	resultStore, cleanup := createResultStore()
	if cleanup != nil {
		defer cleanup()
	}

	// Pre-store any files mentioned in the task (automatic context)
	fileContext, task := preStoreFilesFromPrompt(ctx, task, resultStore)

	toolConfig := tools.ToolConfig{MaxRetries: opts.ToolRetries}
	a, err := CreateAgent(agentName, systemPrompt, provider, toolConfig, resultStore, fileContext)
	if err != nil {
		return err
	}

	if opts.Verbose {
		a = a.Verbose(true)
	}

	fmt.Printf("Running task with %s agent...\n\n", agentName)

	response := a.Execute(ctx, task, opts.MaxIter)

	switch response.Type {
	case agent.ResponseSuccess:
		if opts.Verbose {
			printAgentSteps(response.Steps)
		}
		fmt.Printf("%s\n\n", response.Result)
		if len(response.Steps) > 0 {
			fmt.Printf("(%d steps)\n", len(response.Steps))
		}
		return nil
	case agent.ResponseFailure:
		fmt.Fprintf(os.Stderr, "Error: %s\n", response.Error)
		return fmt.Errorf("task failed: %s", response.Error)
	case agent.ResponseTimeout:
		fmt.Printf("Timeout. Partial result:\n%s\n", response.PartialResult)
		return fmt.Errorf("task timed out")
	default:
		return fmt.Errorf("unknown response type: %v", response.Type)
	}
}

// Chat starts an interactive chat session.
func Chat(ctx context.Context, agentName, systemPrompt, sessionID, dbPath string, opts Options) error {
	provider, err := createProvider(opts.Provider)
	if err != nil {
		return err
	}

	// Create ResultStore for RLM pattern
	resultStore, cleanup := createResultStore()
	if cleanup != nil {
		defer cleanup()
	}

	// Create file context for RLM (will be populated as files are read)
	fileContext := tools.NewStoredFileContext()

	toolConfig := tools.ToolConfig{MaxRetries: opts.ToolRetries}
	a, err := CreateAgent(agentName, systemPrompt, provider, toolConfig, resultStore, fileContext)
	if err != nil {
		return err
	}

	// Set up storage if session provided
	var store *storage.SqliteStorage
	if sessionID != "" {
		s, err := storage.OpenSqlite(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer s.Close()
		store = s
	}

	session := sessionID
	if session == "" {
		session = "default"
	}

	// Load existing history
	var history []llm.ChatMessage
	if store != nil {
		history, err = store.Load(ctx, session)
		if err != nil {
			return fmt.Errorf("failed to load history: %w", err)
		}
		if len(history) > 0 {
			fmt.Printf("Resuming session '%s' (%d messages)\n\n", session, len(history))
		}
	}

	fmt.Printf("Chat with %s agent. Type 'exit' to quit.\n\n", agentName)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}

		response := a.ExecuteWithHistory(ctx, input, history, opts.MaxIter)

		switch response.Type {
		case agent.ResponseSuccess:
			fmt.Printf("\n%s\n\n", response.Result)

			// Add to history
			history = append(history,
				llm.ChatMessage{Role: "user", Content: input},
				llm.ChatMessage{Role: "assistant", Content: response.Result},
			)

			// Save to storage
			if store != nil {
				if err := store.Save(ctx, session, history); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save history: %v\n", err)
				}
			}
		case agent.ResponseFailure:
			fmt.Fprintf(os.Stderr, "\nError: %s\n\n", response.Error)
		case agent.ResponseTimeout:
			fmt.Printf("\nTimeout: %s\n\n", response.PartialResult)
		}
	}

	return scanner.Err()
}

// Orchestrate executes a complex task across multiple agents.
func Orchestrate(ctx context.Context, task string, agentNames []string, sessionID, dbPath string, opts Options) error {
	provider, err := createProvider(opts.Provider)
	if err != nil {
		return err
	}

	toolConfig := tools.ToolConfig{MaxRetries: opts.ToolRetries}
	llmClient := llm.NewClient(provider)

	// Create ResultStore for RLM pattern (used by both agents and supervisor)
	resultStore, cleanup := createResultStore()
	if cleanup != nil {
		defer cleanup()
	}

	// Pre-store any files mentioned in the task (automatic context)
	fileContext, task := preStoreFilesFromPrompt(ctx, task, resultStore)

	// Create agents with shared file context for RLM pattern
	var agents []*agent.Agent
	if len(agentNames) > 0 {
		for _, name := range agentNames {
			a, err := CreateAgent(name, "", provider, toolConfig, resultStore, fileContext)
			if err != nil {
				return fmt.Errorf("failed to create agent %s: %w", name, err)
			}
			agents = append(agents, a)
		}
	} else {
		agents = CreateDefaultAgents(provider, toolConfig, resultStore, fileContext)
	}

	settings, err := config.New(opts.Provider)
	if err != nil {
		return err
	}

	supervisorConfig := orchestration.SupervisorConfig{
		MaxSubGoals:          settings.Agent.MaxSubGoals,
		MaxIterations:        settings.Agent.MaxIterations,
		LargeResultThreshold: 1024, // 1KB threshold
	}

	supervisor := orchestration.NewSupervisor(agents, llmClient, supervisorConfig)

	// Also give ResultStore to supervisor for storing large agent results
	if resultStore != nil {
		supervisor = supervisor.WithResultStore(resultStore)
	}

	if sessionID != "" {
		store, err := storage.OpenSqlite(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()
		fmt.Printf("Orchestrating task with session '%s'...\n\n", sessionID)
		supervisor = supervisor.WithStorage(store, sessionID)
	} else {
		fmt.Printf("Orchestrating task...\n\n")
	}

	if opts.Verbose {
		supervisor = supervisor.Verbose(true)
	}

	response := supervisor.Orchestrate(ctx, task, opts.MaxIter)

	switch response.Type {
	case orchestration.ResponseSuccess:
		if opts.Verbose {
			printOrchestrationSteps(response.Steps)
		}
		fmt.Printf("%s\n\n", response.Result)
		fmt.Printf("Completed in %d steps\n", len(response.Steps))
		printTokenStats(response.Metadata)
		return nil
	case orchestration.ResponseFailure:
		if opts.Verbose {
			printOrchestrationSteps(response.Steps)
		}
		fmt.Fprintf(os.Stderr, "Failed: %s\n", response.Error)
		fmt.Fprintf(os.Stderr, "Completed %d steps before failure\n", len(response.Steps))
		return fmt.Errorf("orchestration failed: %s", response.Error)
	case orchestration.ResponseTimeout:
		if opts.Verbose {
			printOrchestrationSteps(response.Steps)
		}
		fmt.Printf("Timeout. Partial: %s\n", response.PartialResult)
		fmt.Printf("Completed %d steps\n", len(response.Steps))
		return fmt.Errorf("orchestration timed out")
	default:
		return fmt.Errorf("unknown response type: %v", response.Type)
	}
}

// RLM executes a task using the Recursive Language Model pattern.
// Uses spawn-based architecture where the root agent can spawn sub-agents dynamically.
func RLM(ctx context.Context, task string, maxDepth, timeoutSecs int, mcpServers []string, mcpConfigPath string, opts Options) error {
	startTime := time.Now()

	// Reset metrics for this session
	metrics := tools.ResetMetrics()

	provider, err := createProvider(opts.Provider)
	if err != nil {
		return err
	}

	// Create ResultStore for DSA-based storage/search
	resultStore, cleanup := createResultStore()
	if cleanup != nil {
		defer cleanup()
	}

	// Print metrics at the end
	defer func() {
		metrics.TotalDuration.Store(int64(time.Since(startTime)))
		fmt.Printf("\n--- RLM Metrics ---\n%s\n", metrics.String())
	}()

	// Pre-store any files mentioned in the task
	fileContext, task := preStoreFilesFromPrompt(ctx, task, resultStore)

	// Store the user's prompt as searchable context (RLM pattern)
	if resultStore != nil {
		_, _ = resultStore.StoreContent(ctx, model.ContentKey{ContentType: "file", Path: "user_prompt"}, task)
		fileContext.Add("user_prompt")
	}

	// Session ID for ResultStore - matches model.FileKey namespace so
	// files stored by read_file can be searched by DSA tools
	sessionID := "file"

	// Clear stale session data from previous runs
	// This ensures we only search content stored in THIS session, not old data
	// whose in-memory content is no longer available
	if resultStore != nil {
		_ = resultStore.DeleteSession(ctx, sessionID)
	}

	// Create spawn configuration
	spawnConfig := tools.SpawnConfig{
		MaxDepth:      maxDepth,
		MaxIterations: opts.MaxIter,
		Timeout:       time.Duration(timeoutSecs) * time.Second,
	}
	toolConfig := tools.ToolConfig{MaxRetries: opts.ToolRetries}

	// Build available tools including DSA ResultStore tools
	// Configure read_file to store content for DSA tools (RLM pattern)
	readTool := tools.NewReadFileTool(defaultMaxFileSize)
	if resultStore != nil {
		readTool = readTool.WithContentStore(resultStore).WithFileContext(fileContext)
	}

	availableTools := []tools.Tool{
		readTool,
		tools.NewWriteFileTool(defaultMaxFileSize),
		tools.NewAppendFileTool(defaultMaxFileSize),
		tools.NewEditFileTool(defaultMaxFileSize),
		tools.NewShellTool(defaultTimeout),
		tools.NewGlobTool(1000), // File discovery (paths only, no content)
		tools.NewHTTPTool(defaultTimeout),
		// NOTE: ripgrep intentionally excluded from RLM - use glob + DSA tools instead
	}

	// Add DSA-based ResultStore tools if store is available
	if resultStore != nil {
		availableTools = append(availableTools,
			tools.NewSearchStoredTool(resultStore, sessionID, fileContext),
			tools.NewGetLinesTool(resultStore, sessionID, fileContext),
			tools.NewListStoredTool(resultStore, sessionID, fileContext),
		)
	}

	// Load and connect MCP servers
	allMCPServers, err := loadMCPServers(mcpServers, mcpConfigPath, opts.Verbose)
	if err != nil {
		return err
	}
	mcpConn := connectMCPServers(ctx, allMCPServers, opts.Verbose)
	defer mcpConn.Close()

	// Add MCP tools to available tools
	availableTools, mcpConn.toolNames = mergeTools(availableTools, mcpConn.tools)

	// Create the spawn tool with available tools
	spawnTool := tools.NewSpawnAgentTool(provider, spawnConfig, toolConfig)
	spawnTool = spawnTool.WithTools(availableTools).Verbose(opts.Verbose)

	// Also create parallel spawn tool
	parallelSpawn := tools.NewParallelSpawnTool(spawnTool)

	// Build root agent with spawn capabilities
	allTools := append([]tools.Tool{spawnTool, parallelSpawn}, availableTools...)
	toolMap := make(map[string]tools.Tool)
	for _, t := range allTools {
		toolMap[t.Metadata().Name] = t
	}

	// Build system prompt with MCP tools if any
	mcpToolsSection := buildMCPToolsSection(mcpConn.toolNames)

	systemPrompt := fmt.Sprintf(`You are a root agent using the RLM (Recursive Language Model) pattern.

CRITICAL: This pattern prevents context overflow by storing content externally and working with metadata/summaries.

## Key Capability: SUB-AGENTS
- Use 'spawn' to delegate a specific task to a sub-agent
- Use 'parallel_spawn' to run multiple independent tasks concurrently
- Sub-agents return ONLY answers, not raw content
- You combine sub-agent answers into a final response

## Available Tools

FILE DISCOVERY:
- glob: Find files by pattern (e.g., "**/*.go", "src/**/*.ts") - returns paths only, no content

CONTENT STORAGE (stores for DSA search):
- read_file: Read AND STORE file - returns metadata/summary only, NOT full content

DSA-POWERED SEARCH (requires files stored via read_file first):
- search_stored: Search pattern across ALL stored content (SuffixArray - fast substring search)
- get_lines: Get specific line range from stored content
- list_stored: List stored content with prefix filter (Trie)

SUB-AGENT DELEGATION:
- spawn: Spawn a sub-agent for a specific task
- parallel_spawn: Spawn multiple sub-agents in parallel

FILE MODIFICATION:
- write_file, edit_file, append_file

OTHER:
- execute_shell: Run shell commands
- http_request: Make HTTP requests%s

## REQUIRED WORKFLOW

1. DISCOVER: glob("**/*.go") → returns file paths only
2. STORE: read_file(path) for each file → stores content, returns metadata
3. SEARCH: search_stored("pattern") → finds matches across all stored content
4. EXTRACT: get_lines(path, start, end) → gets specific lines
5. DELEGATE: spawn/parallel_spawn for complex analysis
6. SYNTHESIZE: Combine findings into final answer

## CRITICAL RULES

- NEVER try to process full file content directly
- ALWAYS use glob for discovery (paths only)
- ALWAYS use read_file to store before searching
- ALWAYS use DSA tools (search_stored, get_lines) to examine content
- DELEGATE file analysis to sub-agents for parallelism`, mcpToolsSection)

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task},
	}

	executor := tools.NewExecutor(toolConfig)

	if len(mcpConn.toolNames) > 0 {
		fmt.Printf("Running RLM task (max depth: %d, MCP tools: %d)...\n\n", maxDepth, len(mcpConn.toolNames))
	} else {
		fmt.Printf("Running RLM task (max depth: %d)...\n\n", maxDepth)
	}

	// Run ReAct loop for root agent
	for i := 0; i < opts.MaxIter; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if opts.Verbose {
			fmt.Printf("[root:%d] Processing...\n", i)
		}

		response, err := provider.ChatWithTools(ctx, messages, convertToToolDefs(allTools))
		metrics.LLMCalls.Add(1)
		if err != nil {
			return fmt.Errorf("LLM call failed: %w", err)
		}

		// No tool calls - final answer
		if len(response.ToolCalls) == 0 {
			fmt.Printf("%s\n", response.Content)
			return nil
		}

		if opts.Verbose {
			fmt.Printf("[root:%d] %s\n", i, response.Content)
			for _, tc := range response.ToolCalls {
				args := string(tc.Arguments)
				if len(args) > 100 {
					args = args[:100] + "..."
				}
				fmt.Printf("[root:%d] Calling: %s(%s)\n", i, tc.Name, args)
			}
		}

		// Add assistant message
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
			metrics.ToolCalls.Add(1)
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

			if opts.Verbose {
				displayOutput := output
				if len(displayOutput) > 200 {
					displayOutput = displayOutput[:200] + "..."
				}
				fmt.Printf("[root:%d] Result: %s\n", i, displayOutput)
			}

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    output,
				ToolCallID: tc.ID,
			})
		}
	}

	return fmt.Errorf("reached max iterations without completing")
}

// ReAct executes a task using the ReAct pattern with DSA tools for bounded context.
// Unlike RLM, this uses a single agent without sub-agent spawning.
func ReAct(ctx context.Context, task string, mcpServers []string, mcpConfigPath string, opts Options) error {
	startTime := time.Now()

	provider, err := createProvider(opts.Provider)
	if err != nil {
		return err
	}

	// Create ResultStore for DSA-based storage/search
	resultStore, cleanup := createResultStore()
	if cleanup != nil {
		defer cleanup()
	}

	// Print duration at the end
	defer func() {
		fmt.Printf("\n--- ReAct Metrics ---\n")
		fmt.Printf("Duration: %s\n", time.Since(startTime).Round(time.Millisecond))
	}()

	// Pre-store any files mentioned in the task
	fileContext, task := preStoreFilesFromPrompt(ctx, task, resultStore)

	// Store the user's prompt as searchable context
	if resultStore != nil {
		_, _ = resultStore.StoreContent(ctx, model.ContentKey{ContentType: "file", Path: "user_prompt"}, task)
		fileContext.Add("user_prompt")
	}

	// Session ID for ResultStore
	sessionID := "file"

	// Clear stale session data from previous runs
	if resultStore != nil {
		_ = resultStore.DeleteSession(ctx, sessionID)
	}

	toolConfig := tools.ToolConfig{MaxRetries: opts.ToolRetries}

	// Build available tools including DSA ResultStore tools
	// Configure read_file to store content for DSA tools
	readTool := tools.NewReadFileTool(defaultMaxFileSize)
	if resultStore != nil {
		readTool = readTool.WithContentStore(resultStore).WithFileContext(fileContext)
	}

	// All tools available for ReAct agent
	availableTools := []tools.Tool{
		readTool,
		tools.NewWriteFileTool(defaultMaxFileSize),
		tools.NewAppendFileTool(defaultMaxFileSize),
		tools.NewEditFileTool(defaultMaxFileSize),
		tools.NewShellTool(defaultTimeout),
		tools.NewGlobTool(1000),
		tools.NewHTTPTool(defaultTimeout),
		tools.NewRipgrepTool(defaultTimeout),
	}

	// Add DSA-based ResultStore tools if store is available
	if resultStore != nil {
		availableTools = append(availableTools,
			tools.NewSearchStoredTool(resultStore, sessionID, fileContext),
			tools.NewGetLinesTool(resultStore, sessionID, fileContext),
			tools.NewListStoredTool(resultStore, sessionID, fileContext),
		)
	}

	// Load and connect MCP servers
	allMCPServers, err := loadMCPServers(mcpServers, mcpConfigPath, opts.Verbose)
	if err != nil {
		return err
	}
	mcpConn := connectMCPServers(ctx, allMCPServers, opts.Verbose)
	defer mcpConn.Close()

	// Add MCP tools to available tools
	availableTools, mcpConn.toolNames = mergeTools(availableTools, mcpConn.tools)

	// Build tool map
	toolMap := make(map[string]tools.Tool)
	for _, t := range availableTools {
		toolMap[t.Metadata().Name] = t
	}

	// Build system prompt with MCP tools if any
	mcpToolsSection := buildMCPToolsSection(mcpConn.toolNames)

	systemPrompt := fmt.Sprintf(`You are a ReAct agent with DSA-powered tools for efficient file analysis.

## Key Feature: BOUNDED CONTEXT
Your tools store content externally and return metadata only. This prevents context overflow.

## Available Tools

FILE DISCOVERY:
- glob: Find files by pattern (e.g., "**/*.go", "src/**/*.yaml") - returns paths only

CONTENT STORAGE (stores for DSA search):
- read_file: Read AND STORE file - returns metadata/summary, NOT full content

DSA-POWERED SEARCH (requires files stored via read_file first):
- search_stored: Search pattern across ALL stored content (O(m log n) SuffixArray search)
- get_lines: Get specific line range from stored content
- list_stored: List stored content with prefix filter (O(m+k) Trie lookup)

FILE MODIFICATION:
- write_file, edit_file, append_file

OTHER:
- execute_shell: Run shell commands
- http_request: Make HTTP requests
- ripgrep: Search files on disk (fallback if DSA not applicable)%s

## RECOMMENDED WORKFLOW

1. DISCOVER: glob("**/*.yaml") → returns file paths
2. STORE: read_file(path) for each file → stores content, returns metadata
3. SEARCH: search_stored("pattern") → finds matches across all stored content
4. EXTRACT: get_lines(path, start, end) → gets specific lines you need
5. ANALYZE: Use the extracted content to answer the question

## IMPORTANT
- read_file returns METADATA, not content - use get_lines to fetch specific sections
- search_stored searches ALL stored files at once using SuffixArray
- This is more efficient than ripgrep when analyzing multiple related files`, mcpToolsSection)

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task},
	}

	executor := tools.NewExecutor(toolConfig)

	if len(mcpConn.toolNames) > 0 {
		fmt.Printf("Running ReAct task (MCP tools: %d)...\n\n", len(mcpConn.toolNames))
	} else {
		fmt.Printf("Running ReAct task...\n\n")
	}

	// Run ReAct loop
	for i := 0; i < opts.MaxIter; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if opts.Verbose {
			fmt.Printf("[react:%d] Processing...\n", i)
		}

		response, err := provider.ChatWithTools(ctx, messages, convertToToolDefs(availableTools))
		if err != nil {
			return fmt.Errorf("LLM call failed: %w", err)
		}

		// No tool calls - final answer
		if len(response.ToolCalls) == 0 {
			fmt.Printf("%s\n", response.Content)
			return nil
		}

		if opts.Verbose {
			fmt.Printf("[react:%d] %s\n", i, response.Content)
			for _, tc := range response.ToolCalls {
				args := string(tc.Arguments)
				if len(args) > 100 {
					args = args[:100] + "..."
				}
				fmt.Printf("[react:%d] Calling: %s(%s)\n", i, tc.Name, args)
			}
		}

		// Add assistant message
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

			if opts.Verbose {
				displayOutput := output
				if len(displayOutput) > 200 {
					displayOutput = displayOutput[:200] + "..."
				}
				fmt.Printf("[react:%d] Result: %s\n", i, displayOutput)
			}

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    output,
				ToolCallID: tc.ID,
			})
		}
	}

	return fmt.Errorf("reached max iterations without completing")
}

// ReactChat starts an interactive chat session using ReAct pattern with DSA tools.
func ReactChat(ctx context.Context, sessionID, dbPath string, mcpServers []string, mcpConfigPath string, opts Options) error {
	provider, err := createProvider(opts.Provider)
	if err != nil {
		return err
	}

	// Create ResultStore for DSA-based storage/search
	resultStore, cleanup := createResultStore()
	if cleanup != nil {
		defer cleanup()
	}

	// Create file context for DSA tools
	fileContext := tools.NewStoredFileContext()

	// Session ID for ResultStore operations
	storeSessionID := "file"

	toolConfig := tools.ToolConfig{MaxRetries: opts.ToolRetries}

	// Build available tools including DSA ResultStore tools
	readTool := tools.NewReadFileTool(defaultMaxFileSize)
	if resultStore != nil {
		readTool = readTool.WithContentStore(resultStore).WithFileContext(fileContext)
	}

	availableTools := []tools.Tool{
		readTool,
		tools.NewWriteFileTool(defaultMaxFileSize),
		tools.NewAppendFileTool(defaultMaxFileSize),
		tools.NewEditFileTool(defaultMaxFileSize),
		tools.NewShellTool(defaultTimeout),
		tools.NewGlobTool(1000),
		tools.NewHTTPTool(defaultTimeout),
		tools.NewRipgrepTool(defaultTimeout),
	}

	// Add DSA-based ResultStore tools if store is available
	if resultStore != nil {
		availableTools = append(availableTools,
			tools.NewSearchStoredTool(resultStore, storeSessionID, fileContext),
			tools.NewGetLinesTool(resultStore, storeSessionID, fileContext),
			tools.NewListStoredTool(resultStore, storeSessionID, fileContext),
		)
	}

	// Load and connect MCP servers
	allMCPServers, err := loadMCPServers(mcpServers, mcpConfigPath, opts.Verbose)
	if err != nil {
		return err
	}
	mcpConn := connectMCPServers(ctx, allMCPServers, opts.Verbose)
	defer mcpConn.Close()

	// Add MCP tools to available tools
	availableTools, mcpConn.toolNames = mergeTools(availableTools, mcpConn.tools)

	// Build tool map
	toolMap := make(map[string]tools.Tool)
	for _, t := range availableTools {
		toolMap[t.Metadata().Name] = t
	}

	// Build system prompt with MCP tools if any
	mcpToolsSection := buildMCPToolsSection(mcpConn.toolNames)

	systemPrompt := fmt.Sprintf(`You are a ReAct agent with DSA-powered tools for efficient file analysis.

## Key Feature: BOUNDED CONTEXT
Your tools store content externally and return metadata only. This prevents context overflow.

## Available Tools

FILE DISCOVERY:
- glob: Find files by pattern (e.g., "**/*.go", "src/**/*.yaml") - returns paths only

CONTENT STORAGE (stores for DSA search):
- read_file: Read AND STORE file - returns metadata/summary, NOT full content

DSA-POWERED SEARCH (requires files stored via read_file first):
- search_stored: Search pattern across ALL stored content (O(m log n) SuffixArray search)
- get_lines: Get specific line range from stored content
- list_stored: List stored content with prefix filter (O(m+k) Trie lookup)

FILE MODIFICATION:
- write_file, edit_file, append_file

OTHER:
- execute_shell: Run shell commands
- http_request: Make HTTP requests
- ripgrep: Search files on disk (fallback if DSA not applicable)%s

## RECOMMENDED WORKFLOW

1. DISCOVER: glob("**/*.yaml") → returns file paths
2. STORE: read_file(path) for each file → stores content, returns metadata
3. SEARCH: search_stored("pattern") → finds matches across all stored content
4. EXTRACT: get_lines(path, start, end) → gets specific lines you need
5. ANALYZE: Use the extracted content to answer the question

## IMPORTANT
- read_file returns METADATA, not content - use get_lines to fetch specific sections
- search_stored searches ALL stored files at once using SuffixArray
- This is more efficient than ripgrep when analyzing multiple related files`, mcpToolsSection)

	// Set up conversation persistence if session provided
	var store *storage.SqliteStorage
	if sessionID != "" {
		s, err := storage.OpenSqlite(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer s.Close()
		store = s
	}

	session := sessionID
	if session == "" {
		session = "default"
	}

	// Load existing history
	var history []llm.ChatMessage
	if store != nil {
		history, err = store.Load(ctx, session)
		if err != nil {
			return fmt.Errorf("failed to load history: %w", err)
		}
		if len(history) > 0 {
			fmt.Printf("Resuming session '%s' (%d messages)\n\n", session, len(history))
		}
	}

	fmt.Printf("ReAct Chat with DSA tools. Type 'exit' to quit.\n\n")

	executor := tools.NewExecutor(toolConfig)
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}

		// Pre-store any files mentioned in input.
		// We intentionally discard the returned fileContext - the main fileContext
		// at the function level tracks all stored files across turns.
		_, input = preStoreFilesFromPrompt(ctx, input, resultStore)

		// Build messages for this turn
		messages := []llm.ChatMessage{
			{Role: "system", Content: systemPrompt},
		}
		messages = append(messages, history...)
		messages = append(messages, llm.ChatMessage{Role: "user", Content: input})

		// Run ReAct loop for this turn
		var finalResponse string
		for i := 0; i < opts.MaxIter; i++ {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if opts.Verbose {
				fmt.Printf("[react:%d] Processing...\n", i)
			}

			response, err := provider.ChatWithTools(ctx, messages, convertToToolDefs(availableTools))
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n\n", err)
				break
			}

			// No tool calls - final answer
			if len(response.ToolCalls) == 0 {
				finalResponse = response.Content
				break
			}

			if opts.Verbose {
				fmt.Printf("[react:%d] %s\n", i, response.Content)
				for _, tc := range response.ToolCalls {
					args := string(tc.Arguments)
					if len(args) > 100 {
						args = args[:100] + "..."
					}
					fmt.Printf("[react:%d] Calling: %s(%s)\n", i, tc.Name, args)
				}
			}

			// Add assistant message
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

				if opts.Verbose {
					displayOutput := output
					if len(displayOutput) > 200 {
						displayOutput = displayOutput[:200] + "..."
					}
					fmt.Printf("[react:%d] Result: %s\n", i, displayOutput)
				}

				messages = append(messages, llm.ChatMessage{
					Role:       "tool",
					Content:    output,
					ToolCallID: tc.ID,
				})
			}
		}

		if finalResponse != "" {
			fmt.Printf("\n%s\n\n", finalResponse)

			// Add to history (just user input and final response)
			history = append(history,
				llm.ChatMessage{Role: "user", Content: input},
				llm.ChatMessage{Role: "assistant", Content: finalResponse},
			)

			// Save to storage
			if store != nil {
				if err := store.Save(ctx, session, history); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save history: %v\n", err)
				}
			}
		}
	}

	return scanner.Err()
}

// ReactOrchestrate executes a complex task across multiple agents using ReAct pattern with DSA tools.
func ReactOrchestrate(ctx context.Context, task string, agentNames []string, sessionID, dbPath string, mcpServers []string, mcpConfigPath string, opts Options) error {
	provider, err := createProvider(opts.Provider)
	if err != nil {
		return err
	}

	toolConfig := tools.ToolConfig{MaxRetries: opts.ToolRetries}
	llmClient := llm.NewClient(provider)

	// Create ResultStore for DSA-based storage/search
	resultStore, cleanup := createResultStore()
	if cleanup != nil {
		defer cleanup()
	}

	// Pre-store any files mentioned in the task
	fileContext, task := preStoreFilesFromPrompt(ctx, task, resultStore)

	// Create agents with shared file context
	var agents []*agent.Agent
	if len(agentNames) > 0 {
		for _, name := range agentNames {
			a, err := CreateAgent(name, "", provider, toolConfig, resultStore, fileContext)
			if err != nil {
				return fmt.Errorf("failed to create agent %s: %w", name, err)
			}
			agents = append(agents, a)
		}
	} else {
		agents = CreateDefaultAgents(provider, toolConfig, resultStore, fileContext)
	}

	settings, err := config.New(opts.Provider)
	if err != nil {
		return err
	}

	supervisorConfig := orchestration.SupervisorConfig{
		MaxSubGoals:          settings.Agent.MaxSubGoals,
		MaxIterations:        settings.Agent.MaxIterations,
		LargeResultThreshold: 1024, // 1KB threshold
	}

	supervisor := orchestration.NewSupervisor(agents, llmClient, supervisorConfig)

	// Also give ResultStore to supervisor for storing large agent results
	if resultStore != nil {
		supervisor = supervisor.WithResultStore(resultStore)
	}

	if sessionID != "" {
		store, err := storage.OpenSqlite(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer store.Close()
		fmt.Printf("Orchestrating task with session '%s'...\n\n", sessionID)
		supervisor = supervisor.WithStorage(store, sessionID)
	} else {
		fmt.Printf("Orchestrating task...\n\n")
	}

	if opts.Verbose {
		supervisor = supervisor.Verbose(true)
	}

	response := supervisor.Orchestrate(ctx, task, opts.MaxIter)

	switch response.Type {
	case orchestration.ResponseSuccess:
		if opts.Verbose {
			printOrchestrationSteps(response.Steps)
		}
		fmt.Printf("%s\n\n", response.Result)
		fmt.Printf("Completed in %d steps\n", len(response.Steps))
		printTokenStats(response.Metadata)
		return nil
	case orchestration.ResponseFailure:
		if opts.Verbose {
			printOrchestrationSteps(response.Steps)
		}
		fmt.Fprintf(os.Stderr, "Failed: %s\n", response.Error)
		fmt.Fprintf(os.Stderr, "Completed %d steps before failure\n", len(response.Steps))
		return fmt.Errorf("orchestration failed: %s", response.Error)
	case orchestration.ResponseTimeout:
		if opts.Verbose {
			printOrchestrationSteps(response.Steps)
		}
		fmt.Printf("Timeout. Partial: %s\n", response.PartialResult)
		fmt.Printf("Completed %d steps\n", len(response.Steps))
		return fmt.Errorf("orchestration timed out")
	default:
		return fmt.Errorf("unknown response type: %v", response.Type)
	}
}

// convertToToolDefs converts tools to LLM tool definitions.
func convertToToolDefs(tools []tools.Tool) []llm.ToolDefinition {
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

// ListTools lists all available tools.
func ListTools(verbose bool) {
	registry := tools.NewRegistry()

	// Register default tools (errors ignored - no duplicates in this list)
	_ = registry.Register(tools.NewReadFileTool(defaultMaxFileSize))
	_ = registry.Register(tools.NewWriteFileTool(defaultMaxFileSize))
	_ = registry.Register(tools.NewAppendFileTool(defaultMaxFileSize))
	_ = registry.Register(tools.NewEditFileTool(defaultMaxFileSize))
	_ = registry.Register(tools.NewShellTool(defaultTimeout))
	_ = registry.Register(tools.NewHTTPTool(defaultTimeout))
	_ = registry.Register(tools.NewRipgrepTool(defaultTimeout))

	fmt.Println("Available tools:")
	fmt.Println()

	for _, meta := range registry.List() {
		fmt.Printf("  %s\n", meta.Name)
		fmt.Printf("    %s\n", meta.Description)

		if verbose && len(meta.Parameters) > 0 {
			fmt.Println("    Parameters:")
			for _, param := range meta.Parameters {
				req := ""
				if param.Required {
					req = "*"
				}
				fmt.Printf("      %s%s: %s - %s\n", param.Name, req, param.ParamType, param.Description)
			}
		}
		fmt.Println()
	}
}

// Helper functions

// defaultDBPath is the unified database path for all storage.
const defaultDBPath = ".ariadne/ariadne.db"

// loadMCPServers loads MCP server commands from config and merges with explicit list.
func loadMCPServers(mcpServers []string, mcpConfigPath string, verbose bool) ([]string, error) {
	allServers := mcpServers
	if mcpConfigPath == "" {
		return allServers, nil
	}

	config, err := mcp.LoadConfig(mcpConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load MCP config: %w", err)
	}

	allServers = append(allServers, config.ServerCommands()...)
	if verbose {
		fmt.Printf("Loaded %d MCP servers from config\n", len(config.MCPServers))
	}
	return allServers, nil
}

// mcpConnection holds MCP managers and discovered tools.
type mcpConnection struct {
	managers  []*mcp.ToolManager
	tools     []tools.Tool
	toolNames []string
}

// Close closes all MCP managers.
func (c *mcpConnection) Close() {
	for _, m := range c.managers {
		m.Close()
	}
}

// connectMCPServers connects to MCP servers and discovers their tools.
func connectMCPServers(ctx context.Context, serverCmds []string, verbose bool) *mcpConnection {
	conn := &mcpConnection{}

	for _, serverCmd := range serverCmds {
		parts := strings.Fields(serverCmd)
		if len(parts) == 0 {
			continue
		}
		cmd := parts[0]
		args := parts[1:]

		if verbose {
			fmt.Printf("Connecting to MCP server: %s\n", serverCmd)
		}

		manager, err := mcp.DiscoverTools(ctx, cmd, args...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to connect to MCP server '%s': %v\n", serverCmd, err)
			continue
		}

		conn.managers = append(conn.managers, manager)
		for _, tool := range manager.Tools() {
			conn.tools = append(conn.tools, tool)
			conn.toolNames = append(conn.toolNames, tool.Metadata().Name)
		}

		if verbose {
			fmt.Printf("Discovered %d tools from MCP server\n", len(manager.Tools()))
		}
	}

	return conn
}

// buildMCPToolsSection creates the MCP tools section for system prompts.
func buildMCPToolsSection(toolNames []string) string {
	if len(toolNames) == 0 {
		return ""
	}
	return fmt.Sprintf("\n\nMCP Tools (from connected servers):\n- %s", strings.Join(toolNames, "\n- "))
}

// mergeTools combines base tools with MCP tools, skipping MCP tools that duplicate base tool names.
// This prevents "Duplicate function declaration" errors from LLM providers.
// Returns the merged tools and the names of MCP tools that were actually added.
func mergeTools(baseTools []tools.Tool, mcpTools []tools.Tool) ([]tools.Tool, []string) {
	// Build set of base tool names
	baseNames := make(map[string]bool)
	for _, t := range baseTools {
		baseNames[t.Metadata().Name] = true
	}

	// Add MCP tools that don't conflict with base tools
	result := make([]tools.Tool, len(baseTools))
	copy(result, baseTools)

	var addedNames []string
	for _, t := range mcpTools {
		name := t.Metadata().Name
		if baseNames[name] {
			// Skip duplicate - prefer built-in tool
			continue
		}
		result = append(result, t)
		addedNames = append(addedNames, name)
	}

	return result, addedNames
}

// createResultStore creates a ResultStore for RLM pattern.
// Returns the store and a cleanup function (may be nil if creation fails).
func createResultStore() (*storage.ResultStore, func()) {
	// Open unified SQLite storage for ContentStorage
	db, err := storage.OpenSqlite(defaultDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: RLM disabled, failed to open database: %v\n", err)
		return nil, nil
	}

	store, err := storage.NewResultStore(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: RLM disabled, failed to create store: %v\n", err)
		db.Close()
		return nil, nil
	}

	return store, func() {
		_ = store.Close() // Best-effort cleanup
		_ = db.Close()
	}
}

// preStoreFilesFromPrompt detects file paths in the prompt and pre-stores them.
// Returns the file context with stored files and a modified prompt with metadata.
func preStoreFilesFromPrompt(ctx context.Context, prompt string, store *storage.ResultStore) (*tools.StoredFileContext, string) {
	fileContext := tools.NewStoredFileContext()
	if store == nil {
		return fileContext, prompt
	}

	// Find file paths in prompt (absolute paths starting with /)
	paths := extractFilePaths(prompt)
	if len(paths) == 0 {
		return fileContext, prompt
	}

	var storedInfo []string
	for _, path := range paths {
		// Check if file exists and read it
		content, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files that can't be read
		}

		// Store in ResultStore
		key := storage.ResultKey{
			SessionID: "file",
			Key:       path,
		}
		meta, err := store.Store(ctx, key, string(content), storage.DefaultStoreOptions())
		if err != nil {
			continue
		}

		// Track in context
		fileContext.Add(path)
		storedInfo = append(storedInfo, fmt.Sprintf("- %s (%d lines, %d bytes)", path, meta.LineCount, meta.ByteSize))
	}

	// Add context info to prompt if files were stored
	if len(storedInfo) > 0 {
		contextNote := fmt.Sprintf("\n\n[Files pre-stored as context - use get_lines to read, key is automatic]\n%s", strings.Join(storedInfo, "\n"))
		return fileContext, prompt + contextNote
	}

	return fileContext, prompt
}

// extractFilePaths finds absolute file paths in text.
func extractFilePaths(text string) []string {
	var paths []string
	words := strings.Fields(text)
	for _, word := range words {
		// Clean up punctuation
		word = strings.Trim(word, "\"',;:()[]{}")
		// Check if it looks like an absolute path
		if strings.HasPrefix(word, "/") && len(word) > 1 {
			// Basic validation - contains at least one more /
			if strings.Contains(word[1:], "/") || strings.Contains(word, ".") {
				paths = append(paths, word)
			}
		}
	}
	return paths
}

func createProvider(providerName string) (llm.Provider, error) {
	if providerName == "" {
		return nil, fmt.Errorf("--provider is required for this command")
	}

	providerType, err := llm.ParseProviderType(providerName)
	if err != nil {
		return nil, err
	}

	settings, err := config.New(providerName)
	if err != nil {
		return nil, err
	}

	apiKey, err := config.APIKeyFor(providerName)
	if err != nil {
		return nil, err
	}

	return providerType.
		Model(settings.LLM.Model).
		MaxTokens(settings.LLM.MaxTokens).
		Temperature(float32(settings.LLM.Temperature)).
		APIKey(apiKey)
}

const (
	maxAgentObservationLen        = 400
	maxOrchestrationObservationLen = 200
)

func printAgentSteps(steps []agent.Step) {
	fmt.Println("--- Steps ---")
	for _, step := range steps {
		fmt.Printf("[%d] %s\n", step.Iteration, step.Thought)
		if step.Action != nil {
			fmt.Printf("    Action: %s\n", *step.Action)
		}
		if step.Observation != nil {
			obs := truncateString(*step.Observation, maxAgentObservationLen)
			fmt.Printf("    Observation: %s\n", obs)
		}
		fmt.Println()
	}
	fmt.Println("-------------")
	fmt.Println()
}

func printOrchestrationSteps(steps []orchestration.Step) {
	fmt.Println("--- Steps ---")
	for _, step := range steps {
		fmt.Printf("[%d] %s\n", step.Iteration, step.Thought)
		if step.Action != nil {
			fmt.Printf("    Action: %s\n", *step.Action)
		}
		if step.Observation != nil {
			obs := truncateString(*step.Observation, maxOrchestrationObservationLen)
			fmt.Printf("    Observation: %s\n", obs)
		}
		fmt.Println()
	}
	fmt.Println("-------------")
	fmt.Println()
}

// truncateString truncates a string to maxLen runes, preserving UTF-8 boundaries.
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// bytesPerToken is the approximate bytes per token for estimation.
const bytesPerToken = 4

// printTokenStats prints token usage statistics.
func printTokenStats(meta *orchestration.Metadata) {
	if meta == nil || meta.TokenStats == nil {
		return
	}
	stats := meta.TokenStats
	fmt.Printf("\nToken Usage:\n")
	fmt.Printf("  LLM calls: %d\n", stats.LLMCalls)
	fmt.Printf("  Prompt tokens: %d\n", stats.PromptTokens)
	fmt.Printf("  Completion tokens: %d\n", stats.CompletionTokens)
	fmt.Printf("  Total tokens: %d\n", stats.TotalTokens)
	if stats.ResultsStored > 0 {
		fmt.Printf("  Results stored: %d\n", stats.ResultsStored)
		fmt.Printf("  Context bytes saved: %d (~%d tokens)\n", stats.BytesSaved, stats.BytesSaved/bytesPerToken)
	}
}
