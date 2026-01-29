// Package storage provides persistence for conversations and memories.
//
// Enhanced Memory Storage provides rich memory capabilities beyond simple
// conversation history. Supports episodic memory (past executions),
// orchestration memory (decisions), and conversation memory (chat history).
package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MemoryType represents types of memory for different use cases.
type MemoryType string

const (
	// MemoryEpisodic represents past task executions and results.
	MemoryEpisodic MemoryType = "episodic"
	// MemoryOrchestration represents supervisor/router decisions and handoffs.
	MemoryOrchestration MemoryType = "orchestration"
	// MemoryConversation represents chat history (existing conversation storage).
	MemoryConversation MemoryType = "conversation"
)

// String returns the string representation of the memory type.
func (m MemoryType) String() string {
	return string(m)
}

// ParseMemoryType parses a string into a MemoryType.
func ParseMemoryType(s string) (MemoryType, error) {
	switch strings.ToLower(s) {
	case "episodic":
		return MemoryEpisodic, nil
	case "orchestration":
		return MemoryOrchestration, nil
	case "conversation":
		return MemoryConversation, nil
	default:
		return "", fmt.Errorf("unknown memory type: %s", s)
	}
}

// MemoryEntry represents a rich memory entry with metadata.
type MemoryEntry struct {
	// ID is a unique identifier for this memory.
	ID string `json:"id"`
	// SessionID is the session this memory belongs to.
	SessionID string `json:"session_id"`
	// AgentID is the optional agent that created this memory (empty if none).
	AgentID string `json:"agent_id,omitempty"`
	// Type is the type of memory.
	Type MemoryType `json:"memory_type"`
	// Content is the actual content.
	Content string `json:"content"`
	// CreatedAt is the Unix timestamp when created.
	CreatedAt int64 `json:"created_at"`
	// AccessedAt is the Unix timestamp when last accessed.
	AccessedAt int64 `json:"accessed_at"`
	// AccessCount is the number of times this memory has been accessed.
	AccessCount uint32 `json:"access_count"`
	// Metadata is optional JSON metadata for extensibility (empty if none).
	Metadata string `json:"metadata,omitempty"`
}

// NewMemoryEntry creates a new memory entry with defaults.
func NewMemoryEntry(sessionID string, memoryType MemoryType, content string) MemoryEntry {
	now := time.Now().Unix()
	return MemoryEntry{
		ID:          uuid.New().String(),
		SessionID:   sessionID,
		AgentID:     "", // Empty means no agent
		Type:        memoryType,
		Content:     content,
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 0, // Not accessed yet
		Metadata:    "", // Empty means no metadata
	}
}

// WithAgent sets the agent ID.
func (m MemoryEntry) WithAgent(agentID string) MemoryEntry {
	m.AgentID = agentID
	return m
}

// WithMetadata sets metadata as JSON string.
func (m MemoryEntry) WithMetadata(metadata string) MemoryEntry {
	m.Metadata = metadata
	return m
}

// MemoryStorage is the extended storage interface for rich memory capabilities.
// Extends ConversationStorage with structured memory operations.
type MemoryStorage interface {
	// StoreMemory stores a memory entry.
	StoreMemory(ctx context.Context, entry MemoryEntry) error

	// QueryMemories queries memories with optional filters.
	QueryMemories(ctx context.Context, sessionID string, memoryType *MemoryType, limit int) ([]MemoryEntry, error)

	// GetRecentMemories gets recent memories across all types.
	GetRecentMemories(ctx context.Context, sessionID string, limit int) ([]MemoryEntry, error)

	// GetMemory gets a specific memory by ID and updates access tracking.
	GetMemory(ctx context.Context, id string) (*MemoryEntry, error)

	// DeleteMemory deletes a specific memory.
	DeleteMemory(ctx context.Context, id string) error

	// DeleteSessionMemories deletes all memories for a session.
	DeleteSessionMemories(ctx context.Context, sessionID string) error
}
