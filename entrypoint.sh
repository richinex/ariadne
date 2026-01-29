#!/bin/bash
set -euo pipefail

# GitHub Actions entrypoint for Ariadne
# Maps INPUT_* environment variables to ariadne CLI commands

# Required inputs
TASK="${INPUT_TASK:-}"
PROVIDER="${INPUT_PROVIDER:-}"
API_KEY="${INPUT_API_KEY:-}"

# Optional inputs with defaults
COMMAND="${INPUT_COMMAND:-react-run}"
MAX_ITERATIONS="${INPUT_MAX_ITERATIONS:-10}"
VERBOSE="${INPUT_VERBOSE:-false}"
DEPTH="${INPUT_DEPTH:-3}"
TIMEOUT="${INPUT_TIMEOUT:-120}"

# Validate required inputs
if [ -z "$TASK" ]; then
    echo "Error: 'task' input is required"
    exit 1
fi

if [ -z "$PROVIDER" ]; then
    echo "Error: 'provider' input is required"
    exit 1
fi

if [ -z "$API_KEY" ]; then
    echo "Error: 'api_key' input is required"
    exit 1
fi

# Map provider to appropriate environment variable
case "$PROVIDER" in
    openai)
        export OPENAI_API_KEY="$API_KEY"
        ;;
    anthropic)
        export ANTHROPIC_API_KEY="$API_KEY"
        ;;
    deepseek)
        export DEEPSEEK_API_KEY="$API_KEY"
        ;;
    gemini)
        export GEMINI_API_KEY="$API_KEY"
        ;;
    *)
        echo "Error: Unknown provider '$PROVIDER'. Supported: openai, anthropic, deepseek, gemini"
        exit 1
        ;;
esac

# Build ariadne command as array (prevents shell injection)
CMD_ARGS=(
    "$COMMAND"
    "--provider" "$PROVIDER"
    "--max-iterations" "$MAX_ITERATIONS"
)

# Add command-specific flags
if [ "$COMMAND" = "rlm" ]; then
    CMD_ARGS+=("--depth" "$DEPTH" "--timeout" "$TIMEOUT")
fi

# Add verbose flag if enabled
if [ "$VERBOSE" = "true" ]; then
    CMD_ARGS+=("--verbose")
fi

# Add the task as the final argument
CMD_ARGS+=("$TASK")

# Output the command for debugging (verbose mode)
if [ "$VERBOSE" = "true" ]; then
    echo "Running: ariadne ${CMD_ARGS[*]}"
fi

# Execute ariadne and capture output (safe from shell injection)
OUTPUT_FILE=$(mktemp -t ariadne-output.XXXXXXXXXX)
EXIT_CODE=0

ariadne "${CMD_ARGS[@]}" | tee "$OUTPUT_FILE" || EXIT_CODE=$?

# Set GitHub Actions outputs (guard against missing GITHUB_OUTPUT in local testing)
if [ -n "${GITHUB_OUTPUT:-}" ]; then
    RESULT=$(cat "$OUTPUT_FILE")
    echo "result<<EOF" >> "$GITHUB_OUTPUT"
    echo "$RESULT" >> "$GITHUB_OUTPUT"
    echo "EOF" >> "$GITHUB_OUTPUT"
    echo "exit-code=$EXIT_CODE" >> "$GITHUB_OUTPUT"
else
    echo "Warning: GITHUB_OUTPUT not set (running locally?)"
fi

# Clean up
rm -f "$OUTPUT_FILE"

exit $EXIT_CODE
