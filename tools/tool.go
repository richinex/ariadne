// Package tools provides the tool system for agents.
//
// Information Hiding:
// - Tool execution details hidden behind interface
// - Tool parameters and schemas hidden in implementations
// - Registry implementation details hidden from consumers
// - Error handling internalized per tool
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// ToolParameter defines a parameter schema for a tool.
type ToolParameter struct {
	Name        string `json:"name"`
	ParamType   string `json:"param_type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolMetadata describes what a tool does and how to use it.
type ToolMetadata struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  []ToolParameter `json:"parameters"`
}

// String returns a string representation of the tool metadata.
func (m ToolMetadata) String() string {
	return fmt.Sprintf("%s: %s", m.Name, m.Description)
}

// ToolResult represents the result of a tool execution.
// Success is determined by whether Error is nil.
type ToolResult struct {
	Output string `json:"output"`
	Error  error  `json:"-"` // Excluded from JSON, use MarshalJSON for custom serialization
}

// MarshalJSON implements custom JSON marshaling for ToolResult.
func (t ToolResult) MarshalJSON() ([]byte, error) {
	if t.Error != nil {
		return json.Marshal(struct {
			Success bool   `json:"success"`
			Output  string `json:"output"`
			Error   string `json:"error"`
		}{
			Success: false,
			Output:  t.Output,
			Error:   t.Error.Error(),
		})
	}
	return json.Marshal(struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
	}{
		Success: true,
		Output:  t.Output,
	})
}

// Success returns true if the tool execution succeeded.
func (t ToolResult) Success() bool {
	return t.Error == nil
}

// SuccessResult creates a successful tool result.
func SuccessResult(output string) ToolResult {
	return ToolResult{Output: output}
}

// FailureResult creates a failed tool result.
func FailureResult(err error) ToolResult {
	return ToolResult{Error: err}
}

// FailureResultf creates a failed tool result with a formatted error message.
func FailureResultf(format string, args ...interface{}) ToolResult {
	return ToolResult{Error: fmt.Errorf(format, args...)}
}

// Tool is the interface that all tools must implement.
//
// Information Hiding: Tool implementations hide their internal execution logic,
// data structures, and error handling strategies behind this interface.
type Tool interface {
	// Metadata returns tool metadata (name, description, parameters).
	Metadata() ToolMetadata

	// Execute runs the tool with given arguments.
	Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)

	// Validate validates arguments before execution (optional).
	Validate(args json.RawMessage) error
}

// BaseTool provides a default implementation for Validate.
type BaseTool struct{}

// Validate provides a default no-op validation.
func (BaseTool) Validate(args json.RawMessage) error {
	return nil
}

// ToolConfig holds tool execution configuration.
// The zero value is safe: timeout defaults to 30s, retries to 3, and sandboxing is enabled.
type ToolConfig struct {
	TimeoutSecs uint64
	MaxRetries  uint32
	NoSandbox   bool // Default false = sandboxed (safe by default)
}

// Timeout returns the configured timeout, defaulting to 30 seconds if zero.
func (c *ToolConfig) Timeout() uint64 {
	if c == nil || c.TimeoutSecs == 0 {
		return 30
	}
	return c.TimeoutSecs
}

// Retries returns the configured max retries, defaulting to 3 if zero.
func (c *ToolConfig) Retries() uint32 {
	if c == nil || c.MaxRetries == 0 {
		return 3
	}
	return c.MaxRetries
}

// Sandboxed returns true if sandboxing is enabled (default).
func (c *ToolConfig) Sandboxed() bool {
	if c == nil {
		return true
	}
	return !c.NoSandbox
}

// DefaultToolConfig returns the default tool configuration.
// Note: The zero value of ToolConfig is also safe and provides the same defaults.
func DefaultToolConfig() ToolConfig {
	return ToolConfig{
		TimeoutSecs: 30,
		MaxRetries:  3,
		NoSandbox:   false,
	}
}

// pathAllowed checks if a path is within the allowed paths.
// If allowedPaths is empty, all paths are allowed.
func pathAllowed(path string, allowedPaths []string) bool {
	if len(allowedPaths) == 0 {
		return true
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, allowed := range allowedPaths {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absPath, allowedAbs) {
			return true
		}
	}
	return false
}

// pathAllowedForWrite checks if a path's parent directory is within allowed paths.
// Used for write operations where the file may not exist yet.
func pathAllowedForWrite(path string, allowedPaths []string) bool {
	if len(allowedPaths) == 0 {
		return true
	}
	parent := filepath.Dir(path)
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	for _, allowed := range allowedPaths {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absParent, allowedAbs) {
			return true
		}
	}
	return false
}
