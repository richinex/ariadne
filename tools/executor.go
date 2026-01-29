// Tool Executor with Retry Logic.
//
// Information Hiding:
// - Retry strategy implementation hidden
// - Backoff algorithm hidden
// - Error classification logic hidden

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Executor provides tool execution with retry and timeout support.
type Executor struct {
	config ToolConfig
}

// NewExecutor creates a new tool executor with the given configuration.
func NewExecutor(config ToolConfig) *Executor {
	return &Executor{config: config}
}

// NewDefaultExecutor creates an executor with default configuration.
func NewDefaultExecutor() *Executor {
	return &Executor{config: DefaultToolConfig()}
}

// Execute runs a tool with retry logic.
func (e *Executor) Execute(ctx context.Context, tool Tool, args json.RawMessage) (ToolResult, error) {
	var lastErr error
	toolName := tool.Metadata().Name
	maxRetries := e.config.Retries()

	for attempt := uint32(0); attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := e.calculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return ToolResult{}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := tool.Execute(ctx, args)
		if err != nil {
			lastErr = err
			continue
		}

		if result.Success() {
			return result, nil
		}

		// Check if we should retry this failure
		if !e.shouldRetry(result) {
			return result, nil
		}

		lastErr = result.Error
	}

	// All retries exhausted
	errMsg := "unknown error"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	return FailureResultf("tool '%s' failed after %d attempts: %s", toolName, maxRetries, errMsg), nil
}

// calculateBackoff returns the backoff duration for the given attempt.
func (e *Executor) calculateBackoff(attempt uint32) time.Duration {
	const (
		baseDelay = 100 * time.Millisecond
		maxDelay  = 5 * time.Second
	)

	delay := baseDelay * time.Duration(1<<attempt)
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

// shouldRetry determines if an error is retryable.
func (e *Executor) shouldRetry(result ToolResult) bool {
	if result.Error == nil {
		return true
	}

	errLower := strings.ToLower(result.Error.Error())

	// Don't retry validation errors or permission issues
	nonRetryable := []string{"validation", "not allowed", "permission", "empty"}
	for _, s := range nonRetryable {
		if strings.Contains(errLower, s) {
			return false
		}
	}

	// Always retry timeouts and network errors
	retryable := []string{"timeout", "connection", "network"}
	for _, s := range retryable {
		if strings.Contains(errLower, s) {
			return true
		}
	}

	// Default: retry
	return true
}

// ExecuteWithTimeout runs a tool with a specific timeout.
func (e *Executor) ExecuteWithTimeout(ctx context.Context, tool Tool, args json.RawMessage, timeout time.Duration) (ToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return e.Execute(ctx, tool, args)
}

// ExecuteOnce runs a tool once without retries.
func ExecuteOnce(ctx context.Context, tool Tool, args json.RawMessage) (ToolResult, error) {
	// Validate first
	if err := tool.Validate(args); err != nil {
		return FailureResult(fmt.Errorf("validation failed: %w", err)), nil
	}

	return tool.Execute(ctx, args)
}
