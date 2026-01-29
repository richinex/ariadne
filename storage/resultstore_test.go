package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInMemoryResultStore(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()
	key := ResultKey{SessionID: "test-session", Key: "file.txt"}
	content := "Hello, World!\nThis is a test.\nLine three."

	// Test Store
	meta, err := store.Store(ctx, key, content, DefaultStoreOptions())
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if meta.LineCount != 3 {
		t.Errorf("expected 3 lines, got %d", meta.LineCount)
	}
	if meta.ByteSize != len(content) {
		t.Errorf("expected %d bytes, got %d", len(content), meta.ByteSize)
	}
	if meta.ContentHash == "" {
		t.Error("expected non-empty content hash")
	}

	// Test Get
	result, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
		return
	}
	if result.Content != content {
		t.Errorf("expected content %q, got %q", content, result.Content)
	}

	// Test GetMetadata
	gotMeta, err := store.GetMetadata(ctx, key)
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if gotMeta == nil {
		t.Fatal("expected metadata, got nil")
		return
	}
	if gotMeta.LineCount != 3 {
		t.Errorf("expected 3 lines, got %d", gotMeta.LineCount)
	}
}

func TestResultStoreGetLines(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()
	key := ResultKey{SessionID: "test-session", Key: "file.txt"}
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"

	_, err := store.Store(ctx, key, content, DefaultStoreOptions())
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Get lines 2-4
	lines, err := store.GetLines(ctx, key, LineRange{Start: 2, End: 4})
	if err != nil {
		t.Fatalf("GetLines failed: %v", err)
	}

	expected := "Line 2\nLine 3\nLine 4"
	if lines != expected {
		t.Errorf("expected %q, got %q", expected, lines)
	}
}

func TestResultStoreSearch(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()

	// Store multiple results
	key1 := ResultKey{SessionID: "test-session", Key: "file1.txt"}
	content1 := "Hello World\nThis contains pattern here\nEnd"

	key2 := ResultKey{SessionID: "test-session", Key: "file2.txt"}
	content2 := "Another file\nNo match\nMore content"

	key3 := ResultKey{SessionID: "test-session", Key: "file3.txt"}
	content3 := "Third file\nThe pattern appears again\nDone"

	_, _ = store.Store(ctx, key1, content1, DefaultStoreOptions())
	_, _ = store.Store(ctx, key2, content2, DefaultStoreOptions())
	_, _ = store.Store(ctx, key3, content3, DefaultStoreOptions())

	// Search for "pattern"
	matches, err := store.Search(ctx, "test-session", "pattern", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}

	// Verify matches are from correct files
	foundFile1, foundFile3 := false, false
	for _, m := range matches {
		if m.Key.Key == "file1.txt" {
			foundFile1 = true
		}
		if m.Key.Key == "file3.txt" {
			foundFile3 = true
		}
	}
	if !foundFile1 || !foundFile3 {
		t.Errorf("expected matches in file1.txt and file3.txt")
	}
}

