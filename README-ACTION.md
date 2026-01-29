# Ariadne GitHub Action

> Like Ariadne's thread guiding Theseus through the labyrinth, this action helps you navigate massive codebases with LLM-powered agents.

Run intelligent AI agents in your CI/CD workflows for automated code analysis, PR reviews, and complex reasoning tasks across large contexts.

## Features

- **Navigate Complex Codebases**: RLM (Reasoning with Large Models) mode explores deep context like a thread through a maze
- **Multiple LLM Providers**: OpenAI, Anthropic, DeepSeek, and Gemini
- **Two Agent Modes**:
  - `react-run`: Single-agent ReAct loop for focused tasks
  - `rlm`: Reasoning agent with tree search for navigating massive contexts
- **Flexible Configuration**: Control iterations, depth, timeouts, and verbosity
- **GitHub Actions Native**: Capture outputs, set exit codes, and integrate with workflows

## Quick Start

```yaml
- name: Navigate codebase with AI
  uses: richinex/ariadne-action@v1
  with:
    task: "Review this PR for security vulnerabilities and hardcoded secrets"
    provider: openai
    api_key: ${{ secrets.OPENAI_API_KEY }}
```

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `task` | Yes | - | The task or prompt for the agent to execute |
| `command` | No | `react-run` | Command to run: `react-run` or `rlm` |
| `provider` | Yes | - | LLM provider: `openai`, `anthropic`, `deepseek`, or `gemini` |
| `api_key` | Yes | - | API key for the LLM provider (use secrets!) |
| `max-iterations` | No | `10` | Maximum number of reasoning iterations |
| `verbose` | No | `false` | Enable verbose logging |
| `depth` | No | `3` | Search depth for RLM mode (rlm only) |
| `timeout` | No | `120` | Timeout in seconds for RLM mode (rlm only) |

## Outputs

| Output | Description |
|--------|-------------|
| `result` | Complete output from the Ariadne agent |
| `exit-code` | Exit code from execution (0 = success, non-zero = failure) |

## Provider Setup

