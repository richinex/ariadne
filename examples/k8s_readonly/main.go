// Kubernetes read-only agent example
//
// Demonstrates a specialized agent for Kubernetes read-only operations.
// Uses shell tool with kubectl for cluster inspection.
//
// Run with:
//   KUBECONFIG=path/to/kubeconfig go run ./examples/k8s_readonly

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

	providerName := os.Getenv("LLM_PROVIDER")
	if providerName == "" {
		providerName = "deepseek"
	}

	provider, err := createProvider(providerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== K8s Read-Only Agent Example ===")
	fmt.Printf("\nUsing: %s (%s)\n\n", provider.Name(), provider.Model())

	// Check if kubectl is available
	if os.Getenv("KUBECONFIG") == "" {
		fmt.Println("Note: KUBECONFIG not set. kubectl commands may fail.")
	}

	// Create agent with shell and ripgrep tools
	// The system prompt enforces read-only kubectl usage
	cfg := agent.NewBuilder("k8s_readonly").
		Description("Kubernetes read-only operations").
		SystemPrompt(
			"You are a Kubernetes operations specialist. Use ONLY kubectl via the " +
				"execute_shell tool. Use read-only kubectl subcommands (get, describe, logs, explain, top). " +
				"Do NOT apply, delete, patch, or scale resources unless explicitly requested. " +
				"Always limit log volume: use --since (<=1h) and/or --tail (<=200) for kubectl logs. " +
				"If you need to search files, use the ripgrep tool instead of shell commands like grep.",
		).
		Tool(tools.NewShellTool(30)).
		Tool(tools.NewRipgrepTool(30)).
		Build()

	a := agent.New(cfg, provider)

	task := "List cluster nodes and show their status. If kubectl is not configured, explain what the command would be."
	fmt.Printf("Task: %s\n\n", task)

	response := a.Execute(context.Background(), task, 5)

	switch response.Type {
	case agent.ResponseSuccess:
		fmt.Printf("Result:\n%s\n", response.Result)
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
