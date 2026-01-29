// Package model provides domain types shared across packages.
package model

import "context"

// Step represents a single step in a reasoning process.
// Used by both agents and orchestration for tracking progress.
type Step struct {
	Iteration   int
	Thought     string
	Action      *string
	Observation *string
}

// ToolCall contains metrics about a tool invocation.
// Used for tracking and analytics in both agent and orchestration contexts.
type ToolCall struct {
	Name       string `json:"name"`
	InputSize  int    `json:"input_size"`
	OutputSize int    `json:"output_size"`
	DurationMs uint64 `json:"duration_ms"`
	Success    bool   `json:"success"`
}

// ContentKey uniquely identifies stored content with type safety.
// This prevents misuse by enforcing structure over raw strings.
type ContentKey struct {
	ContentType string // Type of content: "file", "tool", "query", etc.
	Path        string // Unique path or identifier within content type
}

// String returns the canonical string representation.
func (k ContentKey) String() string {
	return k.ContentType + ":" + k.Path
}

// FileKey creates a ContentKey for file content.
func FileKey(path string) ContentKey {
	return ContentKey{ContentType: "file", Path: path}
}

// ContentStore is an interface for storing large content externally.
// This enables the RLM (Recursive Language Model) pattern where large
// outputs are stored as variables instead of being passed in prompts.
// Tools can use this to store content and return references.
type ContentStore interface {
	// StoreContent saves content and returns metadata about where it was stored.
	StoreContent(ctx context.Context, key ContentKey, content string) (StoredContent, error)
}

// StoredContent contains metadata about stored content.
// Content is stored in SQLite and accessed via DSA tools (search_stored, get_lines).
type StoredContent struct {
	Reference string // Content reference in "type:path" format
	Lines     int    // Total number of lines
	Bytes     int    // Total size in bytes
	Preview   string // First few lines for quick viewing
}
