// Filesystem Tools - Read, Write, Edit, Append operations.
//
// Information Hiding:
// - File I/O implementation details hidden
// - Path validation and security checks hidden
// - Error handling for file operations abstracted

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/richinex/ariadne/model"
)

// ReadFileTool reads file contents.
// Implements RLM pattern: files are stored externally and a reference is returned.
type ReadFileTool struct {
	BaseTool
	allowedPaths []string
	maxSizeBytes int64
	contentStore model.ContentStore
	fileContext  *StoredFileContext
}

// NewReadFileTool creates a new read file tool.
func NewReadFileTool(maxSizeBytes int64) *ReadFileTool {
	return &ReadFileTool{
		maxSizeBytes: maxSizeBytes,
	}
}

// WithAllowedPaths sets the allowed path prefixes.
func (t *ReadFileTool) WithAllowedPaths(paths []string) *ReadFileTool {
	t.allowedPaths = paths
	return t
}

// WithContentStore enables RLM pattern - files stored externally, reference returned.
func (t *ReadFileTool) WithContentStore(store model.ContentStore) *ReadFileTool {
	t.contentStore = store
	return t
}

// WithFileContext enables automatic tracking of stored files.
func (t *ReadFileTool) WithFileContext(ctx *StoredFileContext) *ReadFileTool {
	t.fileContext = ctx
	return t
}

// Metadata returns the tool metadata.
func (t *ReadFileTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "read_file",
		Description: "Read the contents of a file from the filesystem",
		Parameters: []ToolParameter{
			{Name: "path", ParamType: "string", Description: "Path to the file to read", Required: true},
		},
	}
}

type readFileArgs struct {
	Path string `json:"path"`
}

// Validate validates the arguments.
func (t *ReadFileTool) Validate(args json.RawMessage) error {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	return nil
}

// Execute reads the file.
// If ContentStore is configured and file exceeds threshold, stores externally
// and returns a reference (RLM pattern) instead of full content.
func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	if a.Path == "" {
		return FailureResultf("path cannot be empty"), nil
	}

	if !pathAllowed(a.Path, t.allowedPaths) {
		return FailureResultf("access to path '%s' is not allowed", a.Path), nil
	}

	// Check file exists
	info, err := os.Stat(a.Path)
	if os.IsNotExist(err) {
		return FailureResultf("file does not exist: %s", a.Path), nil
	}
	if err != nil {
		return FailureResult(fmt.Errorf("failed to read file metadata: %w", err)), nil
	}

	// Check file size
	if info.Size() > t.maxSizeBytes {
		return FailureResultf("file too large: %d bytes (max: %d bytes)", info.Size(), t.maxSizeBytes), nil
	}

	// Read file
	content, err := os.ReadFile(a.Path)
	if err != nil {
		return FailureResult(fmt.Errorf("failed to read file: %w", err)), nil
	}

	// RLM pattern: always store files externally when ContentStore is available
	// This keeps agent context small - agent uses get_lines/search_stored to explore
	if t.contentStore != nil {
		stored, err := t.contentStore.StoreContent(ctx, model.FileKey(a.Path), string(content))
		if err != nil {
			// Fall back to returning content if storage fails
			return SuccessResult(string(content)), nil
		}

		// Track this file in context for easy reference
		if t.fileContext != nil {
			t.fileContext.Add(a.Path)
		}

		// Return ONLY metadata - no content
		// Agent can use get_lines without specifying key (uses last stored)
		return SuccessResult(fmt.Sprintf(
			"[File stored: %d bytes, %d lines]\nUse get_lines to retrieve content (key is automatic).",
			stored.Bytes, stored.Lines,
		)), nil
	}

	return SuccessResult(string(content)), nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct {
	BaseTool
	allowedPaths []string
	maxSizeBytes int64
}

