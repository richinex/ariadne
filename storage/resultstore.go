// ResultStore implementation with SQLite persistence.
//
// Architecture:
// - In-memory: Trie for key lookup, SuffixArray for search, map for O(1) by hash
// - SQLite: ContentStorage for persistence (content + metadata)
package storage

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/richinex/ariadne/model"
	"github.com/richinex/ariadne/internal/dsa"
)

// ResultStoreInterface is the interface for result storage operations.
type ResultStoreInterface interface {
	// Store saves content with the given key.
	// Returns metadata (for lightweight passing to supervisor).
	Store(ctx context.Context, key ResultKey, content string, opts StoreOptions) (ResultMetadata, error)

	// Get retrieves full content by key.
	Get(ctx context.Context, key ResultKey) (*Result, error)

	// GetMetadata retrieves just metadata (no content load).
	GetMetadata(ctx context.Context, key ResultKey) (*ResultMetadata, error)

	// GetLines retrieves a specific line range from stored content.
	GetLines(ctx context.Context, key ResultKey, lineRange LineRange) (string, error)

	// Search finds pattern across all stored content in session.
	Search(ctx context.Context, sessionID string, pattern string, limit int) ([]SearchMatch, error)

	// GetByPrefix returns all results with keys starting with prefix.
	GetByPrefix(ctx context.Context, sessionID string, prefix string) ([]ResultMetadata, error)

	// Delete removes a stored result.
	Delete(ctx context.Context, key ResultKey) error

	// DeleteSession removes all results for a session.
	DeleteSession(ctx context.Context, sessionID string) error

	// List returns all result metadata for a session.
	List(ctx context.Context, sessionID string, opts QueryOptions) ([]ResultMetadata, error)

	// Close releases resources.
	Close() error
}

// ResultStore implements ResultStoreInterface with in-memory indexing and file storage.
// ResultStore implements ResultStoreInterface with in-memory indexing and SQLite persistence.
// All content is stored in SQLite for durability and in memory for fast access.
type ResultStore struct {
	mu sync.RWMutex

	// In-memory indexes
	keyIndex     *dsa.Trie[ResultKey] // Key -> ResultKey for prefix search
	keyToHash    map[string]string    // compositeKey -> contentHash for O(1) lookup
	contentIndex map[string]*Result   // ContentHash -> Result for dedup
	sessionIndex map[string][]string  // SessionID -> list of keys

	// Lazy-built suffix array for search
	searchIndex     *dsa.SuffixArray
	searchContent   string           // Concatenated content for search
	searchPositions []searchPosition // Map positions back to results
	searchDirty     bool             // Need to rebuild search index

	// SQLite storage for persistence (optional)
	contentDB ContentStorage
}

// searchPosition maps suffix array positions to results.
type searchPosition struct {
	key   ResultKey
	start int // Start position in concatenated content
	end   int // End position
}

// NewResultStore creates a result store with optional SQLite persistence.
// contentDB provides persistence across sessions (optional, nil for memory-only).
//
// Ownership: ResultStore takes ownership of contentDB. Calling Close() will
// close the contentDB. If NewResultStore fails, the caller retains ownership
// of contentDB and must close it.
func NewResultStore(contentDB ContentStorage) (*ResultStore, error) {
	store := &ResultStore{
		keyIndex:     dsa.NewTrie[ResultKey](),
		keyToHash:    make(map[string]string),
		contentIndex: make(map[string]*Result),
		sessionIndex: make(map[string][]string),
		searchDirty:  true,
		contentDB:    contentDB,
	}

	// Load existing content from SQLite
	if contentDB != nil {
		if err := store.loadFromContentStorage(); err != nil {
			return nil, fmt.Errorf("failed to load from SQLite: %w", err)
		}
	}

	return store, nil
}

// NewInMemoryResultStore creates a result store without persistence.
func NewInMemoryResultStore() *ResultStore {
	return &ResultStore{
		keyIndex:     dsa.NewTrie[ResultKey](),
		keyToHash:    make(map[string]string),
		contentIndex: make(map[string]*Result),
		sessionIndex: make(map[string][]string),
		searchDirty:  true,
	}
}