func TestResultStoreGetByPrefix(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()

	_, _ = store.Store(ctx, ResultKey{SessionID: "test", Key: "src/main.go"}, "content1", DefaultStoreOptions())
	_, _ = store.Store(ctx, ResultKey{SessionID: "test", Key: "src/util.go"}, "content2", DefaultStoreOptions())
	_, _ = store.Store(ctx, ResultKey{SessionID: "test", Key: "test/main_test.go"}, "content3", DefaultStoreOptions())

	results, err := store.GetByPrefix(ctx, "test", "src/")
	if err != nil {
		t.Fatalf("GetByPrefix failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestResultStoreContentDeduplication(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()
	content := "Same content for both"

	key1 := ResultKey{SessionID: "test", Key: "file1.txt"}
	key2 := ResultKey{SessionID: "test", Key: "file2.txt"}

	meta1, _ := store.Store(ctx, key1, content, DefaultStoreOptions())
	meta2, _ := store.Store(ctx, key2, content, DefaultStoreOptions())

	// Same content should have same hash
	if meta1.ContentHash != meta2.ContentHash {
		t.Error("expected same hash for duplicate content")
	}
}

func TestResultStoreDelete(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()
	key := ResultKey{SessionID: "test", Key: "file.txt"}

	_, _ = store.Store(ctx, key, "content", DefaultStoreOptions())

	// Verify exists
	result, _ := store.Get(ctx, key)
	if result == nil {
		t.Fatal("expected result before delete")
	}

	// Delete
	err := store.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	result, _ = store.Get(ctx, key)
	if result != nil {
		t.Error("expected nil after delete")
	}
}

func TestResultStoreDeleteSession(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()

	_, _ = store.Store(ctx, ResultKey{SessionID: "session1", Key: "file1.txt"}, "content1", DefaultStoreOptions())
	_, _ = store.Store(ctx, ResultKey{SessionID: "session1", Key: "file2.txt"}, "content2", DefaultStoreOptions())
	_, _ = store.Store(ctx, ResultKey{SessionID: "session2", Key: "file3.txt"}, "content3", DefaultStoreOptions())

	// Delete session1
	err := store.DeleteSession(ctx, "session1")
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// session1 results should be gone
	list1, _ := store.List(ctx, "session1", QueryOptions{})
	if len(list1) != 0 {
		t.Errorf("expected 0 results for session1, got %d", len(list1))
	}

	// session2 should still exist
	list2, _ := store.List(ctx, "session2", QueryOptions{})
	if len(list2) != 1 {
		t.Errorf("expected 1 result for session2, got %d", len(list2))
	}
}

func TestResultStoreWithPersistence(t *testing.T) {
	// Create temp directory for SQLite database
	tmpDir, err := os.MkdirTemp("", "resultstore-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "results.db")

	// Open SQLite for ContentStorage
	db, err := OpenSqlite(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	store, err := NewResultStore(db)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create store: %v", err)
	}

	ctx := context.Background()
	key := ResultKey{SessionID: "test", Key: "large.txt"}

	// Create content for testing
	content := strings.Repeat("This is a line of content for testing.\n", 500)

	_, err = store.Store(ctx, key, content, DefaultStoreOptions())
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify we can retrieve content
	result, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if result.Content != content {
		t.Error("content mismatch")
	}

	// Test persistence by creating new store with same database
	store.Close()

	// Reopen database and store
	db2, err := OpenSqlite(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer db2.Close()

	store2, err := NewResultStore(db2)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer store2.Close()

	// Should be able to retrieve from reopened store (content persisted in SQLite)
	result2, err := store2.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get from reopened store failed: %v", err)
	}
	if result2 == nil {
		t.Fatal("expected result from reopened store")
		return
	}
	if result2.Content != content {
		t.Error("content mismatch from reopened store")
	}
}

func TestResultStoreSummaryGeneration(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()
	key := ResultKey{SessionID: "test", Key: "file.txt"}

	// Long content with many lines
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "This is line number " + string(rune('A'+i))
	}
	content := strings.Join(lines, "\n")

	opts := StoreOptions{
		SummaryLines:  3,
		SummaryLength: 100,
	}

	meta, err := store.Store(ctx, key, content, opts)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Summary should be limited
	summaryLines := strings.Count(meta.Summary, "\n") + 1
	if summaryLines > 3 {
		t.Errorf("expected at most 3 lines in summary, got %d", summaryLines)
	}
}

func TestResultStoreList(t *testing.T) {
	store := NewInMemoryResultStore()
	defer store.Close()

	ctx := context.Background()

	_, _ = store.Store(ctx, ResultKey{SessionID: "test", Key: "a.txt"}, "content a", DefaultStoreOptions())
	_, _ = store.Store(ctx, ResultKey{SessionID: "test", Key: "b.txt"}, "content b", DefaultStoreOptions())
	_, _ = store.Store(ctx, ResultKey{SessionID: "test", Key: "c.txt"}, "content c", DefaultStoreOptions())

	// Test without pagination
	list, _ := store.List(ctx, "test", QueryOptions{})
	if len(list) != 3 {
		t.Errorf("expected 3 results, got %d", len(list))
	}

	// Test with limit
	list, _ = store.List(ctx, "test", QueryOptions{Limit: 2})
	if len(list) != 2 {
		t.Errorf("expected 2 results with limit, got %d", len(list))
	}

	// Test with offset
	list, _ = store.List(ctx, "test", QueryOptions{Offset: 1, Limit: 2})
	if len(list) != 2 {
		t.Errorf("expected 2 results with offset, got %d", len(list))
	}
}