// NewWriteFileTool creates a new write file tool.
func NewWriteFileTool(maxSizeBytes int64) *WriteFileTool {
	return &WriteFileTool{
		maxSizeBytes: maxSizeBytes,
	}
}

// WithAllowedPaths sets the allowed path prefixes.
func (t *WriteFileTool) WithAllowedPaths(paths []string) *WriteFileTool {
	t.allowedPaths = paths
	return t
}

// Metadata returns the tool metadata.
func (t *WriteFileTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "write_file",
		Description: "Write content to a file on the filesystem",
		Parameters: []ToolParameter{
			{Name: "path", ParamType: "string", Description: "Path to the file to write", Required: true},
			{Name: "content", ParamType: "string", Description: "Content to write", Required: true},
		},
	}
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Validate validates the arguments.
func (t *WriteFileTool) Validate(args json.RawMessage) error {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	return nil
}

// Execute writes to the file.
func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	if a.Path == "" {
		return FailureResultf("path cannot be empty"), nil
	}

	if int64(len(a.Content)) > t.maxSizeBytes {
		return FailureResultf("content too large: %d bytes (max: %d bytes)", len(a.Content), t.maxSizeBytes), nil
	}

	if !pathAllowedForWrite(a.Path, t.allowedPaths) {
		return FailureResultf("access to path '%s' is not allowed", a.Path), nil
	}

	// Create parent directory if needed
	dir := parentDir(a.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return FailureResult(fmt.Errorf("failed to create directory: %w", err)), nil
	}

	// Write file
	if err := os.WriteFile(a.Path, []byte(a.Content), 0644); err != nil {
		return FailureResult(fmt.Errorf("failed to write file: %w", err)), nil
	}

	return SuccessResult(fmt.Sprintf("Successfully wrote %d bytes to %s", len(a.Content), a.Path)), nil
}

// parentDir returns the parent directory of a path.
func parentDir(path string) string {
	// Find last separator
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return "."
}

// AppendFileTool appends content to a file.
type AppendFileTool struct {
	BaseTool
	allowedPaths []string
	maxSizeBytes int64
}

// NewAppendFileTool creates a new append file tool.
func NewAppendFileTool(maxSizeBytes int64) *AppendFileTool {
	return &AppendFileTool{
		maxSizeBytes: maxSizeBytes,
	}
}

// WithAllowedPaths sets the allowed path prefixes.
func (t *AppendFileTool) WithAllowedPaths(paths []string) *AppendFileTool {
	t.allowedPaths = paths
	return t
}

// Metadata returns the tool metadata.
func (t *AppendFileTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "append_file",
		Description: "Append content to an existing file on the filesystem. Creates the file if it doesn't exist.",
		Parameters: []ToolParameter{
			{Name: "path", ParamType: "string", Description: "Path to the file to append to", Required: true},
			{Name: "content", ParamType: "string", Description: "Content to append", Required: true},
		},
	}
}

type appendFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Validate validates the arguments.
func (t *AppendFileTool) Validate(args json.RawMessage) error {
	var a appendFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	return nil
}

// Execute appends to the file.
func (t *AppendFileTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var a appendFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	if a.Path == "" {
		return FailureResultf("path cannot be empty"), nil
	}

	if int64(len(a.Content)) > t.maxSizeBytes {
		return FailureResultf("content too large: %d bytes (max: %d bytes)", len(a.Content), t.maxSizeBytes), nil
	}

	if !pathAllowedForWrite(a.Path, t.allowedPaths) {
		return FailureResultf("access to path '%s' is not allowed", a.Path), nil
	}

	// Create parent directory if needed
	dir := parentDir(a.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return FailureResult(fmt.Errorf("failed to create directory: %w", err)), nil
	}

	// Open file for appending (create if not exists)
	f, err := os.OpenFile(a.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return FailureResult(fmt.Errorf("failed to open file: %w", err)), nil
	}
	defer f.Close()

	if _, err := f.WriteString(a.Content); err != nil {
		return FailureResult(fmt.Errorf("failed to write to file: %w", err)), nil
	}

	return SuccessResult(fmt.Sprintf("Successfully appended %d bytes to %s", len(a.Content), a.Path)), nil
}

