// Package storage provides in-memory conversation storage.
//
// Information Hiding:
// - Map storage structure hidden from users
// - Thread-safe access via RWMutex hidden behind interface
// - Suitable for testing and ephemeral sessions

package storage

import (
	"context"
	"sync"

	"github.com/richinex/ariadne/llm"
)

// InMemoryStorage implements ConversationStorage using an in-memory map.
// Data is lost when process terminates.
type InMemoryStorage struct {
	mu       sync.RWMutex
	sessions map[string][]llm.ChatMessage
}

// NewInMemoryStorage creates a new in-memory storage.
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		sessions: make(map[string][]llm.ChatMessage),
	}
}

// Save saves conversation history for a session.
func (s *InMemoryStorage) Save(ctx context.Context, sessionID string, history []llm.ChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy to avoid external mutations
	copied := make([]llm.ChatMessage, len(history))
	copy(copied, history)
	s.sessions[sessionID] = copied

	return nil
}

// Load loads conversation history for a session.
// Returns empty slice if session doesn't exist.
func (s *InMemoryStorage) Load(ctx context.Context, sessionID string) ([]llm.ChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history, ok := s.sessions[sessionID]
	if !ok {
		return []llm.ChatMessage{}, nil
	}

	// Return a copy to avoid external mutations
	copied := make([]llm.ChatMessage, len(history))
	copy(copied, history)
	return copied, nil
}

// Delete deletes conversation history for a session.
func (s *InMemoryStorage) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
	return nil
}

// ListSessions lists all session IDs.
func (s *InMemoryStorage) ListSessions(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]string, 0, len(s.sessions))
	for sessionID := range s.sessions {
		sessions = append(sessions, sessionID)
	}
	return sessions, nil
}

// Exists checks if a session exists.
func (s *InMemoryStorage) Exists(ctx context.Context, sessionID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.sessions[sessionID]
	return ok, nil
}

// Verify InMemoryStorage implements ConversationStorage
var _ ConversationStorage = (*InMemoryStorage)(nil)
