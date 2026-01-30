# Ariadne GitHub Action

Run LLM agents in your CI/CD workflows for automated code analysis, PR reviews, and complex reasoning tasks.

## Quick Start

```yaml
- uses: richinex/ariadne@v1
  with:
    task: "Review this PR for security vulnerabilities"
    provider: openai
    api_key: ${{ secrets.OPENAI_API_KEY }}
```

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `task` | Yes | - | The task for the agent to execute |
| `command` | No | `react-run` | Command to run: `react-run` or `rlm` |
| `provider` | Yes | - | LLM provider: `openai`, `anthropic`, `deepseek`, or `gemini` |
| `api_key` | Yes | - | API key for the LLM provider |
| `max_iter` | No | `25` | Maximum number of reasoning iterations |
| `verbose` | No | `false` | Enable verbose logging |
| `depth` | No | `3` | Search depth for RLM mode |
| `timeout` | No | `120` | Timeout in seconds for RLM mode |
| `subagent_provider` | No | - | LLM provider for sub-agents in RLM mode |
| `subagent_api_key` | No | - | API key for subagent provider (defaults to main api_key) |

## Outputs

| Output | Description |
|--------|-------------|
| `result` | Complete output from the agent |
| `exit-code` | Exit code from execution (0 = success) |

## Provider Setup

Add your API key as a repository secret in Settings → Secrets → Actions.

### OpenAI
```yaml
provider: openai
api_key: ${{ secrets.OPENAI_API_KEY }}
```
Get your API key from [OpenAI Platform](https://platform.openai.com/api-keys)

### Anthropic
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

## Examples

### PR Review

```yaml
name: AI PR Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: AI Code Review
        id: review
        uses: richinex/ariadne@v1
        with:
          task: |
            Review the changes in this PR and provide:
            1. Summary of what changed
            2. Potential bugs or issues
            3. Security concerns
            4. Performance considerations
            5. Suggestions for improvement
          provider: openai
          api_key: ${{ secrets.OPENAI_API_KEY }}
          max_iter: 20

      - name: Comment on PR
        run: |
          gh pr comment ${{ github.event.pull_request.number }} --body-file review.md
        env:
          GH_TOKEN: ${{ github.token }}
```

### Security Scanning

```yaml
name: Security Check

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Scan for security issues
        id: security
        uses: richinex/ariadne@v1
        with:
          task: |
            Perform a security analysis:
            1. Check for SQL injection vulnerabilities
            2. Look for hardcoded secrets or credentials
            3. Identify command injection risks
            4. Review error handling for information leakage
            5. Check for insecure file operations
          command: rlm
          provider: gemini
          api_key: ${{ secrets.GEMINI_API_KEY }}
          max_iter: 25
          depth: 3
          timeout: 180

      - name: Upload report
        uses: actions/upload-artifact@v4
        with:
          name: security-report
          path: security-report.md
```

### Codebase Analysis

```yaml
name: Weekly Analysis

on:
  schedule:
    - cron: '0 0 * * 0'
  workflow_dispatch:

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Analyze codebase
        uses: richinex/ariadne@v1
        with:
          task: "Analyze the storage/ directory and identify potential optimizations"
          command: rlm
          provider: deepseek
          api_key: ${{ secrets.DEEPSEEK_API_KEY }}
          depth: 5
          timeout: 300
          max_iter: 30
```

### Documentation Generation

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

      - name: Generate API docs
        id: docs
        uses: richinex/ariadne@v1
        with:
          task: "Generate markdown documentation for all public functions in the tools/ directory"
          provider: gemini
          api_key: ${{ secrets.GEMINI_API_KEY }}
          max_iter: 15

      - name: Save documentation
        run: |
          echo "${{ steps.docs.outputs.result }}" > docs/API.md
          git config user.name "Bot"
          git config user.email "bot@github.actions"
          git add docs/API.md
          git commit -m "Update API documentation" || exit 0
          git push
```

## Command Modes

### react-run

Single agent execution for focused tasks.

```yaml
with:
  command: react-run
  max_iter: 15
```

### rlm

Recursive sub-agent spawning for complex tasks. Sub-agents can spawn their own sub-agents to handle delegation.

```yaml
with:
  command: rlm
  depth: 5
  timeout: 300
  max_iter: 25
```

Use `subagent_provider` to specify a different provider for sub-agents:

```yaml
with:
  command: rlm
  provider: openai
  subagent_provider: gemini
  api_key: ${{ secrets.OPENAI_API_KEY }}
  subagent_api_key: ${{ secrets.GEMINI_API_KEY }}
  depth: 3
```

## Troubleshooting

### Action fails with missing task

Ensure you specify the `task` input:

```yaml
with:
  task: "Your task description"
```

### API key errors

- Verify the secret is set in repository settings
- Ensure the secret name matches exactly
- Check that your API key is valid and has quota

### Timeout issues

Increase `max_iter` and `timeout` for complex tasks:

```yaml
with:
  max_iter: 30
  timeout: 300
```

### Verbose debugging

Enable verbose logging to see detailed execution:

```yaml
with:
  verbose: true
```

## Security

- Array-based command execution prevents shell injection
- API keys handled via environment variables
- No credentials stored in containers
- Secure temporary file creation

## Limitations

- First run requires Docker image build (approximately 5 minutes)
- Subsequent runs use cached layers and are faster
- LLM API costs apply per execution
- Rate limits depend on your API provider tier

## License

MIT