### OpenAI
```yaml
provider: openai
api_key: ${{ secrets.OPENAI_API_KEY }}
```
Get your API key from [OpenAI Platform](https://platform.openai.com/api-keys)

### Anthropic (Claude)
```yaml
provider: anthropic
api_key: ${{ secrets.ANTHROPIC_API_KEY }}
```
Get your API key from [Anthropic Console](https://console.anthropic.com/)

### DeepSeek
```yaml
provider: deepseek
api_key: ${{ secrets.DEEPSEEK_API_KEY }}
```
Get your API key from [DeepSeek Platform](https://platform.deepseek.com/)

### Gemini
```yaml
provider: gemini
api_key: ${{ secrets.GEMINI_API_KEY }}
```
Get your API key from [Google AI Studio](https://aistudio.google.com/app/apikey)

## Usage Examples

### 1. Automated PR Review - Navigate Changes

```yaml
name: AI PR Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Navigate PR changes with Ariadne
        id: review
        uses: richinex/ariadne-action@v1
        with:
          task: |
            Review this pull request for:
            - Security vulnerabilities
            - Performance issues
            - Code quality and best practices
            - Potential bugs
          provider: anthropic
          api_key: ${{ secrets.ANTHROPIC_API_KEY }}
          max-iterations: 15
          verbose: true

      - name: Post review comment
        uses: actions/github-script@v7
        with:
          script: |
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: '## 🧵 Ariadne Code Review\n\n${{ steps.review.outputs.result }}'
            })
```

### 2. Deep Context Navigation with RLM

Use RLM mode to navigate through massive codebases like following a thread through a labyrinth:

```yaml
name: Weekly Deep Analysis

on:
  schedule:
    - cron: '0 0 * * 0'  # Every Sunday
  workflow_dispatch:

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Navigate codebase with RLM
        uses: richinex/ariadne-action@v1
        with:
          task: "Analyze all Go files in the storage/ directory and identify potential optimizations"
          command: rlm
          provider: deepseek
          api_key: ${{ secrets.DEEPSEEK_API_KEY }}
          depth: 5
          timeout: 300
          max-iterations: 20
```

### 3. Security Scanning Quality Gate

```yaml
name: Security Gate

on:
  push:
    branches: [ main, develop ]

jobs:
  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Security scan with Ariadne
        id: security
        uses: richinex/ariadne-action@v1
        with:
          task: "Scan for security vulnerabilities, hardcoded secrets, and SQL injection risks"
          provider: openai
          api_key: ${{ secrets.OPENAI_API_KEY }}
          max-iterations: 10

      - name: Check for critical issues
        run: |
          if echo "${{ steps.security.outputs.result }}" | grep -iq "CRITICAL"; then
            echo "Critical security issues found!"
            exit 1
          fi
```

### 4. Documentation Generation

```yaml
name: Auto-Generate Docs

on:
  push:
    paths:
      - 'src/**/*.go'

jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate API docs with Ariadne
        id: docs
        uses: richinex/ariadne-action@v1
        with:
          task: "Generate markdown documentation for all public functions in the tools/ directory"
          provider: gemini
          api_key: ${{ secrets.GEMINI_API_KEY }}
          max-iterations: 12

      - name: Save documentation
        run: |
          echo "${{ steps.docs.outputs.result }}" > docs/API.md
          git config user.name "Ariadne Bot"
          git config user.email "ariadne@github.actions"
          git add docs/API.md
          git commit -m "docs: Update API documentation" || exit 0
          git push
```

### 5. Test Coverage Analysis

```yaml
name: Test Coverage Navigation

on:
  pull_request:
    paths:
      - '**/*_test.go'

jobs:
  coverage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Navigate test coverage with Ariadne
        uses: richinex/ariadne-action@v1
        with:
          task: "Review test files and identify untested edge cases or missing test scenarios"
          provider: anthropic
          api_key: ${{ secrets.ANTHROPIC_API_KEY }}
          max-iterations: 15
          verbose: true
```

## Understanding RLM Mode

The `rlm` command mode uses tree search to navigate large contexts systematically:

- **Like a Thread Through a Maze**: RLM explores different paths through your codebase
- **Depth Control**: Set how deep to explore each path
- **Timeout Management**: Balance thoroughness with time constraints
- **Best for**: Large refactorings, architectural analysis, cross-file reasoning

```yaml
with:
  command: rlm
  depth: 5        # Explore 5 levels deep
  timeout: 300    # 5 minutes max
```

## Troubleshooting

### Action fails with "Error: 'task' input is required"
Ensure you've specified the `task` input:
```yaml
with:
  task: "Your task description here"
```

### API key errors
- Verify your secret is set in repository settings (Settings → Secrets → Actions)
- Ensure the secret name matches exactly (e.g., `OPENAI_API_KEY`)
- Check that your API key is valid and has available quota

### Timeout issues
For complex navigation tasks, increase `max-iterations` and `timeout`:
```yaml
with:
  max-iterations: 20
  timeout: 300  # 5 minutes (rlm only)
```

### Docker build takes too long
The first run builds the Docker image (~5 minutes). Subsequent runs use cached layers and are faster. For faster startup, upgrade to v2.0 which uses pre-built images (~30 seconds).

### Provider-specific issues
- **OpenAI**: Ensure you're using a valid model (default: gpt-4)
- **Anthropic**: Claude API requires credits
- **DeepSeek**: Check regional availability
- **Gemini**: Requires Google Cloud project setup

### Verbose debugging
Enable verbose logging to see detailed execution:
```yaml
with:
  verbose: true
```

## Architecture

Ariadne uses a multi-stage Docker build:
1. **Builder stage**: Compiles Go binary with CGO enabled (for SQLite)
2. **Runtime stage**: Minimal Alpine image (~50MB) with only required libraries
3. **Entrypoint**: Secure bash wrapper that maps GitHub Actions inputs to CLI flags

### Security Features
- Array-based command execution (prevents shell injection)
- Secure temporary file creation
- API key handling via environment variables
- No credentials stored in containers

## Limitations

- First run requires Docker image build (~5 minutes)
- SQLite requires CGO (slightly larger image size)
- LLM API costs apply per execution
- Rate limits depend on your API provider tier

## Metaphor: Ariadne's Thread

In Greek mythology, Ariadne gave Theseus a ball of thread to navigate the labyrinth and escape after defeating the Minotaur. Similarly:

- **Your Codebase** = The Labyrinth (complex, interconnected, easy to get lost)
- **Ariadne's Thread** = RLM reasoning traces (systematic navigation, finding your way)
- **The Minotaur** = Bugs, tech debt, security issues (the monsters hiding in complexity)
- **Escape** = Understanding, fixes, improvements (successfully navigating to the exit)

RLM mode literally creates a "thread" of reasoning through your codebase, helping you navigate massive contexts that would be overwhelming to explore manually.

## Roadmap

- **v2.0**: Pre-built Docker images on GHCR (faster startup)
- **v3.0**: Multi-agent orchestration support (multiple threads through the maze)
- **v4.0**: OpenTelemetry instrumentation for observability

## Contributing

Issues and pull requests welcome at [github.com/richinex/ariadne](https://github.com/richinex/ariadne)

## License

See [LICENSE](LICENSE) in the main repository.

---

*"With Ariadne's thread, even the most complex labyrinth becomes navigable."*
