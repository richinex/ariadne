// Glob tool for file discovery.
//
// Returns file paths matching a glob pattern without reading content.
// Designed for the RLM pattern where discovery and content loading are separate.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// DefaultGlobMaxResults is the default maximum results per query.
	DefaultGlobMaxResults = 100
	// AbsoluteGlobMaxResults is the hard limit to prevent excessive memory.
	AbsoluteGlobMaxResults = 1000
)

// GlobTool finds files matching glob patterns.
type GlobTool struct {
	maxResults int
}

// NewGlobTool creates a new glob tool.
// If maxResults <= 0, AbsoluteGlobMaxResults is used.
func NewGlobTool(maxResults int) *GlobTool {
	if maxResults <= 0 {
		maxResults = AbsoluteGlobMaxResults
	}
	return &GlobTool{maxResults: maxResults}
}

// Metadata returns tool metadata.
func (t *GlobTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Returns file paths only (no content). Hidden directories (starting with .) are skipped. Use for discovery, then read_file to load content.",
		Parameters: []ToolParameter{
			{Name: "pattern", ParamType: "string", Description: "Glob pattern (e.g., '**/*.go', 'src/**/*.ts', '*.yaml')", Required: true},
			{Name: "path", ParamType: "string", Description: "Base directory to search from (default: current directory)", Required: false},
			{Name: "max_results", ParamType: "integer", Description: fmt.Sprintf("Maximum files to return (default: %d)", DefaultGlobMaxResults), Required: false},
		},
	}
}

// GlobArgs are the arguments for the glob tool.
type GlobArgs struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	MaxResults *int   `json:"max_results"`
}

// Validate validates the arguments.
func (t *GlobTool) Validate(args json.RawMessage) error {
	var globArgs GlobArgs
	if err := json.Unmarshal(args, &globArgs); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(globArgs.Pattern) == "" {
		return fmt.Errorf("pattern is required")
	}
	return nil
}

// Execute runs the glob search.
// Errors are returned via ToolResult to allow partial results and user-friendly messages.
func (t *GlobTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var globArgs GlobArgs
	if err := json.Unmarshal(args, &globArgs); err != nil {
		return FailureResultf("invalid arguments: %v", err), nil
	}

	basePath := globArgs.Path
	if basePath == "" {
		basePath = "."
	}

	maxResults := DefaultGlobMaxResults
	if globArgs.MaxResults != nil && *globArgs.MaxResults > 0 {
		maxResults = *globArgs.MaxResults
	}
	if maxResults > t.maxResults {
		maxResults = t.maxResults
	}

	matches, err := t.findMatches(ctx, basePath, globArgs.Pattern, maxResults)
	if err != nil {
		return FailureResultf("%v", err), nil
	}

	return t.formatResult(globArgs.Pattern, basePath, matches, maxResults), nil
}

// findMatches finds files matching the pattern in basePath.
func (t *GlobTool) findMatches(ctx context.Context, basePath, pattern string, maxResults int) ([]string, error) {
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("invalid base path: %w", err)
	}

	dirInfo, err := os.Stat(absBase)
	if err != nil {
		return nil, fmt.Errorf("path not found: %s", basePath)
	}
	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", basePath)
	}

	// Normalize pattern: strip leading "./" as it's redundant
	pattern = strings.TrimPrefix(pattern, "./")

	if strings.Contains(pattern, "**") {
		return t.findMatchesRecursive(ctx, absBase, pattern, maxResults)
	}
	return t.findMatchesSimple(absBase, pattern, maxResults)
}

// findMatchesRecursive handles patterns with ** using WalkDir.
func (t *GlobTool) findMatchesRecursive(ctx context.Context, absBase, pattern string, maxResults int) ([]string, error) {
	var matches []string

	err := filepath.WalkDir(absBase, func(path string, entry os.DirEntry, err error) error {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			if os.IsPermission(err) {
				return filepath.SkipDir
			}
			// Skip other errors (I/O issues, etc.)
			return nil
		}

		if entry.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(entry.Name(), ".") && entry.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(absBase, path)
		if err != nil {
			return nil
		}

		if matchGlobPattern(relPath, pattern) {
			matches = append(matches, relPath)
			if len(matches) >= maxResults {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return matches, err
	}

	sort.Strings(matches)
	return matches, nil
}

// findMatchesSimple handles patterns without ** using filepath.Glob.
func (t *GlobTool) findMatchesSimple(absBase, pattern string, maxResults int) ([]string, error) {
	fullPattern := filepath.Join(absBase, pattern)
	globMatches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern: %w", err)
	}

	var matches []string
	for _, m := range globMatches {
		fileInfo, err := os.Stat(m)
		if err != nil || fileInfo.IsDir() {
			continue
		}
		relPath, err := filepath.Rel(absBase, m)
		if err != nil {
			continue
		}
		matches = append(matches, relPath)
		if len(matches) >= maxResults {
			break
		}
	}

	sort.Strings(matches)
	return matches, nil
}

// formatResult formats the matches into a ToolResult.
func (t *GlobTool) formatResult(pattern, basePath string, matches []string, maxResults int) ToolResult {
	if len(matches) == 0 {
		return SuccessResult(fmt.Sprintf("No files found matching pattern '%s' in %s", pattern, basePath))
	}

	var result strings.Builder
	fmt.Fprintf(&result, "Found %d files matching '%s':\n", len(matches), pattern)
	for _, m := range matches {
		fmt.Fprintln(&result, m)
	}

	if len(matches) >= maxResults {
		fmt.Fprintf(&result, "\n(limited to %d results)", maxResults)
	}

	return SuccessResult(result.String())
}

// matchGlobPattern matches a path against a glob pattern with ** support.
func matchGlobPattern(path, pattern string) bool {
	// Normalize separators
	path = filepath.ToSlash(path)
	pattern = filepath.ToSlash(pattern)

	parts := strings.Split(pattern, "**")

	if len(parts) == 1 {
		// No **, use simple match
		return matchPattern(pattern, path)
	}

	// Handle ** patterns
	// **/*.go means any directory depth followed by .go files
	// src/**/*.go means src/ then any depth then .go files

	// Check prefix (before first **)
	prefix := strings.TrimSuffix(parts[0], "/")
	if prefix != "" && !strings.HasPrefix(path, prefix) {
		return false
	}

	// Check suffix (after last **)
	suffix := strings.TrimPrefix(parts[len(parts)-1], "/")
	if suffix != "" {
		if strings.Contains(suffix, "/") {
			// Suffix has directory components
			if !strings.HasSuffix(path, suffix) {
				if !matchPattern("*/"+suffix, "/"+path) {
					return false
				}
			}
		} else {
			// Suffix is just a filename pattern
			if !matchPattern(suffix, filepath.Base(path)) {
				return false
			}
		}
	}

	return true
}

// matchPattern wraps filepath.Match, returning false on error.
func matchPattern(pattern, name string) bool {
	matched, err := filepath.Match(pattern, name)
	return err == nil && matched
}
