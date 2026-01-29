package storage

import (
	"context"
	"testing"

	"github.com/richinex/davingo/llm"
)

func TestInMemoryStorageSaveAndLoad(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	messages := []llm.ChatMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	if err := storage.Save(ctx, "test-session", messages); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := storage.Load(ctx, "test-session")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded))
	}
	if loaded[0].Content != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", loaded[0].Content)
	}
	if loaded[1].Content != "Hi there" {
		t.Errorf("expected 'Hi there', got '%s'", loaded[1].Content)
	}
}

func TestInMemoryStorageLoadNonexistentSession(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	loaded, err := storage.Load(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("expected empty slice, got %d messages", len(loaded))
	}
}

func TestInMemoryStorageDeleteSession(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	messages := []llm.ChatMessage{
		{Role: "user", Content: "Test"},
	}

	if err := storage.Save(ctx, "test-session", messages); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	exists, err := storage.Exists(ctx, "test-session")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("expected session to exist")
	}

	if err := storage.Delete(ctx, "test-session"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	exists, err = storage.Exists(ctx, "test-session")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected session to not exist after deletion")
	}
}

func TestInMemoryStorageListSessions(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	msg := []llm.ChatMessage{
		{Role: "user", Content: "Test"},
	}

	if err := storage.Save(ctx, "session-1", msg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := storage.Save(ctx, "session-2", msg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	sessions, err := storage.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}

	found1, found2 := false, false
	for _, s := range sessions {
		if s == "session-1" {
			found1 = true
		}
		if s == "session-2" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Errorf("expected to find both sessions, found1=%v found2=%v", found1, found2)
	}
}

func TestInMemoryStorageIsolation(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	// Save messages
	original := []llm.ChatMessage{
		{Role: "user", Content: "Original"},
	}
	if err := storage.Save(ctx, "test-session", original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Modify the original slice
	original[0].Content = "Modified"

	// Load and verify the stored copy is not affected
	loaded, err := storage.Load(ctx, "test-session")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded[0].Content != "Original" {
		t.Errorf("expected 'Original', got '%s' - storage should copy data", loaded[0].Content)
	}
}
