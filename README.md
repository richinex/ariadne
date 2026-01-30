# Ariadne (Experimental)

Ariadne is a CLI tool and GitHub Action for running LLM agents with bounded context.
It provides two execution patterns: ReAct for single-agent tasks and RLM for recursive sub-agent spawning.
It is an experimental project exploring ways to incorporate LLMs into a Github actions workflow.

## Quick Start

```bash
# Install
go install github.com/richinex/ariadne/cmd/ariadne@latest

# Configure API key
export OPENAI_API_KEY="sk-..."

# Run a task
ariadne --provider openai react-run "analyze this codebase"
```

## Installation

### From Binary

```bash
go install github.com/richinex/ariadne/cmd/ariadne@latest
```

### From Source

```bash
git clone https://github.com/richinex/ariadne.git
cd ariadne
go build -o ariadne ./cmd/ariadne
```

### GitHub Action

```yaml
- uses: richinex/ariadne@v1
  with:
    task: "Review this PR for security issues"
    provider: openai
    api_key: ${{ secrets.OPENAI_API_KEY }}
```

## Features

- Single-agent ReAct pattern for focused tasks
- Recursive sub-agent spawning with RLM pattern
- Bounded context using Suffix Array and Trie data structures
- Multiple LLM providers: OpenAI, Anthropic, DeepSeek, Gemini
- Model Context Protocol (MCP) server support
- Interactive chat sessions with persistence
- Multi-agent orchestration
- GitHub Actions integration

## Configuration

Set your API key as an environment variable:

```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export DEEPSEEK_API_KEY="sk-..."
export GEMINI_API_KEY="..."
```

Or create a `.env` file in your working directory.

## Usage

### react-run

Execute a single task with a single agent.

```bash
ariadne --provider openai react-run "summarize this file"
ariadne --provider deepseek react-run "analyze all Go files"
```

### react-chat

Start an interactive chat session with conversation persistence.

```bash
ariadne --provider anthropic react-chat
ariadne --provider openai react-chat --session my-session
```

### react-orchestrate

Run multi-agent orchestration with specialized agents.

```bash
ariadne --provider openai react-orchestrate "analyze this codebase" --agent file --agent shell
```

### rlm

Execute tasks using recursive sub-agent spawning. Sub-agents can spawn their own sub-agents to handle complex tasks through delegation.

```bash
ariadne --provider deepseek rlm "analyze all Go files and summarize each"
ariadne --provider openai rlm "list files and explain what each does" --depth 5
```

## Available Tools

### File Operations
- `read_file` - Read and store file content
- `write_file` - Write content to file
- `edit_file` - Edit file with search and replace
- `append_file` - Append content to file
- `glob` - Find files by pattern

### DSA Search
- `search_stored` - Search pattern across stored content using Suffix Array
- `get_lines` - Get specific line range from stored content
- `list_stored` - List stored content using Trie prefix search

### Command and Web
- `execute_shell` - Run shell commands
- `http_request` - Make HTTP requests
- `ripgrep` - Search files with ripgrep

### RLM Tools
- `spawn` - Spawn a sub-agent for a task
- `parallel_spawn` - Spawn multiple sub-agents concurrently

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--provider` | LLM provider (openai, anthropic, deepseek, gemini) | required |
| `--max-iter` | Maximum agent iterations | 10 |
| `--verbose` | Show detailed output | false |

## Examples

```bash
# Simple file analysis
ariadne -p openai react-run "what does main.go do?"

# Multi-file analysis with verbose output
ariadne -p deepseek react-run "analyze all Go files" --verbose

# Recursive analysis with sub-agents
ariadne -p deepseek rlm "analyze the tools/ directory" --depth 3

# Interactive session with persistence
ariadne -p anthropic react-chat --session coding

# With MCP server
ariadne -p openai react-run "task" --mcp "npx -y @modelcontextprotocol/server-filesystem ."
```

## Architecture

Ariadne uses data structures to provide bounded context and prevent token overflow:

- **Suffix Array**: O(m log n) pattern search across stored files
- **Radix Tree**: O(k) prefix lookup for file paths
- **SQLite**: Unified storage for conversations and content
- **Content-addressable storage**: Deduplication using xxhash

When you read a file with `read_file`, the content is stored externally and only metadata is returned to the agent. Search operations use `search_stored` to query across all stored files without loading them into context.

## ReAct vs RLM

| Feature | ReAct | RLM |
|---------|-------|-----|
| Pattern | Single agent loop | Recursive sub-agents |
| Session persistence | Yes | No |
| Best for | Interactive tasks | Complex delegation |
| Spawn tools | No | Yes |

Use ReAct for interactive sessions and iterative work. Use RLM for complex tasks that benefit from recursive decomposition and parallel execution.

## Providers

| Provider | Environment Variable |
|----------|---------------------|
| OpenAI | `OPENAI_API_KEY` |
| Anthropic | `ANTHROPIC_API_KEY` |
| DeepSeek | `DEEPSEEK_API_KEY` |
| Gemini | `GEMINI_API_KEY` |

## MCP Support

Ariadne supports Model Context Protocol servers for dynamic tool discovery:

```bash
# Single MCP server
ariadne -p openai react-run "task" --mcp "npx -y @modelcontextprotocol/server-filesystem ."

# Multiple MCP servers
ariadne -p openai react-run "task" \
  --mcp "npx -y @modelcontextprotocol/server-filesystem ." \
  --mcp "npx -y @modelcontextprotocol/server-memory"

# MCP config file
ariadne -p openai react-run "task" --mcp-config ~/.config/claude/mcp.json
```

## Documentation

- [GitHub Action Documentation](README-ACTION.md)

## License

MIT

