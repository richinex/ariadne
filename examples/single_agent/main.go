// Single agent task execution
//
// Demonstrates creating and using a single agent to complete a task.
//
// Run with: go run ./examples/single_agent

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/richinex/ariadne/agent"
	"github.com/richinex/ariadne/config"
	"github.com/richinex/ariadne/llm"
	"github.com/richinex/ariadne/tools"
)

func main() {
	godotenv.Load()

	// Get provider from environment or use deepseek
	providerName := os.Getenv("LLM_PROVIDER")
	if providerName == "" {
		providerName = "deepseek"
	}

	provider, err := createProvider(providerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Single Agent Example ===")
	fmt.Printf("\nUsing: %s (%s)\n\n", provider.Name(), provider.Model())

	// Create agent with specific tools
	cfg := agent.NewBuilder("file_inspector").
		Description("Inspects files and directories").
		SystemPrompt(
			"You are a file system inspector. Use tools to examine files and directories. " +
				"Be concise and clear in your responses.",
		).
		Tool(tools.NewReadFileTool(1024 * 1024)).
		Tool(tools.NewShellTool(30)).
		Build()

	a := agent.New(cfg, provider)

	fmt.Printf("Agent: %s - %s\n\n", a.Name(), a.Description())

	// Execute task
	task := "List the Go source files in the current directory"
	fmt.Printf("Task: %s\n\n", task)

	response := a.Execute(context.Background(), task, 5)

	switch response.Type {
	case agent.ResponseSuccess:
		fmt.Printf("Result:\n%s\n\n", response.Result)
		fmt.Printf("Steps: %d\n", len(response.Steps))
		fmt.Printf("Execution time: %dms\n", response.Metadata.ExecutionTimeMs)
	case agent.ResponseFailure:
		fmt.Printf("Failed: %s\n", response.Error)
	case agent.ResponseTimeout:
		fmt.Printf("Timeout: %s\n", response.PartialResult)
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
