// Creating agents with custom tools
//
// Demonstrates how to create custom tools by implementing the Tool interface
// and use them with agents.
//
// Run with: go run ./examples/custom_tools

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/richinex/davingo/agent"
	"github.com/richinex/davingo/config"
	"github.com/richinex/davingo/llm"
	"github.com/richinex/davingo/tools"
)

// CalculateTool performs basic arithmetic
type CalculateTool struct{}

func (t *CalculateTool) Metadata() tools.ToolMetadata {
	return tools.ToolMetadata{
		Name:        "calculate",
		Description: "Perform basic arithmetic. Supports add, subtract, multiply, divide.",
		Parameters: []tools.ToolParameter{
			{Name: "operation", ParamType: "string", Description: "The operation: add, subtract, multiply, or divide", Required: true},
			{Name: "a", ParamType: "number", Description: "First operand", Required: true},
			{Name: "b", ParamType: "number", Description: "Second operand", Required: true},
		},
	}
}

func (t *CalculateTool) Validate(args json.RawMessage) error {
	var params struct {
		Operation string  `json:"operation"`
		A         float64 `json:"a"`
		B         float64 `json:"b"`
	}
	return json.Unmarshal(args, &params)
}

func (t *CalculateTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params struct {
		Operation string  `json:"operation"`
		A         float64 `json:"a"`
		B         float64 `json:"b"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.FailureResultf("Invalid arguments: %v", err), nil
	}

	var result float64
	switch params.Operation {
	case "add":
		result = params.A + params.B
	case "subtract":
		result = params.A - params.B
	case "multiply":
		result = params.A * params.B
	case "divide":
		if params.B == 0 {
			return tools.FailureResultf("Error: Division by zero"), nil
		}
		result = params.A / params.B
	default:
		return tools.FailureResultf("Unknown operation: %s", params.Operation), nil
	}

	return tools.SuccessResult(fmt.Sprintf("%.0f %s %.0f = %.2f", params.A, params.Operation, params.B, result)), nil
}

// GreetTool generates personalized greetings
type GreetTool struct{}

func (t *GreetTool) Metadata() tools.ToolMetadata {
	return tools.ToolMetadata{
		Name:        "greet",
		Description: "Generate a personalized greeting",
		Parameters: []tools.ToolParameter{
			{Name: "name", ParamType: "string", Description: "Name of the person to greet", Required: true},
			{Name: "style", ParamType: "string", Description: "Greeting style: formal, casual, or pirate", Required: true},
		},
	}
}

func (t *GreetTool) Validate(args json.RawMessage) error {
	var params struct {
		Name  string `json:"name"`
		Style string `json:"style"`
	}
	return json.Unmarshal(args, &params)
}

func (t *GreetTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params struct {
		Name  string `json:"name"`
		Style string `json:"style"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.FailureResultf("Invalid arguments: %v", err), nil
	}

	var greeting string
	switch params.Style {
	case "formal":
		greeting = fmt.Sprintf("Good day, %s. It is a pleasure to make your acquaintance.", params.Name)
	case "casual":
		greeting = fmt.Sprintf("Hey %s! What's up?", params.Name)
	case "pirate":
		greeting = fmt.Sprintf("Ahoy, %s! Arrr, welcome aboard, matey!", params.Name)
	default:
		greeting = fmt.Sprintf("Hello, %s!", params.Name)
	}

	return tools.SuccessResult(greeting), nil
}

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

	fmt.Println("=== Custom Tools Example ===")
	fmt.Printf("Using: %s (%s)\n\n", provider.Name(), provider.Model())

	// Create agent with custom tools
	cfg := agent.NewBuilder("calculator").
		Description("An agent that can calculate and greet").
		SystemPrompt(
			"You are a friendly assistant that can do math and greet people. " +
				"Use the calculate tool for arithmetic and the greet tool for greetings.",
		).
		Tool(&CalculateTool{}).
		Tool(&GreetTool{}).
		Build()

	a := agent.New(cfg, provider)

	// Test the custom tools
	tasks := []string{
		"What is 42 multiplied by 7?",
		"Greet Alice in pirate style",
		"Calculate 100 divided by 4, then greet Bob formally",
	}

	for _, task := range tasks {
		fmt.Printf("Task: %s\n", task)

		response := a.Execute(context.Background(), task, 5)

		switch response.Type {
		case agent.ResponseSuccess:
			fmt.Printf("Result: %s\n\n", response.Result)
		case agent.ResponseFailure:
			fmt.Printf("Failed: %s\n\n", response.Error)
		case agent.ResponseTimeout:
			fmt.Println("Timeout")
		}
		fmt.Println("---")
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
