// Multi-agent orchestration with supervisor
//
// Demonstrates the supervisor pattern: decomposing complex tasks
// and coordinating multiple agents to accomplish them.
//
// Run with: go run ./examples/supervisor

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/richinex/ariadne/agent"
	"github.com/richinex/ariadne/config"
	"github.com/richinex/ariadne/llm"
	"github.com/richinex/ariadne/orchestration"
	"github.com/richinex/ariadne/tools"
)

func main() {
	godotenv.Load()

	fmt.Println("=== Supervisor Example ===")

	providerName := os.Getenv("LLM_PROVIDER")
	if providerName == "" {
		providerName = "deepseek"
	}

	provider, err := createProvider(providerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Using: %s (%s)\n\n", provider.Name(), provider.Model())

	// Create specialized agents
	fileAgent := agent.New(
		agent.NewBuilder("file").
			Description("File operations - read and write files").
			SystemPrompt("You are a file operations specialist. Use the available tools to work with files.").
			Tool(tools.NewReadFileTool(1024 * 1024)).
			Tool(tools.NewWriteFileTool(1024 * 1024)).
			Build(),
		provider,
	)

	shellAgent := agent.New(
		agent.NewBuilder("shell").
			Description("Shell commands - execute terminal commands").
			SystemPrompt("You are a shell command specialist. Execute commands safely and report results.").
			Tool(tools.NewShellTool(30)).
			Build(),
		provider,
	)

	generalAgent := agent.New(
		agent.NewBuilder("general").
			Description("General assistant - answer questions and provide help").
			SystemPrompt("You are a helpful general assistant. Answer questions clearly and concisely.").
			Build(),
		provider,
	)

	agents := []*agent.Agent{fileAgent, shellAgent, generalAgent}

	fmt.Println("Available agents for orchestration:")
	for _, a := range agents {
		fmt.Printf("  - %s\n", a.Name())
	}
	fmt.Println()

	// Get settings for supervisor config
	settings, err := config.New(providerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load settings: %v\n", err)
		os.Exit(1)
	}

	supervisorConfig := orchestration.SupervisorConfig{
		MaxSubGoals:   settings.Agent.MaxSubGoals,
		MaxIterations: settings.Agent.MaxIterations,
	}

	llmClient := llm.NewClient(provider)
	supervisor := orchestration.NewSupervisor(agents, llmClient, supervisorConfig)

	// Execute task
	task := "List the Go source files in the current directory, count how many there are, " +
		"and tell me which one has the most lines"

	fmt.Printf("Task: %s\n\n", task)
	fmt.Println("Orchestrating...")

	response := supervisor.Orchestrate(context.Background(), task, 8)

	switch response.Type {
	case orchestration.ResponseSuccess:
		fmt.Printf("Result:\n%s\n\n", response.Result)
		fmt.Printf("Orchestration steps: %d\n", len(response.Steps))
		for i, step := range response.Steps {
			if step.Action != nil {
				fmt.Printf("  Step %d: %s\n", i+1, *step.Action)
			}
		}
	case orchestration.ResponseFailure:
		fmt.Printf("Failed: %s\n", response.Error)
	case orchestration.ResponseTimeout:
		fmt.Printf("Timeout after %d steps: %s\n", len(response.Steps), response.PartialResult)
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
