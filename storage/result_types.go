// Result types for hybrid storage of large agent outputs.
//
// Information Hiding:
// - Storage layer details (memory, SQLite, file) hidden behind interface
// - DSA implementation details (Trie, SuffixArray) encapsulated
// - Content addressing and deduplication handled internally
package storage

import (
	"time"
)

// ResultKey uniquely identifies a stored result.
type ResultKey struct {
	SessionID string // Session this result belongs to
	Key       string // User-provided key (e.g., file path, query)
}

// ResultMetadata contains summary information about stored content.
// This is what the supervisor receives instead of full content.
type ResultMetadata struct {
	Key         ResultKey `json:"key"`
	ContentHash string    `json:"content_hash"` // Hash of content for deduplication
	Summary     string    `json:"summary"`      // First N characters or lines
	LineCount   int       `json:"line_count"`   // Total lines in content
	ByteSize    int       `json:"byte_size"`    // Size in bytes
	CreatedAt   time.Time `json:"created_at"`
	AccessedAt  time.Time `json:"accessed_at"`
	AccessCount int       `json:"access_count"`
}

// Result contains the full stored content with metadata.
type Result struct {
	Metadata ResultMetadata
	Content  string // Full content
}

// SearchMatch represents a pattern match within stored results.
type SearchMatch struct {
	Key      ResultKey // Which result this match is in
	Position int       // Character position in content
	Line     int       // Line number (1-indexed)
	Context  string    // Surrounding context (the line containing match)
}

// StoreOptions configures how content is stored.
type StoreOptions struct {
	SummaryLength int  // Max characters for summary (default: 200)
	SummaryLines  int  // Max lines for summary (default: 5)
	ForceStore    bool // Store even if below threshold
}

// DefaultStoreOptions returns sensible defaults.
func DefaultStoreOptions() StoreOptions {
	return StoreOptions{
		SummaryLength: 200,
		SummaryLines:  5,
		ForceStore:    false,
	}
}

// QueryOptions configures result retrieval.
type QueryOptions struct {
	Limit  int // Max results to return
	Offset int // Skip first N results
}

// LineRange specifies a range of lines to retrieve.
type LineRange struct {
	Start int // Start line (1-indexed, inclusive)
	End   int // End line (1-indexed, inclusive)
}
