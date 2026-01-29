// ResultStore Tools - Expose ResultStore operations to agents.
//
// These tools let agents search, retrieve, and explore stored content
// using the built-in DSA (Trie, SuffixArray) instead of external tools.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/richinex/davingo/storage"
)

// StoredFileContext tracks recently stored files for easy reference.
// This enables the RLM pattern where agents don't need to re-specify keys.
type StoredFileContext struct {
	mu    sync.RWMutex
	files []string // Most recent first
}

// NewStoredFileContext creates a new context tracker.
func NewStoredFileContext() *StoredFileContext {
	return &StoredFileContext{
		files: make([]string, 0),
	}
}

// Add tracks a newly stored file.
func (c *StoredFileContext) Add(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Add to front (most recent first)
	c.files = append([]string{key}, c.files...)
	// Keep only last 10
	if len(c.files) > 10 {
		c.files = c.files[:10]
	}
}

// Last returns the most recently stored file key, or empty if none.
func (c *StoredFileContext) Last() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.files) == 0 {
		return ""
	}
	return c.files[0]
}

// List returns all tracked files (most recent first).
func (c *StoredFileContext) List() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, len(c.files))
	copy(result, c.files)
	return result
}

// SearchStoredTool searches across ALL stored content using SuffixArray.
type SearchStoredTool struct {
	BaseTool
	store       *storage.ResultStore
	sessionID   string
	fileContext *StoredFileContext
}

// NewSearchStoredTool creates a tool for searching stored content.
func NewSearchStoredTool(store *storage.ResultStore, sessionID string, fileContext *StoredFileContext) *SearchStoredTool {
	return &SearchStoredTool{
		store:       store,
		sessionID:   sessionID,
		fileContext: fileContext,
	}
}

func (t *SearchStoredTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "search_stored",
		Description: "Search pattern across ALL stored content in this session. Uses SuffixArray for O(m log n) search. Returns matching lines with context.",
		Parameters: []ToolParameter{
			{Name: "pattern", ParamType: "string", Description: "The search pattern", Required: true},
			{Name: "limit", ParamType: "integer", Description: "Maximum results (default: 20)", Required: false},
		},
	}
}

type searchStoredArgs struct {
	Pattern string `json:"pattern"`
	Limit   *int   `json:"limit"`
}

func (t *SearchStoredTool) Validate(args json.RawMessage) error {
	var a searchStoredArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(a.Pattern) == "" {
		return fmt.Errorf("pattern cannot be empty")
	}
	return nil
}

