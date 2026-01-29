// Multi-agent business workflow with persistent memory
//
// Demonstrates distinct business domain agents collaborating:
// - Researcher: Gathers information from files and sources
// - Analyst: Processes data and identifies patterns
// - Reporter: Synthesizes findings into actionable reports
//
// Run with: go run ./examples/multi_agent_memory

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/richinex/davingo/agent"
	"github.com/richinex/davingo/config"
	"github.com/richinex/davingo/llm"
	"github.com/richinex/davingo/orchestration"
	"github.com/richinex/davingo/storage"
	"github.com/richinex/davingo/tools"
)

// researcher creates a Research agent - gathers information
func researcher(provider llm.Provider) *agent.Agent {
	return agent.New(
		agent.NewBuilder("researcher").
			Description("Gathers information from files, directories, and system sources. Use for data collection tasks.").
			SystemPrompt(
				"You are a Research Specialist. Your role is to gather raw information and data.\n\n"+
					"Responsibilities:\n"+
					"- Read files and extract relevant content\n"+
					"- List directory contents to discover available data\n"+
					"- Execute commands to gather system information\n"+
					"- Report findings in a structured format\n\n"+
					"Always be thorough and report exactly what you find, without interpretation.",
			).
			Tool(tools.NewReadFileTool(1024 * 1024)).
			Tool(tools.NewShellTool(30)).
			Build(),
		provider,
	)
}

// analyst creates an Analyst agent - processes and interprets data
func analyst(provider llm.Provider) *agent.Agent {
	return agent.New(
		agent.NewBuilder("analyst").
			Description("Analyzes data, identifies patterns, and provides insights. Use after data has been gathered.").
			SystemPrompt(
				"You are a Data Analyst. Your role is to process information and extract insights.\n\n"+
					"Responsibilities:\n"+
					"- Analyze data provided in your task description\n"+
					"- Identify patterns, anomalies, and key metrics\n"+
					"- Compare and contrast different data points\n"+
					"- Provide quantitative assessments when possible\n\n"+
					"Focus on facts and measurable observations. Separate findings from interpretations.",
			).
			Tool(tools.NewShellTool(30)).
			Build(),
		provider,
	)
}

// reporter creates a Reporter agent - synthesizes findings into reports
func reporter(provider llm.Provider) *agent.Agent {
	return agent.New(
		agent.NewBuilder("reporter").
			Description("Synthesizes findings into clear reports with recommendations. Use as the final step.").
			SystemPrompt(
				"You are a Report Writer. Your role is to synthesize information into actionable reports.\n\n"+
					"Responsibilities:\n"+
					"- Combine findings from multiple sources\n"+
					"- Write clear, structured summaries\n"+
					"- Highlight key takeaways and action items\n"+
					"- Format output for easy consumption\n\n"+
					"Be concise but comprehensive. Use bullet points and clear headings.",
			).
			Tool(tools.NewWriteFileTool(1024 * 1024)).
			Build(),
		provider,
	)
}

// codeReviewer creates a Code Reviewer agent
func codeReviewer(provider llm.Provider) *agent.Agent {
	return agent.New(
		agent.NewBuilder("code_reviewer").
			Description("Reviews code for quality, patterns, and potential issues. Specialized in code analysis.").
			SystemPrompt(
				"You are a Code Review Specialist. Your role is to analyze code quality.\n\n"+
					"Responsibilities:\n"+
					"- Review code structure and organization\n"+
					"- Identify potential bugs or issues\n"+
					"- Assess code style and best practices\n"+
					"- Suggest improvements\n\n"+
					"Be specific about line numbers and concrete suggestions.",
			).
			Tool(tools.NewReadFileTool(1024 * 1024)).
			Tool(tools.NewShellTool(30)).
			Build(),
		provider,
	)
}

func main() {
	godotenv.Load()

	fmt.Println("=== Multi-Agent Business Workflow ===")

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

	// Storage setup - SQLite for conversation and memory persistence
	store, err := storage.OpenSqlite("./business_workflow.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	sessionID := "code-review-workflow"

	// Check for existing session
	history, err := store.Load(context.Background(), sessionID)
	if err == nil && len(history) > 0 {
		fmt.Printf("Resuming workflow with %d previous interactions\n\n", len(history))
	} else {
		fmt.Println("Starting new workflow session")
	}

	// Create agents
	agents := []*agent.Agent{
		researcher(provider),
		analyst(provider),
		reporter(provider),
		codeReviewer(provider),
	}

	fmt.Println("Business Agents:")
	for _, a := range agents {
		fmt.Printf("  - %s: %s\n", a.Name(), a.Description())
	}
	fmt.Println()

	// Supervisor setup with memory persistence
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
	supervisor := orchestration.NewSupervisor(agents, llmClient, supervisorConfig).
		WithStorage(store, sessionID)

	// Business workflow task
	task := "Analyze the codebase structure in the current directory. " +
		"First, have the researcher gather information about the Go files and their sizes. " +
		"Then, have the analyst identify which modules are largest and most complex. " +
		"Finally, have the reporter create a summary of the codebase architecture."

	fmt.Printf("Business Task:\n%s\n\n", task)
	fmt.Println("Orchestrating workflow...")
	fmt.Println("---")

	response := supervisor.Orchestrate(context.Background(), task, 10)

	switch response.Type {
	case orchestration.ResponseSuccess:
		fmt.Println("\n=== Workflow Complete ===")
		fmt.Printf("Final Report:\n%s\n\n", response.Result)

		fmt.Println("Agent Handoffs:")
		for i, step := range response.Steps {
			if step.Action != nil {
				action := *step.Action
				if len(action) > 70 {
					action = action[:70] + "..."
				}
				fmt.Printf("  %d. %s\n", i+1, action)
			}
		}

		// Save the workflow result
		newHistory := append(history,
			llm.ChatMessage{Role: "user", Content: task},
			llm.ChatMessage{Role: "assistant", Content: response.Result},
		)
		if err := store.Save(context.Background(), sessionID, newHistory); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save history: %v\n", err)
		} else {
			fmt.Printf("\nWorkflow saved to SQLite (session: %s)\n", sessionID)
		}

	case orchestration.ResponseFailure:
		fmt.Printf("Workflow failed: %s\n", response.Error)
		fmt.Printf("Completed %d steps before failure\n", len(response.Steps))

	case orchestration.ResponseTimeout:
		fmt.Printf("Workflow timeout: %s\n", response.PartialResult)
		fmt.Printf("Completed %d steps\n", len(response.Steps))
	}

	// Show session history
	fmt.Println("\n=== Session History ===")
	sessions, _ := store.ListSessions(context.Background())
	for _, sid := range sessions {
		h, _ := store.Load(context.Background(), sid)
		fmt.Printf("  - %s (%d messages)\n", sid, len(h))
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
