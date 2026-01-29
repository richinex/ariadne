// RLM Spawn Example - Demonstrates recursive sub-agent spawning.
//
// This example shows the true RLM pattern where:
// - A root agent spawns sub-agents to handle specific tasks
// - Sub-agents can spawn their own sub-agents (recursive)
// - Children return only answers, not raw content
// - Parent never sees raw content, only summaries
//
// Usage:
//   OPENAI_API_KEY=xxx go run examples/rlm_spawn/main.go

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/richinex/davingo/llm"
	"github.com/richinex/davingo/tools"
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable required")
	}

	ctx := context.Background()

	// Create LLM provider
	provider := llm.NewOpenAIProvider(apiKey, "gpt-4o-mini", 4096, 0.7)

	// Create spawn tool with configuration
	spawnConfig := tools.SpawnConfig{
		MaxDepth:      3,  // Allow up to 3 levels of recursion
		MaxIterations: 5,  // Max iterations per agent
		Timeout:       60, // 60 second timeout per agent
	}
	toolConfig := tools.DefaultToolConfig()

	// Create the spawn tool
	spawnTool := tools.NewSpawnAgentTool(provider, spawnConfig, toolConfig)

	// Add other tools that sub-agents can use
	availableTools := []tools.Tool{
		tools.NewReadFileTool(1024 * 1024), // 1MB max
		tools.NewRipgrepTool(30),           // 30 second timeout
		tools.NewShellTool(30),
	}
	spawnTool = spawnTool.WithTools(availableTools)

	// Also create parallel spawn tool
	parallelSpawn := tools.NewParallelSpawnTool(spawnTool)

	// Demo 1: Simple spawn
	fmt.Println("=== Demo 1: Simple Spawn ===")
	fmt.Println("Spawning a sub-agent to analyze current directory...")

	args1, _ := json.Marshal(map[string]string{
		"task":    "List the Go files in the current directory and describe what each one does based on the filename.",
		"context": "You are in a Go project directory.",
	})

	result1, err := spawnTool.Execute(ctx, args1)
	if err != nil {
		log.Fatalf("Spawn failed: %v", err)
	}

	fmt.Printf("Result:\n%s\n\n", result1.Output)

	// Demo 2: Parallel spawn
	fmt.Println("=== Demo 2: Parallel Spawn ===")
	fmt.Println("Spawning multiple sub-agents in parallel...")

	args2, _ := json.Marshal(map[string]interface{}{
		"tasks": []map[string]string{
			{"task": "What is 2 + 2?"},
			{"task": "What is 10 * 5?"},
			{"task": "What is 100 / 4?"},
		},
	})

	result2, err := parallelSpawn.Execute(ctx, args2)
	if err != nil {
		log.Fatalf("Parallel spawn failed: %v", err)
	}

	fmt.Printf("Results:\n%s\n\n", result2.Output)

	fmt.Println("=== RLM Demo Complete ===")
	fmt.Println("The spawn tool enables recursive sub-agent patterns.")
	fmt.Println("Sub-agents executed, returned answers, and terminated.")
	fmt.Println("No raw content was passed to the parent - only summaries.")
}