// Store saves content with the given key.
// Content is stored in memory for fast access and persisted to SQLite.
func (s *ResultStore) Store(ctx context.Context, key ResultKey, content string, opts StoreOptions) (ResultMetadata, error) {
	// Apply defaults for zero values
	if opts.SummaryLength <= 0 {
		opts.SummaryLength = 200
	}
	if opts.SummaryLines <= 0 {
		opts.SummaryLines = 5
	}

	// Compute content hash for deduplication
	hash := computeContentHash(content)
	compositeKey := composeResultKey(key)

	now := time.Now()
	meta := ResultMetadata{
		Key:         key,
		ContentHash: hash,
		Summary:     generateResultSummary(content, opts),
		LineCount:   countResultLines(content),
		ByteSize:    len(content),
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 1,
	}

	// Update in-memory indexes under lock
	s.mu.Lock()
	if existing, ok := s.contentIndex[hash]; ok {
		// Content already exists - just add new key mapping
		existing.Metadata.AccessedAt = now
		existing.Metadata.AccessCount++
		meta = existing.Metadata
		meta.Key = key // Return with requested key
	} else {
		result := &Result{
			Metadata: meta,
			Content:  content,
		}
		s.contentIndex[hash] = result
	}
	s.keyIndex.Insert(compositeKey, key)
	s.keyToHash[compositeKey] = hash
	s.updateSessionIndex(key)
	s.searchDirty = true
	s.mu.Unlock()

	// Persist to SQLite if available (outside lock)
	if s.contentDB != nil {
		result := ContentResult{
			SessionID:   meta.Key.SessionID,
			Key:         meta.Key.Key,
			ContentHash: meta.ContentHash,
			Content:     content, // Store actual content in SQLite
			Summary:     meta.Summary,
			LineCount:   meta.LineCount,
			ByteSize:    meta.ByteSize,
			CreatedAt:   meta.CreatedAt.Unix(),
			AccessedAt:  meta.AccessedAt.Unix(),
			AccessCount: meta.AccessCount,
		}
		if err := s.contentDB.StoreResult(ctx, result); err != nil {
			return ResultMetadata{}, fmt.Errorf("failed to persist to SQLite: %w", err)
		}
	}

	return meta, nil
}

// Get retrieves full content by key.
// Content is always in memory (loaded from SQLite on startup).
func (s *ResultStore) Get(ctx context.Context, key ResultKey) (*Result, error) {
	compositeKey := composeResultKey(key)

	s.mu.RLock()
	hash, found := s.keyToHash[compositeKey]
	if !found {
		s.mu.RUnlock()
		return nil, nil
	}

	result, ok := s.contentIndex[hash]
	if !ok {
		s.mu.RUnlock()
		return nil, nil
	}

	metadata := result.Metadata
	metadata.Key = key // Return with requested key
	content := result.Content
	s.mu.RUnlock()

	// Update access tracking under lock
	now := time.Now()
	s.mu.Lock()
	if r, ok := s.contentIndex[hash]; ok {
		r.Metadata.AccessedAt = now
		r.Metadata.AccessCount++
		metadata.AccessedAt = now
		metadata.AccessCount = r.Metadata.AccessCount
	}
	s.mu.Unlock()

	// Update database access tracking (outside lock)
	if s.contentDB != nil {
		if err := s.contentDB.UpdateResultAccess(ctx, key.SessionID, key.Key); err != nil {
			fmt.Fprintf(os.Stderr, "storage: failed to update access tracking: %v\n", err)
		}
	}

	return &Result{
		Metadata: metadata,
		Content:  content,
	}, nil
}

// GetMetadata retrieves just metadata (no content load).
func (s *ResultStore) GetMetadata(ctx context.Context, key ResultKey) (*ResultMetadata, error) {
	compositeKey := composeResultKey(key)

	s.mu.RLock()
	hash, found := s.keyToHash[compositeKey]
	if !found {
		s.mu.RUnlock()
		return nil, nil
	}

	result, ok := s.contentIndex[hash]
	if !ok {
		s.mu.RUnlock()
		return nil, nil
	}

	meta := result.Metadata
	meta.Key = key // Return with requested key
	s.mu.RUnlock()

	return &meta, nil
}

// GetLines retrieves a specific line range from stored content.
func (s *ResultStore) GetLines(ctx context.Context, key ResultKey, lineRange LineRange) (string, error) {
	result, err := s.Get(ctx, key)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}

	lines := strings.Split(result.Content, "\n")
	start := lineRange.Start - 1 // Convert to 0-indexed
	end := lineRange.End

	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return "", nil
	}

	return strings.Join(lines[start:end], "\n"), nil
}

