# Ariadne

A CLI tool for running LLM agents with DSA (Data Structure & Algorithm) powered bounded context. Translated from Reagent (Rust) to idiomatic Go.

Implements two execution patterns:
- **ReAct**: Single agent with DSA tools (Suffix Array, Trie) for bounded context
- **RLM**: Recursive Language Model with dynamic sub-agent spawning (stateless)

## Installation

```bash
go install github.com/richinex/ariadne/cmd/ariadne@latest
```

Or build from source:

```bash
git clone https://github.com/richinex/ariadne.git
cd ariadne
go build -o ariadne ./cmd/ariadne
```

## Configuration

Set your API key as an environment variable:

```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export DEEPSEEK_API_KEY="sk-..."
export GEMINI_API_KEY="..."
```

Or create a `.env` file in your working directory.

## Commands

### react run - Execute a single task with DSA tools

```bash
ariadne --provider openai react run "summarize this file"
ariadne --provider deepseek react run "analyze all Go files in this project"
```

**Flags:**
- `--mcp` - MCP server command (repeatable for multiple servers)
- `--mcp-config` - Path to MCP config file

### react chat - Interactive chat session with DSA tools

```bash
ariadne --provider openai react chat
ariadne --provider anthropic react chat --session my-session
```

**Flags:**
- `--session` - Session ID for persistence
- `--db` - Database path (default: `.ariadne/ariadne.db`)
- `--mcp` - MCP server command (repeatable)
- `--mcp-config` - Path to MCP config file

### react orchestrate - Multi-agent orchestration with DSA tools

```bash
ariadne --provider openai react orchestrate "analyze this codebase" --agent file --agent shell
```

**Flags:**
- `-a, --agent` - Agents to use (can specify multiple)
- `--session` - Session ID for persistence
- `--db` - Database path (default: `.ariadne/ariadne.db`)
- `--mcp` - MCP server command (repeatable)
- `--mcp-config` - Path to MCP config file

### rlm - Recursive Language Model pattern (stateless)

Execute tasks using recursive sub-agent spawning. Sub-agents can spawn their own sub-agents to any depth. Unlike ReAct commands, RLM is stateless with no session persistence.

```bash
ariadne --provider deepseek rlm "analyze all Go files and summarize each"
ariadne --provider openai rlm "list files and explain what each does" --depth 5
ariadne --provider anthropic rlm "complex multi-step task" --verbose
```

**Flags:**
- `--depth` - Maximum recursion depth for sub-agents (default: `3`)
- `--timeout` - Timeout in seconds per sub-agent (default: `120`)
- `--mcp` - MCP server command (repeatable)
- `--mcp-config` - Path to MCP config file

**How RLM works:**
1. Root agent receives task
2. Root can spawn sub-agents via `spawn` or `parallel_spawn` tools
3. Sub-agents execute, return **answers only** (not raw content), then terminate
4. Sub-agents can spawn their own sub-agents (recursive)
5. Root combines answers into final response

**Context efficiency:** Sub-agents return summaries, not raw data. A 10KB file becomes a 500-byte summary.

### tools - List available tools

```bash
ariadne tools
ariadne tools --verbose  # Show parameters
```

## Global Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--provider` | `-p` | LLM provider (`openai`, `anthropic`, `deepseek`, `gemini`) | required |
| `--max-iter` | `-m` | Maximum agent iterations | `10` |
| `--tool-retries` | | Maximum tool retry attempts | `3` |
| `--verbose` | `-v` | Show detailed output including reasoning steps | `false` |

## Available Tools

### File Operations
| Tool | Description |
|------|-------------|
| `read_file` | Read AND STORE file - returns metadata/summary, NOT full content |
| `write_file` | Write content to file |
| `edit_file` | Edit file with search/replace |
| `append_file` | Append content to file |
| `glob` | Find files by pattern (e.g., `**/*.go`) - returns paths only |

### DSA-Powered Search (requires files stored via read_file first)
| Tool | Description |
|------|-------------|
| `search_stored` | Search pattern across ALL stored content (O(m log n) SuffixArray) |
| `get_lines` | Get specific line range from stored content |
| `list_stored` | List stored content with prefix filter (O(k) Trie lookup) |

### Command & Web
| Tool | Description |
|------|-------------|
| `execute_shell` | Run shell commands |
| `http_request` | Make HTTP requests |
| `ripgrep` | Search files with ripgrep (ReAct only, fallback) |

### RLM-Specific
| Tool | Description |
|------|-------------|
| `spawn` | Spawn a sub-agent for a specific task |
| `parallel_spawn` | Spawn multiple sub-agents concurrently |

## Architecture

