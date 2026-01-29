package storage

import (
	"context"
	"testing"
	"time"

	"github.com/richinex/ariadne/llm"
)

func TestSqliteStorageSaveAndLoad(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

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

func TestSqliteStorageLoadNonexistentSession(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	loaded, err := storage.Load(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("expected empty slice, got %d messages", len(loaded))
	}
}

func TestSqliteStorageDeleteSession(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

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

func TestSqliteStorageListSessions(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

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
}

func TestSqliteStorageOverwriteSession(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	messages1 := []llm.ChatMessage{
		{Role: "user", Content: "First"},
	}

	messages2 := []llm.ChatMessage{
		{Role: "user", Content: "Second"},
		{Role: "assistant", Content: "Response"},
	}

	if err := storage.Save(ctx, "test-session", messages1); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := storage.Save(ctx, "test-session", messages2); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := storage.Load(ctx, "test-session")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded))
	}
	if loaded[0].Content != "Second" {
		t.Errorf("expected 'Second', got '%s'", loaded[0].Content)
	}
}

func TestSqliteStorageStoreAndQueryMemory(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	entry := NewMemoryEntry("test-session", MemoryEpisodic, "Test memory content").
		WithAgent("test-agent")

	if err := storage.StoreMemory(ctx, entry); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	memType := MemoryEpisodic
	memories, err := storage.QueryMemories(ctx, "test-session", &memType, 10)
	if err != nil {
		t.Fatalf("QueryMemories failed: %v", err)
	}

	if len(memories) != 1 {
		t.Errorf("expected 1 memory, got %d", len(memories))
	}
	if memories[0].Content != "Test memory content" {
		t.Errorf("expected 'Test memory content', got '%s'", memories[0].Content)
	}
	if memories[0].AgentID != "test-agent" {
		t.Errorf("expected agent_id 'test-agent', got %q", memories[0].AgentID)
	}
}

func TestSqliteStorageQueryMemoriesByType(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Store different types of memories
	episodic := NewMemoryEntry("test-session", MemoryEpisodic, "Episodic memory")
	orchestration := NewMemoryEntry("test-session", MemoryOrchestration, "Orchestration memory")
	conversation := NewMemoryEntry("test-session", MemoryConversation, "Conversation memory")

	if err := storage.StoreMemory(ctx, episodic); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}
	if err := storage.StoreMemory(ctx, orchestration); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}
	if err := storage.StoreMemory(ctx, conversation); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Query only episodic
	memType := MemoryEpisodic
	episodicMemories, err := storage.QueryMemories(ctx, "test-session", &memType, 10)
	if err != nil {
		t.Fatalf("QueryMemories failed: %v", err)
	}
	if len(episodicMemories) != 1 {
		t.Errorf("expected 1 episodic memory, got %d", len(episodicMemories))
	}
	if episodicMemories[0].Content != "Episodic memory" {
		t.Errorf("expected 'Episodic memory', got '%s'", episodicMemories[0].Content)
	}

	// Query all
	allMemories, err := storage.GetRecentMemories(ctx, "test-session", 10)
	if err != nil {
		t.Fatalf("GetRecentMemories failed: %v", err)
	}
	if len(allMemories) != 3 {
		t.Errorf("expected 3 memories, got %d", len(allMemories))
	}
}

func TestSqliteStorageGetMemoryUpdatesAccess(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	entry := NewMemoryEntry("test-session", MemoryEpisodic, "Test content")
	id := entry.ID

	if err := storage.StoreMemory(ctx, entry); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Access the memory
	memory, err := storage.GetMemory(ctx, id)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if memory == nil {
		t.Fatal("expected memory to exist")
		return
	}
	if memory.AccessCount != 1 { // Initial 0 + our access
		t.Errorf("expected access_count 1, got %d", memory.AccessCount)
	}

	// Access again
	memory, err = storage.GetMemory(ctx, id)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if memory.AccessCount != 2 {
		t.Errorf("expected access_count 2, got %d", memory.AccessCount)
	}
}

func TestSqliteStorageDeleteMemory(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	entry := NewMemoryEntry("test-session", MemoryEpisodic, "Test content")
	id := entry.ID

	if err := storage.StoreMemory(ctx, entry); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Verify it exists
	memory, err := storage.GetMemory(ctx, id)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if memory == nil {
		t.Fatal("expected memory to exist")
	}

	// Delete it
	if err := storage.DeleteMemory(ctx, id); err != nil {
		t.Fatalf("DeleteMemory failed: %v", err)
	}

	// Verify it's gone
	memory, err = storage.GetMemory(ctx, id)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if memory != nil {
		t.Error("expected memory to be deleted")
	}
}

func TestSqliteStorageDeleteSessionMemories(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Store memories in two sessions
	if err := storage.StoreMemory(ctx, NewMemoryEntry("session-1", MemoryEpisodic, "Memory 1")); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}
	if err := storage.StoreMemory(ctx, NewMemoryEntry("session-1", MemoryOrchestration, "Memory 2")); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}
	if err := storage.StoreMemory(ctx, NewMemoryEntry("session-2", MemoryEpisodic, "Memory 3")); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Delete session-1 memories
	if err := storage.DeleteSessionMemories(ctx, "session-1"); err != nil {
		t.Fatalf("DeleteSessionMemories failed: %v", err)
	}

	// Verify session-1 is empty but session-2 is intact
	session1, err := storage.GetRecentMemories(ctx, "session-1", 10)
	if err != nil {
		t.Fatalf("GetRecentMemories failed: %v", err)
	}
	session2, err := storage.GetRecentMemories(ctx, "session-2", 10)
	if err != nil {
		t.Fatalf("GetRecentMemories failed: %v", err)
	}

	if len(session1) != 0 {
		t.Errorf("expected 0 memories in session-1, got %d", len(session1))
	}
	if len(session2) != 1 {
		t.Errorf("expected 1 memory in session-2, got %d", len(session2))
	}
}