// Search finds pattern across all stored content in session.
func (s *ResultStore) Search(ctx context.Context, sessionID string, pattern string, limit int) ([]SearchMatch, error) {
	// Check if rebuild needed
	s.mu.RLock()
	needsRebuild := s.searchDirty
	s.mu.RUnlock()

	// Rebuild outside lock (rebuildSearchIndex manages its own locking)
	if needsRebuild {
		if err := s.rebuildSearchIndexForSession(sessionID); err != nil {
			return nil, err
		}
	}

	// Read search index under lock
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.searchIndex == nil || len(s.searchContent) == 0 {
		return nil, nil
	}

	// Search using suffix array
	positions := s.searchIndex.Search(pattern)

	var matches []SearchMatch
	for _, pos := range positions {
		if limit > 0 && len(matches) >= limit {
			break
		}

		// Find which result this position belongs to
		for _, sp := range s.searchPositions {
			if sp.key.SessionID != sessionID {
				continue
			}
			if pos >= sp.start && pos < sp.end {
				// Calculate line number
				contentBefore := s.searchContent[:pos]
				lineNum := strings.Count(contentBefore, "\n") + 1

				// Extract context line
				lineStart := strings.LastIndex(s.searchContent[:pos], "\n") + 1
				lineEnd := strings.Index(s.searchContent[pos:], "\n")
				if lineEnd == -1 {
					lineEnd = len(s.searchContent)
				} else {
					lineEnd += pos
				}
				context := s.searchContent[lineStart:lineEnd]

				matches = append(matches, SearchMatch{
					Key:      sp.key,
					Position: pos - sp.start, // Position within the result
					Line:     lineNum,
					Context:  context,
				})
				break
			}
		}
	}

	return matches, nil
}

// GetByPrefix returns all results with keys starting with prefix.
func (s *ResultStore) GetByPrefix(ctx context.Context, sessionID string, prefix string) ([]ResultMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	compositePrefix := sessionID + ":" + prefix
	keys := s.keyIndex.StartsWith(compositePrefix)

	results := make([]ResultMetadata, 0, len(keys))
	for _, compositeKey := range keys {
		// O(1) lookup via keyToHash -> contentIndex instead of O(N) scan
		hash, found := s.keyToHash[compositeKey]
		if !found {
			continue
		}

		result, ok := s.contentIndex[hash]
		if !ok {
			continue
		}

		// Get the actual key for this composite key
		rk, _ := s.keyIndex.Search(compositeKey)
		meta := result.Metadata
		meta.Key = rk // Return with the requested key
		results = append(results, meta)
	}

	return results, nil
}

// Delete removes a stored result.
func (s *ResultStore) Delete(ctx context.Context, key ResultKey) error {
	compositeKey := composeResultKey(key)

	// Get hash under lock
	s.mu.Lock()
	hash, found := s.keyToHash[compositeKey]
	if !found {
		s.mu.Unlock()
		return nil
	}

	// Remove from indexes
	delete(s.contentIndex, hash)
	delete(s.keyToHash, compositeKey)
	s.keyIndex.Delete(compositeKey)

	// Remove from session index
	if keys, ok := s.sessionIndex[key.SessionID]; ok {
		for i, k := range keys {
			if k == key.Key {
				s.sessionIndex[key.SessionID] = append(keys[:i], keys[i+1:]...)
				break
			}
		}
	}

	s.searchDirty = true
	s.mu.Unlock()

	// Delete from SQLite outside lock
	if s.contentDB != nil {
		if err := s.contentDB.DeleteResult(ctx, key.SessionID, key.Key); err != nil {
			return fmt.Errorf("failed to delete from SQLite: %w", err)
		}
	}

	return nil
}

// DeleteSession removes all results for a session.
func (s *ResultStore) DeleteSession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	keys, ok := s.sessionIndex[sessionID]
	if !ok {
		s.mu.Unlock()
		return nil
	}

	for _, key := range keys {
		rk := ResultKey{SessionID: sessionID, Key: key}
		compositeKey := composeResultKey(rk)

		if hash, found := s.keyToHash[compositeKey]; found {
			delete(s.contentIndex, hash)
			delete(s.keyToHash, compositeKey)
		}
		s.keyIndex.Delete(compositeKey)
	}

	delete(s.sessionIndex, sessionID)
	s.searchDirty = true
	s.mu.Unlock()

	// Delete from SQLite outside lock
	if s.contentDB != nil {
		if err := s.contentDB.DeleteSessionResults(ctx, sessionID); err != nil {
			return fmt.Errorf("failed to delete session from SQLite: %w", err)
		}
	}

	return nil
}

// List returns all result metadata for a session.
func (s *ResultStore) List(ctx context.Context, sessionID string, opts QueryOptions) ([]ResultMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []ResultMetadata
	for _, result := range s.contentIndex {
		if result.Metadata.Key.SessionID == sessionID {
			results = append(results, result.Metadata)
		}
	}

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		return []ResultMetadata{}, nil
	}

	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, nil
}

// Close releases resources including the ContentStorage.
func (s *ResultStore) Close() error {
	s.mu.Lock()
	// Clear in-memory state to help GC
	s.contentIndex = nil
	s.keyToHash = nil
	s.sessionIndex = nil
	s.searchIndex = nil
	s.searchContent = ""
	s.searchPositions = nil
	contentDB := s.contentDB
	s.contentDB = nil
	s.mu.Unlock()

	// Close the content database if we own it
	if contentDB != nil {
		if closer, ok := contentDB.(interface{ Close() error }); ok {
			return closer.Close()
		}
	}
	return nil
}

