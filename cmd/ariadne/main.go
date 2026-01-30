// Package main provides the ariadne CLI entry point.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/richinex/ariadne/cli"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	provider    string
	maxIter     int
	toolRetries uint32
	verbose     bool
)

func main() {
	// Load .env file if present (ignore "file not found" errors)
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: failed to load .env file: %v\n", err)
		}
	}

	rootCmd := &cobra.Command{
		Use:   "ariadne",
		Short: "LLM agents with DSA-powered bounded context",
		Long: `A CLI tool for running LLM agents with DSA (Data Structure & Algorithm) powered tools.

Two patterns available:
- react: Single agent with DSA tools (Suffix Array, Trie) for bounded context
- rlm: Recursive Language Model with sub-agent spawning (stateless)`,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&provider, "provider", "p", "", "LLM provider (openai, anthropic, deepseek, gemini)")
	rootCmd.PersistentFlags().IntVarP(&maxIter, "max-iter", "m", 10, "Maximum iterations for agent execution")
	rootCmd.PersistentFlags().Uint32Var(&toolRetries, "tool-retries", 3, "Maximum retries for tool execution")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show verbose output")

	// Add commands
	rootCmd.AddCommand(reactRunCmd())
	rootCmd.AddCommand(reactChatCmd())
	rootCmd.AddCommand(reactOrchestrateCmd())
	rootCmd.AddCommand(rlmCmd())
	rootCmd.AddCommand(toolsCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func reactRunCmd() *cobra.Command {
	var mcpServers []string
	var mcpConfigPath string

	cmd := &cobra.Command{
		Use:   "react-run [task]",
		Short: "Execute a task with a single agent + DSA tools",
		Long: `Execute tasks using the ReAct (Reasoning + Acting) pattern with DSA tools.

DSA tools provide bounded context via:
- Suffix Array: O(m log n) pattern search across all stored files
- Radix Trie: O(m+k) prefix lookups
- SQLite: Content persistence across sessions`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := cli.Options{
				Provider:    provider,
				MaxIter:     maxIter,
				ToolRetries: toolRetries,
				Verbose:     verbose,
			}
			return cli.ReAct(context.Background(), args[0], mcpServers, mcpConfigPath, opts)
		},
	}

	cmd.Flags().StringArrayVar(&mcpServers, "mcp", nil, "MCP server command (repeatable)")
	cmd.Flags().StringVar(&mcpConfigPath, "mcp-config", "", "Path to MCP config file")

	return cmd
}

func reactChatCmd() *cobra.Command {
	var sessionID string
	var dbPath string
	var mcpServers []string
	var mcpConfigPath string

	cmd := &cobra.Command{
		Use:   "react-chat",
		Short: "Start an interactive chat session with DSA tools",
		Long: `Start an interactive chat session using the ReAct pattern with DSA tools.

DSA tools provide bounded context via:
- Suffix Array: O(m log n) pattern search across all stored files
- Radix Trie: O(m+k) prefix lookups
- SQLite: Content persistence across sessions`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := cli.Options{
				Provider:    provider,
				MaxIter:     maxIter,
				ToolRetries: toolRetries,
				Verbose:     verbose,
			}
			return cli.ReactChat(context.Background(), sessionID, dbPath, mcpServers, mcpConfigPath, opts)
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID for conversation persistence")
	cmd.Flags().StringVar(&dbPath, "db", ".ariadne/ariadne.db", "Database path for storage")
	cmd.Flags().StringArrayVar(&mcpServers, "mcp", nil, "MCP server command (repeatable)")
	cmd.Flags().StringVar(&mcpConfigPath, "mcp-config", "", "Path to MCP config file")

	return cmd
}

func reactOrchestrateCmd() *cobra.Command {
	var agentNames []string
	var sessionID string
	var dbPath string
	var mcpServers []string
	var mcpConfigPath string

	cmd := &cobra.Command{
		Use:   "react-orchestrate [task]",
		Short: "Orchestrate a task across multiple agents with DSA tools",
		Long: `Orchestrate a task across multiple agents using the ReAct pattern with DSA tools.

DSA tools provide bounded context via:
- Suffix Array: O(m log n) pattern search across all stored files
- Radix Trie: O(m+k) prefix lookups
- SQLite: Content persistence across sessions`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := cli.Options{
				Provider:    provider,
				MaxIter:     maxIter,
				ToolRetries: toolRetries,
				Verbose:     verbose,
			}
			return cli.ReactOrchestrate(context.Background(), args[0], agentNames, sessionID, dbPath, mcpServers, mcpConfigPath, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&agentNames, "agent", "a", nil, "Agent(s) to use (can specify multiple)")
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID for memory persistence")
	cmd.Flags().StringVar(&dbPath, "db", ".ariadne/ariadne.db", "Database path for storage")
	cmd.Flags().StringArrayVar(&mcpServers, "mcp", nil, "MCP server command (repeatable)")
	cmd.Flags().StringVar(&mcpConfigPath, "mcp-config", "", "Path to MCP config file")

	return cmd
}

func rlmCmd() *cobra.Command {
	var maxDepth int
	var timeout int
	var mcpServers []string
	var mcpConfigPath string
	var subagentProvider string

	cmd := &cobra.Command{
		Use:   "rlm [task]",
		Short: "Execute a task using RLM pattern (recursive sub-agent spawning, stateless)",
		Long: `Execute a task using the Recursive Language Model (RLM) pattern.

The RLM pattern enables:
- Dynamic sub-agent spawning (no fixed agent list)
- Recursive spawning (sub-agents can spawn their own sub-agents)
- Parallel task execution
- Context efficiency (children return answers, not raw content)
- STATELESS: No session persistence

Cost optimization:
- Use --subagent-provider to specify a cheaper model for sub-agents
- Example: --provider openai --subagent-provider deepseek (root uses gpt-5.2, children use deepseek-chat)

MCP servers can be added with --mcp flag (repeatable) or --mcp-config file.

Based on Alex Zhang's RLM architecture.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := cli.Options{
				Provider:         provider,
				SubagentProvider: subagentProvider,
				MaxIter:          maxIter,
				ToolRetries:      toolRetries,
				Verbose:          verbose,
			}
			return cli.RLM(context.Background(), args[0], maxDepth, timeout, mcpServers, mcpConfigPath, opts)
		},
	}

	cmd.Flags().IntVar(&maxDepth, "depth", 3, "Maximum recursion depth for sub-agents")
	cmd.Flags().IntVar(&timeout, "timeout", 120, "Timeout in seconds per sub-agent")
	cmd.Flags().StringVar(&subagentProvider, "subagent-provider", "", "LLM provider for sub-agents (cost optimization): openai, anthropic, deepseek, gemini")
	cmd.Flags().StringArrayVar(&mcpServers, "mcp", nil, "MCP server command (repeatable)")
	cmd.Flags().StringVar(&mcpConfigPath, "mcp-config", "", "Path to MCP config file")

	return cmd
}

func toolsCmd() *cobra.Command {
	var verboseTools bool

	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List available tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.ListTools(verboseTools)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&verboseTools, "verbose", "V", false, "Show tool parameters")

	return cmd
}
