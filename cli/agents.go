// Pre-built agent configurations for CLI commands.
//
// Information Hiding:
// - Agent creation details hidden
// - Tool configuration hidden

package cli

import (
	"github.com/richinex/davingo/agent"
	"github.com/richinex/davingo/llm"
	"github.com/richinex/davingo/storage"
	"github.com/richinex/davingo/tools"
)

// AgentType represents available agent types.
type AgentType string

const (
	AgentGeneral AgentType = "general"
	AgentFile    AgentType = "file"
	AgentShell   AgentType = "shell"
	AgentWeb     AgentType = "web"
)

const (
	defaultMaxFileSize = 1024 * 1024 // 1MB
	defaultTimeout     = 30          // seconds
)

// CreateAgent creates an agent by name with the given provider.
// resultStore is optional - if provided, enables RLM pattern with full ResultStore capabilities.
// fileContext is optional - if provided, uses shared context for tracking stored files.
func CreateAgent(name string, systemPrompt string, provider llm.Provider, toolConfig tools.ToolConfig, resultStore *storage.ResultStore, fileContext *tools.StoredFileContext) (*agent.Agent, error) {
	var builder *agent.Builder

	switch AgentType(name) {
	case AgentGeneral:
		prompt := systemPrompt
		if prompt == "" {
			prompt = "You are a helpful assistant. Answer questions clearly and concisely."
		}
		builder = agent.NewBuilder("general").
			Description("General assistant").
			SystemPrompt(prompt)

	case AgentFile:
		prompt := systemPrompt
		if prompt == "" {
			prompt = `You are a file operations specialist using the RLM (Recursive Language Model) pattern.

CRITICAL - How file reading works:
- When you read a file, it gets stored automatically and tracked as context
- You will NOT see file content - just metadata (size, lines)
- Use get_lines to retrieve content - the key is AUTOMATIC (uses last stored file)
- Do NOT hallucinate or guess content - retrieve it first

Available tools:
1. get_lines - Get lines from stored file: {"start": 1, "end": 100} (key is automatic)
2. search_stored - Search ALL stored content: {"pattern": "keyword"}
3. list_stored - List what's been stored

Workflow:
1. read_file → file is stored, tracked automatically
2. get_lines → retrieve content (no key needed, uses last stored file)
3. Build understanding from retrieved content ONLY

Example: After read_file, just call get_lines with start/end - no key needed.

NEVER provide a summary without first retrieving actual content via get_lines.`
		}
		// Use provided file context or create new one
		if fileContext == nil {
			fileContext = tools.NewStoredFileContext()
		}
		sessionID := "file" // Default session for file operations

		// Create ReadFileTool with optional ResultStore for RLM pattern
		readTool := tools.NewReadFileTool(defaultMaxFileSize)
		if resultStore != nil {
			readTool = readTool.WithContentStore(resultStore).WithFileContext(fileContext)
		}

		builder = agent.NewBuilder("file").
			Description("File operations agent with search capabilities").
			SystemPrompt(prompt).
			Tool(readTool).
			Tool(tools.NewWriteFileTool(defaultMaxFileSize)).
			Tool(tools.NewAppendFileTool(defaultMaxFileSize)).
			Tool(tools.NewRipgrepTool(defaultTimeout)).
			Tool(tools.NewShellTool(defaultTimeout))

		// Add ResultStore tools if available (full RLM capabilities)
		if resultStore != nil {
			builder = builder.
				Tool(tools.NewSearchStoredTool(resultStore, sessionID, fileContext)).
				Tool(tools.NewGetLinesTool(resultStore, sessionID, fileContext)).
				Tool(tools.NewListStoredTool(resultStore, sessionID, fileContext))
		}

	case AgentShell:
		prompt := systemPrompt
		if prompt == "" {
			prompt = "You are a shell command specialist. Execute commands safely and report results."
		}
		builder = agent.NewBuilder("shell").
			Description("Shell command executor").
			SystemPrompt(prompt).
			Tool(tools.NewShellTool(defaultTimeout))

	case AgentWeb:
		prompt := systemPrompt
		if prompt == "" {
			prompt = "You are an HTTP client specialist. Make web requests and process responses."
		}
		builder = agent.NewBuilder("web").
			Description("HTTP client agent").
			SystemPrompt(prompt).
			Tool(tools.NewHTTPTool(defaultTimeout))

	default:
		// Fall back to general
		prompt := systemPrompt
		if prompt == "" {
			prompt = "You are a helpful assistant."
		}
		builder = agent.NewBuilder(name).
			Description("Custom agent").
			SystemPrompt(prompt)
	}

	config := builder.Build()
	a := agent.New(config, provider).WithToolConfig(toolConfig)

	return a, nil
}

// CreateDefaultAgents creates the default set of agents for orchestration.
// resultStore is optional - if provided, file agent will use RLM pattern.
// fileContext is optional - if provided, shares context across agents.
func CreateDefaultAgents(provider llm.Provider, toolConfig tools.ToolConfig, resultStore *storage.ResultStore, fileContext *tools.StoredFileContext) []*agent.Agent {
	agents := make([]*agent.Agent, 0, 4)

	for _, agentType := range []AgentType{AgentGeneral, AgentFile, AgentShell, AgentWeb} {
		// Error is always nil for known agent types
		a, _ := CreateAgent(string(agentType), "", provider, toolConfig, resultStore, fileContext)
		agents = append(agents, a)
	}

	return agents
}

// ListAvailableAgents returns the names and descriptions of available agents.
func ListAvailableAgents() []agent.AgentInfo {
	return []agent.AgentInfo{
		{Name: "general", Description: "General assistant - answer questions and provide help"},
		{Name: "file", Description: "File operations - read, write, append, search with ripgrep/shell"},
		{Name: "shell", Description: "Shell commands - execute terminal commands"},
		{Name: "web", Description: "HTTP requests - fetch data from web APIs"},
	}
}