```
ariadne/
├── agent/          # ReAct agent implementation
├── cli/            # Command-line interface (ReAct and RLM)
├── cmd/ariadne/    # Main entry point (Cobra CLI)
├── config/         # Configuration management
├── internal/dsa/   # Data structures (Radix tree, SuffixArray)
├── llm/            # LLM provider implementations (OpenAI, Anthropic, DeepSeek, Gemini)
├── mcp/            # Model Context Protocol client for dynamic tools
├── model/          # Shared domain types
├── orchestration/  # Multi-agent orchestration (Supervisor pattern)
├── storage/        # Unified SQLite storage (conversations, content, results)
└── tools/          # Tool implementations (file, shell, HTTP, DSA search, spawn)
```

## DSA-Powered Storage

Ariadne uses advanced data structures to provide **bounded context** - preventing token overflow when working with large files.

**Storage Architecture:**
- **Radix Tree** (go-radix): O(k) prefix lookup for file paths
- **Suffix Array** (custom implementation): O(m log n) pattern search across ALL stored content
- **SQLite**: Unified database for conversations, metadata, and content persistence
- **Content-addressable storage**: Fast deduplication using xxhash

**How it works:**
1. `read_file` stores content externally and returns only metadata/summary
2. `search_stored` searches across ALL stored files using SuffixArray (grep-like, but faster)
3. `get_lines` retrieves specific line ranges without loading full files
4. `list_stored` lists stored content using Trie prefix search

This enables analyzing dozens of files without bloating LLM context.

## Examples

```bash
# Simple file analysis with ReAct
ariadne -p openai react run "what does main.go do?"

# Multi-file analysis with DSA tools
ariadne -p deepseek react run "analyze all Go files in the project" --verbose

# Multi-file analysis with RLM (recursive spawning)
ariadne -p deepseek rlm "analyze the tools/ directory and summarize each file" --verbose

# Interactive coding session with persistence
ariadne -p anthropic react chat --session coding

# Complex orchestration across multiple agents
ariadne -p openai react orchestrate "refactor this module" -a file -a shell

# With MCP servers (Model Context Protocol)
ariadne -p openai react run "task" --mcp "npx -y @modelcontextprotocol/server-filesystem ."
```

## ReAct vs RLM: Which to Use?

Both patterns use **DSA-powered tools** (Suffix Array, Trie) for bounded context.

| Feature | ReAct | RLM |
|---------|-------|-----|
| **Pattern** | Single agent loop | Recursive sub-agent spawning |
| **Session persistence** | Yes (with `--session`) | No (stateless) |
| **Best for** | Interactive tasks, iterative work | Complex multi-step delegation |
| **DSA Tools** | ✅ All DSA tools | ✅ All DSA tools |
| **Ripgrep** | ✅ Available as fallback | ❌ Excluded (use glob + DSA) |
| **Spawn tools** | ❌ Not available | ✅ spawn, parallel_spawn |
| **Orchestration** | ✅ Multi-agent (Supervisor) | ✅ Built-in (via spawn) |
| **Context management** | DSA search + store | DSA search + sub-agent answers |

**Use ReAct when:**
- You need interactive chat sessions with conversation history
- Iterative file analysis and modification
- You want orchestration across predefined agents
- Ripgrep-style search is needed

**Use RLM when:**
- Task requires dynamic delegation to sub-agents
- You want parallel sub-task execution (parallel_spawn)
- Complex workflows with recursive decomposition
- Pure analysis tasks without session state

## Providers

| Provider | Models | Environment Variable |
|----------|--------|---------------------|
| OpenAI | gpt-4o, gpt-4-turbo, gpt-3.5-turbo | `OPENAI_API_KEY` |
| Anthropic | claude-3-opus, claude-3-sonnet, claude-3-haiku | `ANTHROPIC_API_KEY` |
| DeepSeek | deepseek-chat, deepseek-reasoner | `DEEPSEEK_API_KEY` |
| Gemini | gemini-pro, gemini-1.5-pro | `GEMINI_API_KEY` |

## MCP Server Support

Ariadne supports [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) servers for dynamic tool discovery:

```bash
# Single MCP server
ariadne -p openai react run "task" --mcp "npx -y @modelcontextprotocol/server-filesystem ."

# Multiple MCP servers
ariadne -p openai react run "task" \
  --mcp "npx -y @modelcontextprotocol/server-filesystem ." \
  --mcp "npx -y @modelcontextprotocol/server-memory"

# Or use MCP config file (Anthropic format)
ariadne -p openai react run "task" --mcp-config ~/.config/claude/mcp.json
```

**Tested MCP servers:**
- `@modelcontextprotocol/server-filesystem` (14 tools)
- `@modelcontextprotocol/server-memory` (9 tools)

## License

MIT
# ariadne
