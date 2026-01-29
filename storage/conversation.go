// Package storage provides conversation storage abstraction.
//
// Information Hiding:
// - Storage backend implementation details hidden behind interface
// - Allows swapping between memory, filesystem, SQLite without API changes
// - Each storage implementation encapsulates its own data structures and protocols

package storage

import (
	"context"

	"github.com/richinex/davingo/llm"
)

// ConversationStorage defines the interface for storing conversation history.
// Implementations can use different backends (memory, file, database, cache).
type ConversationStorage interface {
	// Save saves conversation history for a session.
	Save(ctx context.Context, sessionID string, history []llm.ChatMessage) error

	// Load loads conversation history for a session.
	// Returns empty slice (not nil) if session doesn't exist.
	// Returns error only for storage failures (I/O errors, etc.), not missing sessions.
	Load(ctx context.Context, sessionID string) ([]llm.ChatMessage, error)

	// Delete deletes conversation history for a session.
	Delete(ctx context.Context, sessionID string) error

	// ListSessions lists all session IDs.
	ListSessions(ctx context.Context) ([]string, error)

	// Exists checks if a session exists.
	Exists(ctx context.Context, sessionID string) (bool, error)
}
