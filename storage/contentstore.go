// Package storage provides content storage for DSA-based result operations.
//
// ContentStorage persists metadata for content that supports Trie-based
// prefix search and SuffixArray-based pattern search operations.

package storage

import (
	"context"
)

// ContentStorage persists content result metadata and tracks access patterns.
type ContentStorage interface {
	// StoreResult stores a content result.
	StoreResult(ctx context.Context, result ContentResult) error

	// LoadAllResults loads all results from storage.
	LoadAllResults(ctx context.Context) ([]ContentResult, error)

	// LoadResultsBySession loads results for a specific session.
	LoadResultsBySession(ctx context.Context, sessionID string) ([]ContentResult, error)

	// UpdateResultAccess updates access timestamp and count for a result.
	UpdateResultAccess(ctx context.Context, sessionID, key string) error

	// DeleteResult removes a specific result.
	DeleteResult(ctx context.Context, sessionID, key string) error

	// DeleteSessionResults removes all results for a session.
	DeleteSessionResults(ctx context.Context, sessionID string) error
}

// ContentResult represents stored content metadata including access tracking.
type ContentResult struct {
	SessionID   string // Session that created this result
	Key         string // Unique key within session
	ContentHash string // Hash of content for deduplication
	Content     string // Actual file content stored in SQLite
	Summary     string // Brief summary of content
	LineCount   int    // Number of lines in content
	ByteSize    int    // Size in bytes
	CreatedAt   int64  // Unix timestamp of creation
	AccessedAt  int64  // Unix timestamp of last access
	AccessCount int    // Number of times accessed
}