// EditFileTool performs search/replace operations on files.
type EditFileTool struct {
	BaseTool
	allowedPaths []string
	maxSizeBytes int64
}

// NewEditFileTool creates a new edit file tool.
func NewEditFileTool(maxSizeBytes int64) *EditFileTool {
	return &EditFileTool{
		maxSizeBytes: maxSizeBytes,
	}
}

// WithAllowedPaths sets the allowed path prefixes.
func (t *EditFileTool) WithAllowedPaths(paths []string) *EditFileTool {
	t.allowedPaths = paths
	return t
}

// Metadata returns the tool metadata.
func (t *EditFileTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "edit_file",
		Description: "Edit a file by replacing a target string with new content",
		Parameters: []ToolParameter{
			{Name: "path", ParamType: "string", Description: "Path to the file to edit", Required: true},
			{Name: "search", ParamType: "string", Description: "String to search for", Required: true},
			{Name: "replace", ParamType: "string", Description: "Replacement string", Required: true},
			{Name: "replace_all", ParamType: "boolean", Description: "Replace all occurrences (default: false)", Required: false},
		},
	}
}

type editFileArgs struct {
	Path       string `json:"path"`
	Search     string `json:"search"`
	Replace    string `json:"replace"`
	ReplaceAll *bool  `json:"replace_all"`
}

// Validate validates the arguments.
func (t *EditFileTool) Validate(args json.RawMessage) error {
	var a editFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if a.Search == "" {
		return fmt.Errorf("search string cannot be empty")
	}
	return nil
}

// Execute performs the edit.
func (t *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var a editFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	if a.Path == "" {
		return FailureResultf("path cannot be empty"), nil
	}
	if a.Search == "" {
		return FailureResultf("search string cannot be empty"), nil
	}

	if !pathAllowedForWrite(a.Path, t.allowedPaths) {
		return FailureResultf("access to path '%s' is not allowed", a.Path), nil
	}

	// Check file exists
	if _, err := os.Stat(a.Path); os.IsNotExist(err) {
		return FailureResultf("file does not exist: %s", a.Path), nil
	}

	// Read file
	content, err := os.ReadFile(a.Path)
	if err != nil {
		return FailureResult(fmt.Errorf("failed to read file: %w", err)), nil
	}

	if int64(len(content)) > t.maxSizeBytes {
		return FailureResultf("file too large: %d bytes (max: %d bytes)", len(content), t.maxSizeBytes), nil
	}

	contentStr := string(content)
	occurrences := strings.Count(contentStr, a.Search)

	if occurrences == 0 {
		return FailureResultf("search string not found"), nil
	}

	replaceAll := a.ReplaceAll != nil && *a.ReplaceAll
	if !replaceAll && occurrences > 1 {
		return FailureResultf("search string occurs %d times; set replace_all=true to replace all", occurrences), nil
	}

	// Perform replacement
	var updated string
	if replaceAll {
		updated = strings.ReplaceAll(contentStr, a.Search, a.Replace)
	} else {
		updated = strings.Replace(contentStr, a.Search, a.Replace, 1)
	}

	if int64(len(updated)) > t.maxSizeBytes {
		return FailureResultf("updated content too large: %d bytes (max: %d bytes)", len(updated), t.maxSizeBytes), nil
	}

	// Write file
	if err := os.WriteFile(a.Path, []byte(updated), 0644); err != nil {
		return FailureResult(fmt.Errorf("failed to write file: %w", err)), nil
	}

	replacedCount := 1
	if replaceAll {
		replacedCount = occurrences
	}

	return SuccessResult(fmt.Sprintf("Replaced %d occurrence(s) in %s", replacedCount, a.Path)), nil
}