// ContentStorage tests

func TestSqliteStorageStoreAndLoadResult(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now().Unix()

	result := ContentResult{
		SessionID:   "test-session",
		Key:         "file.txt",
		ContentHash: "abc123",
		Summary:     "Test summary",
		LineCount:   10,
		ByteSize:    100,
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 1,
	}

	if err := storage.StoreResult(ctx, result); err != nil {
		t.Fatalf("StoreResult failed: %v", err)
	}

	results, err := storage.LoadResultsBySession(ctx, "test-session")
	if err != nil {
		t.Fatalf("LoadResultsBySession failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Key != "file.txt" {
		t.Errorf("expected key 'file.txt', got '%s'", results[0].Key)
	}
	if results[0].ContentHash != "abc123" {
		t.Errorf("expected hash 'abc123', got '%s'", results[0].ContentHash)
	}
}

func TestSqliteStorageLoadAllResults(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now().Unix()

	// Store results in two sessions
	result1 := ContentResult{
		SessionID:   "session-1",
		Key:         "file1.txt",
		ContentHash: "hash1",
		Summary:     "Summary 1",
		LineCount:   5,
		ByteSize:    50,
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 1,
	}
	result2 := ContentResult{
		SessionID:   "session-2",
		Key:         "file2.txt",
		ContentHash: "hash2",
		Summary:     "Summary 2",
		LineCount:   10,
		ByteSize:    100,
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 1,
	}

	if err := storage.StoreResult(ctx, result1); err != nil {
		t.Fatalf("StoreResult failed: %v", err)
	}
	if err := storage.StoreResult(ctx, result2); err != nil {
		t.Fatalf("StoreResult failed: %v", err)
	}

	results, err := storage.LoadAllResults(ctx)
	if err != nil {
		t.Fatalf("LoadAllResults failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestSqliteStorageUpdateResultAccess(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now().Unix()

	result := ContentResult{
		SessionID:   "test-session",
		Key:         "file.txt",
		ContentHash: "abc123",
		Summary:     "Test summary",
		LineCount:   10,
		ByteSize:    100,
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 1,
	}

	if err := storage.StoreResult(ctx, result); err != nil {
		t.Fatalf("StoreResult failed: %v", err)
	}

	// Update access
	if err := storage.UpdateResultAccess(ctx, "test-session", "file.txt"); err != nil {
		t.Fatalf("UpdateResultAccess failed: %v", err)
	}

	results, err := storage.LoadResultsBySession(ctx, "test-session")
	if err != nil {
		t.Fatalf("LoadResultsBySession failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AccessCount != 2 {
		t.Errorf("expected access_count 2, got %d", results[0].AccessCount)
	}
}

func TestSqliteStorageDeleteResult(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now().Unix()

	result := ContentResult{
		SessionID:   "test-session",
		Key:         "file.txt",
		ContentHash: "abc123",
		Summary:     "Test summary",
		LineCount:   10,
		ByteSize:    100,
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 1,
	}

	if err := storage.StoreResult(ctx, result); err != nil {
		t.Fatalf("StoreResult failed: %v", err)
	}

	// Verify it exists
	results, err := storage.LoadResultsBySession(ctx, "test-session")
	if err != nil {
		t.Fatalf("LoadResultsBySession failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Delete it
	if err := storage.DeleteResult(ctx, "test-session", "file.txt"); err != nil {
		t.Fatalf("DeleteResult failed: %v", err)
	}

	// Verify it's gone
	results, err = storage.LoadResultsBySession(ctx, "test-session")
	if err != nil {
		t.Fatalf("LoadResultsBySession failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}

func TestSqliteStorageDeleteSessionResults(t *testing.T) {
	storage, err := NewSqliteInMemory()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now().Unix()

	// Store results in two sessions
	for i, sessionID := range []string{"session-1", "session-1", "session-2"} {
		result := ContentResult{
			SessionID:   sessionID,
			Key:         "file" + string(rune('a'+i)) + ".txt",
			ContentHash: "hash" + string(rune('a'+i)),
			Summary:     "Summary",
			LineCount:   5,
			ByteSize:    50,
			CreatedAt:   now,
			AccessedAt:  now,
			AccessCount: 1,
		}
		if err := storage.StoreResult(ctx, result); err != nil {
			t.Fatalf("StoreResult failed: %v", err)
		}
	}

	// Delete session-1 results
	if err := storage.DeleteSessionResults(ctx, "session-1"); err != nil {
		t.Fatalf("DeleteSessionResults failed: %v", err)
	}

	// Verify session-1 is empty
	session1, err := storage.LoadResultsBySession(ctx, "session-1")
	if err != nil {
		t.Fatalf("LoadResultsBySession failed: %v", err)
	}
	if len(session1) != 0 {
		t.Errorf("expected 0 results for session-1, got %d", len(session1))
	}

	// Verify session-2 is intact
	session2, err := storage.LoadResultsBySession(ctx, "session-2")
	if err != nil {
		t.Fatalf("LoadResultsBySession failed: %v", err)
	}
	if len(session2) != 1 {
		t.Errorf("expected 1 result for session-2, got %d", len(session2))
	}
}