func (t *SearchStoredTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if t.store == nil {
		return FailureResultf("no result store available"), nil
	}

	var a searchStoredArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	if strings.TrimSpace(a.Pattern) == "" {
		return FailureResultf("pattern cannot be empty"), nil
	}

	limit := 20
	if a.Limit != nil && *a.Limit > 0 {
		limit = *a.Limit
	}

	matches, err := t.store.Search(ctx, t.sessionID, a.Pattern, limit)
	if err != nil {
		return FailureResult(fmt.Errorf("search failed: %w", err)), nil
	}

	if len(matches) == 0 {
		return SuccessResult(fmt.Sprintf("No matches found for pattern: %s", a.Pattern)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matches for '%s':\n\n", len(matches), a.Pattern))
	for i, m := range matches {
		sb.WriteString(fmt.Sprintf("[%d] %s (line %d):\n  %s\n\n", i+1, m.Key.Key, m.Line, m.Context))
	}

	return SuccessResult(sb.String()), nil
}

// GetLinesTool retrieves specific line ranges from stored content.
type GetLinesTool struct {
	BaseTool
	store       *storage.ResultStore
	sessionID   string
	fileContext *StoredFileContext
}

// NewGetLinesTool creates a tool for getting line ranges.
func NewGetLinesTool(store *storage.ResultStore, sessionID string, fileContext *StoredFileContext) *GetLinesTool {
	return &GetLinesTool{
		store:       store,
		sessionID:   sessionID,
		fileContext: fileContext,
	}
}

func (t *GetLinesTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "get_lines",
		Description: "Get specific line range from stored content. If key is omitted, uses the most recently stored file.",
		Parameters: []ToolParameter{
			{Name: "key", ParamType: "string", Description: "The storage key (optional - defaults to last stored file)", Required: false},
			{Name: "start", ParamType: "integer", Description: "Start line (1-indexed, inclusive)", Required: true},
			{Name: "end", ParamType: "integer", Description: "End line (1-indexed, inclusive)", Required: true},
		},
	}
}

type getLinesArgs struct {
	Key   string `json:"key"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func (t *GetLinesTool) Validate(args json.RawMessage) error {
	var a getLinesArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Start < 1 {
		return fmt.Errorf("start must be >= 1")
	}
	if a.End < a.Start {
		return fmt.Errorf("end must be >= start")
	}
	return nil
}

func (t *GetLinesTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if t.store == nil {
		return FailureResultf("no result store available"), nil
	}

	var a getLinesArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	// Use last stored file if key not provided
	fileKey := a.Key
	if fileKey == "" && t.fileContext != nil {
		fileKey = t.fileContext.Last()
	}
	if fileKey == "" {
		return FailureResultf("no key provided and no files have been stored yet"), nil
	}

	key := storage.ResultKey{
		SessionID: t.sessionID,
		Key:       fileKey,
	}

	lines, err := t.store.GetLines(ctx, key, storage.LineRange{Start: a.Start, End: a.End})
	if err != nil {
		return FailureResult(fmt.Errorf("failed to get lines: %w", err)), nil
	}

	if lines == "" {
		return SuccessResult(fmt.Sprintf("No content found for key: %s (lines %d-%d)", fileKey, a.Start, a.End)), nil
	}

	return SuccessResult(fmt.Sprintf("Lines %d-%d of %s:\n\n%s", a.Start, a.End, fileKey, lines)), nil
}

// ListStoredTool lists stored results with optional prefix filter.
type ListStoredTool struct {
	BaseTool
	store       *storage.ResultStore
	sessionID   string
	fileContext *StoredFileContext
}

// NewListStoredTool creates a tool for listing stored content.
func NewListStoredTool(store *storage.ResultStore, sessionID string, fileContext *StoredFileContext) *ListStoredTool {
	return &ListStoredTool{
		store:       store,
		sessionID:   sessionID,
		fileContext: fileContext,
	}
}

func (t *ListStoredTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "list_stored",
		Description: "List all stored content in this session. Use prefix to filter (e.g., 'src/' for all files in src). Uses Trie for O(m+k) prefix lookup.",
		Parameters: []ToolParameter{
			{Name: "prefix", ParamType: "string", Description: "Optional prefix filter (e.g., 'src/', 'file:')", Required: false},
		},
	}
}

type listStoredArgs struct {
	Prefix string `json:"prefix"`
}

func (t *ListStoredTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if t.store == nil {
		return FailureResultf("no result store available"), nil
	}

	var a listStoredArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	var results []storage.ResultMetadata
	var err error

	if a.Prefix != "" {
		results, err = t.store.GetByPrefix(ctx, t.sessionID, a.Prefix)
	} else {
		results, err = t.store.List(ctx, t.sessionID, storage.QueryOptions{Limit: 100})
	}

	if err != nil {
		return FailureResult(fmt.Errorf("failed to list stored content: %w", err)), nil
	}

	if len(results) == 0 {
		if a.Prefix != "" {
			return SuccessResult(fmt.Sprintf("No stored content found with prefix: %s", a.Prefix)), nil
		}
		return SuccessResult("No stored content in this session"), nil
	}

	var sb strings.Builder
	if a.Prefix != "" {
		sb.WriteString(fmt.Sprintf("Stored content with prefix '%s' (%d items):\n\n", a.Prefix, len(results)))
	} else {
		sb.WriteString(fmt.Sprintf("All stored content (%d items):\n\n", len(results)))
	}

	for _, meta := range results {
		sb.WriteString(fmt.Sprintf("- %s (%d lines, %d bytes)\n", meta.Key.Key, meta.LineCount, meta.ByteSize))
		if meta.Summary != "" {
			// Show first line of summary
			firstLine := strings.Split(meta.Summary, "\n")[0]
			if len(firstLine) > 60 {
				firstLine = firstLine[:60] + "..."
			}
			sb.WriteString(fmt.Sprintf("  Preview: %s\n", firstLine))
		}
	}

	return SuccessResult(sb.String()), nil
}