// Helper functions

// computeContentHash uses xxHash for fast, high-quality content hashing.
// xxHash is non-cryptographic but ideal for deduplication (10-30x faster than SHA256).
// See: https://github.com/cespare/xxhash
func computeContentHash(content string) string {
	h := xxhash.Sum64String(content)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h)
	return hex.EncodeToString(buf[:])
}

func generateResultSummary(content string, opts StoreOptions) string {
	lines := strings.Split(content, "\n")

	maxLines := opts.SummaryLines
	if maxLines <= 0 {
		maxLines = 5
	}
	maxLen := opts.SummaryLength
	if maxLen <= 0 {
		maxLen = 200
	}

	var summary strings.Builder
	lineCount := 0
	for _, line := range lines {
		if lineCount >= maxLines || summary.Len() >= maxLen {
			break
		}
		if summary.Len() > 0 {
			summary.WriteString("\n")
		}
		summary.WriteString(line)
		lineCount++
	}

	s := summary.String()
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}

func countResultLines(content string) int {
	if len(content) == 0 {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func composeResultKey(key ResultKey) string {
	return key.SessionID + ":" + key.Key
}

func (s *ResultStore) updateSessionIndex(key ResultKey) {
	keys := s.sessionIndex[key.SessionID]
	for _, k := range keys {
		if k == key.Key {
			return // Already in session index
		}
	}
	s.sessionIndex[key.SessionID] = append(keys, key.Key)
}

// rebuildSearchIndexForSession rebuilds the suffix array for searching.
func (s *ResultStore) rebuildSearchIndexForSession(sessionID string) error {
	// Collect content under read lock
	type indexItem struct {
		key     ResultKey
		content string
	}
	var items []indexItem

	s.mu.RLock()
	for _, result := range s.contentIndex {
		if result.Metadata.Key.SessionID != sessionID {
			continue
		}
		items = append(items, indexItem{
			key:     result.Metadata.Key,
			content: result.Content,
		})
	}
	s.mu.RUnlock()

	// Build suffix array (compute-intensive but no locks needed)
	var contentBuilder strings.Builder
	var positions []searchPosition

	for _, item := range items {
		start := contentBuilder.Len()
		contentBuilder.WriteString(item.content)
		contentBuilder.WriteString("\x00")
		positions = append(positions, searchPosition{
			key:   item.key,
			start: start,
			end:   contentBuilder.Len() - 1,
		})
	}

	searchContent := contentBuilder.String()
	var searchIndex *dsa.SuffixArray
	if len(searchContent) > 0 {
		searchIndex = dsa.BuildSuffixArray(searchContent)
	}

	// Update state under write lock
	s.mu.Lock()
	s.searchContent = searchContent
	s.searchIndex = searchIndex
	s.searchPositions = positions
	s.searchDirty = false
	s.mu.Unlock()

	return nil
}

func (s *ResultStore) loadFromContentStorage() error {
	if s.contentDB == nil {
		return nil
	}

	results, err := s.contentDB.LoadAllResults(context.Background())
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range results {
		key := ResultKey{SessionID: r.SessionID, Key: r.Key}
		meta := ResultMetadata{
			Key:         key,
			ContentHash: r.ContentHash,
			Summary:     r.Summary,
			LineCount:   r.LineCount,
			ByteSize:    r.ByteSize,
			CreatedAt:   time.Unix(r.CreatedAt, 0),
			AccessedAt:  time.Unix(r.AccessedAt, 0),
			AccessCount: r.AccessCount,
		}
		result := &Result{
			Metadata: meta,
			Content:  r.Content, // Content loaded from SQLite
		}
		s.contentIndex[meta.ContentHash] = result

		compositeKey := composeResultKey(meta.Key)
		s.keyIndex.Insert(compositeKey, meta.Key)
		s.keyToHash[compositeKey] = meta.ContentHash
		s.updateSessionIndex(meta.Key)
	}

	return nil
}

// StoreContent implements model.ContentStore for the RLM pattern.
// This allows tools to store content externally and return references.
// StoreContent implements model.ContentStore for the RLM pattern.
// This allows tools to store content and return references.
func (s *ResultStore) StoreContent(ctx context.Context, key model.ContentKey, content string) (model.StoredContent, error) {
	// Map ContentKey to ResultKey, using content type as session for isolation
	resultKey := ResultKey{
		SessionID: key.ContentType,
		Key:       key.Path,
	}

	meta, err := s.Store(ctx, resultKey, content, DefaultStoreOptions())
	if err != nil {
		return model.StoredContent{}, err
	}

	return model.StoredContent{
		Reference: key.Path,
		Lines:     meta.LineCount,
		Bytes:     meta.ByteSize,
		Preview:   meta.Summary,
	}, nil
}
