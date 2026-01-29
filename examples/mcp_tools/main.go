// MCP (Model Context Protocol) tool discovery
//
// Demonstrates discovering tools from an MCP server and using them with agents.
// MCP allows external services to provide tools to your agents.
//
// Prerequisites:
// - Node.js installed (for npx)
//
// Run with: go run ./examples/mcp_tools

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/richinex/ariadne/agent"
	"github.com/richinex/ariadne/config"
	"github.com/richinex/ariadne/llm"
	"github.com/richinex/ariadne/mcp"
)

func main() {
	godotenv.Load()

	providerName := os.Getenv("LLM_PROVIDER")
	if providerName == "" {
		providerName = "deepseek"
	}

	provider, err := createProvider(providerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== MCP Tools Example ===")
	fmt.Printf("Using: %s (%s)\n\n", provider.Name(), provider.Model())

	// Discover tools from an MCP server
	fmt.Println("Discovering tools from MCP filesystem server...")

	toolManager, err := mcp.DiscoverTools(
		context.Background(),
		"npx",
		"-y", "@modelcontextprotocol/server-filesystem", "/tmp",
	)

	var builder *agent.Builder

	if err != nil {
		fmt.Printf("Failed to discover MCP tools: %v\n", err)
		fmt.Println("Make sure Node.js is installed and npx is available.")
		fmt.Println("Continuing without MCP tools...")

		builder = agent.NewBuilder("mcp_agent").
			Description("Agent with MCP-provided tools").
			SystemPrompt(
				"You are an assistant. MCP tools are not available, " +
					"so please explain what you would do if you had filesystem tools.",
			)
	} else {
		defer toolManager.Close()

		tools := toolManager.Tools()
		fmt.Printf("Discovered %d tools:\n", len(tools))
		for _, tool := range tools {
			meta := tool.Metadata()
			fmt.Printf("  - %s: %s\n", meta.Name, meta.Description)
		}
		fmt.Println()

		// Build agent with discovered MCP tools
		builder = agent.NewBuilder("mcp_agent").
			Description("Agent with MCP-provided tools").
			SystemPrompt(
				"You are an assistant with access to filesystem tools via MCP. " +
					"Use the available tools to help with file operations.",
			)

		for _, tool := range tools {
			builder = builder.Tool(tool)
		}
	}

	a := agent.New(builder.Build(), provider)

	// Test the agent
	task := "List the contents of /tmp directory"
	fmt.Printf("Task: %s\n\n", task)

	response := a.Execute(context.Background(), task, 5)

	switch response.Type {
	case agent.ResponseSuccess:
		fmt.Printf("Result:\n%s\n\n", response.Result)
		fmt.Printf("Steps taken: %d\n", len(response.Steps))
	case agent.ResponseFailure:
		fmt.Printf("Failed: %s\n", response.Error)
	case agent.ResponseTimeout:
		fmt.Println("Timeout")
	}
}

func createProvider(providerName string) (llm.Provider, error) {
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
