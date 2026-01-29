// Package storage provides SQLite conversation storage.
//
// Information Hiding:
// - SQLite connection management hidden behind interface
// - Schema and migration details encapsulated
// - Thread-safe via sql.DB's built-in connection pooling

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/richinex/davingo/llm"
)

// SqliteStorage implements ConversationStorage and MemoryStorage using SQLite.
// Stores conversation history and memories in a SQLite database file.
// Thread-safe: sql.DB handles connection pooling and concurrent access.
type SqliteStorage struct {
	db *sql.DB
}

// OpenSqlite opens or creates a SQLite database at the given path.
// Creates parent directories if they don't exist.
func OpenSqlite(path string) (*SqliteStorage, error) {
	// Create parent directory if needed
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	storage := &SqliteStorage{db: db}
	if err := storage.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return storage, nil
}

// NewSqliteInMemory creates an in-memory database (useful for testing).
func NewSqliteInMemory() (*SqliteStorage, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to create in-memory SQLite: %w", err)
	}

	storage := &SqliteStorage{db: db}
	if err := storage.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return storage, nil
}

// Close closes the database connection.
func (s *SqliteStorage) Close() error {
	return s.db.Close()
}

func (s *SqliteStorage) createSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			message_index INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE CASCADE,
			UNIQUE(session_id, message_index)
		);

		CREATE INDEX IF NOT EXISTS idx_messages_session
		ON messages(session_id, message_index);

		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			agent_id TEXT,
			memory_type TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			accessed_at INTEGER NOT NULL,
			access_count INTEGER DEFAULT 1,
			metadata TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_memories_session_type
		ON memories(session_id, memory_type, created_at DESC);

		CREATE TABLE IF NOT EXISTS results (
			session_id TEXT NOT NULL,
			key TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			content TEXT NOT NULL,
			summary TEXT NOT NULL,
			line_count INTEGER NOT NULL,
			byte_size INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			accessed_at INTEGER NOT NULL,
			access_count INTEGER DEFAULT 1,
			PRIMARY KEY (session_id, key)
		);

		CREATE INDEX IF NOT EXISTS idx_results_session
		ON results(session_id);

		CREATE INDEX IF NOT EXISTS idx_results_hash
		ON results(content_hash);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	return nil
}

func (s *SqliteStorage) ensureSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO sessions (session_id) VALUES (?)",
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("failed to ensure session: %w", err)
	}
	return nil
}

// Save saves conversation history for a session.
func (s *SqliteStorage) Save(ctx context.Context, sessionID string, history []llm.ChatMessage) error {
	if err := s.ensureSession(ctx, sessionID); err != nil {
		return err
	}

	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	// defer tx.Rollback() is safe even after Commit() - it becomes a no-op
	defer func() { _ = tx.Rollback() }()

	// Clear existing messages for this session
	_, err = tx.ExecContext(ctx, "DELETE FROM messages WHERE session_id = ?", sessionID)
	if err != nil {
		return fmt.Errorf("failed to clear old messages: %w", err)
	}

	// Insert all messages
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO messages (session_id, message_index, role, content) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	for i, msg := range history {
		_, err = stmt.ExecContext(ctx, sessionID, i, msg.Role, msg.Content)
		if err != nil {
			return fmt.Errorf("failed to insert message: %w", err)
		}
	}

	// Update session timestamp
	_, err = tx.ExecContext(ctx,
		"UPDATE sessions SET updated_at = datetime('now') WHERE session_id = ?",
		sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session timestamp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Load loads conversation history for a session.
// Returns empty slice if session doesn't exist.
func (s *SqliteStorage) Load(ctx context.Context, sessionID string) ([]llm.ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT role, content FROM messages WHERE session_id = ? ORDER BY message_index ASC",
		sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	messages := []llm.ChatMessage{} // Start with empty slice, not nil
	for rows.Next() {
		var msg llm.ChatMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// Delete deletes conversation history for a session.
func (s *SqliteStorage) Delete(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM sessions WHERE session_id = ?",
		sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// ListSessions lists all session IDs.
func (s *SqliteStorage) ListSessions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT session_id FROM sessions ORDER BY updated_at DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	sessions := []string{} // Start with empty slice, not nil
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, sessionID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// Exists checks if a session exists.
func (s *SqliteStorage) Exists(ctx context.Context, sessionID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE session_id = ?",
		sessionID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check session existence: %w", err)
	}

	return count > 0, nil
}

// MemoryStorage implementation

// StoreMemory stores a memory entry.
func (s *SqliteStorage) StoreMemory(ctx context.Context, entry MemoryEntry) error {
	if err := s.ensureSession(ctx, entry.SessionID); err != nil {
		return err
	}

	// Convert empty strings to NULL for optional fields
	var agentID, metadata interface{}
	if entry.AgentID != "" {
		agentID = entry.AgentID
	}
	if entry.Metadata != "" {
		metadata = entry.Metadata
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO memories
		(id, session_id, agent_id, memory_type, content, created_at, accessed_at, access_count, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID,
		entry.SessionID,
		agentID,
		entry.Type.String(),
		entry.Content,
		entry.CreatedAt,
		entry.AccessedAt,
		entry.AccessCount,
		metadata,
	)
	if err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	return nil
}

// QueryMemories queries memories with optional filters.
func (s *SqliteStorage) QueryMemories(ctx context.Context, sessionID string, memoryType *MemoryType, limit int) ([]MemoryEntry, error) {
	var rows *sql.Rows
	var err error

	if memoryType != nil {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, session_id, agent_id, memory_type, content, created_at, accessed_at, access_count, metadata
			FROM memories
			WHERE session_id = ? AND memory_type = ?
			ORDER BY created_at DESC
			LIMIT ?`,
			sessionID, memoryType.String(), limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, session_id, agent_id, memory_type, content, created_at, accessed_at, access_count, metadata
			FROM memories
			WHERE session_id = ?
			ORDER BY created_at DESC
			LIMIT ?`,
			sessionID, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query memories: %w", err)
	}
	defer rows.Close()

	memories := []MemoryEntry{} // Start with empty slice, not nil
	for rows.Next() {
		entry, err := s.scanMemoryRow(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating memories: %w", err)
	}

	return memories, nil
}

// scanMemoryRow scans a single memory row from the result set.
func (s *SqliteStorage) scanMemoryRow(rows *sql.Rows) (MemoryEntry, error) {
	var entry MemoryEntry
	var memTypeStr string
	var agentID, metadata sql.NullString

	err := rows.Scan(
		&entry.ID,
		&entry.SessionID,
		&agentID,
		&memTypeStr,
		&entry.Content,
		&entry.CreatedAt,
		&entry.AccessedAt,
		&entry.AccessCount,
		&metadata,
	)
	if err != nil {
		return MemoryEntry{}, fmt.Errorf("failed to scan memory: %w", err)
	}

	if agentID.Valid {
		entry.AgentID = agentID.String
	}
	if metadata.Valid {
		entry.Metadata = metadata.String
	}

	memType, err := ParseMemoryType(memTypeStr)
	if err != nil {
		// Invalid memory type in database indicates data corruption or schema mismatch.
		// Return error rather than silently defaulting.
		return MemoryEntry{}, fmt.Errorf("invalid memory type %q in database: %w", memTypeStr, err)
	}
	entry.Type = memType

	return entry, nil
}

// GetRecentMemories gets recent memories across all types.
func (s *SqliteStorage) GetRecentMemories(ctx context.Context, sessionID string, limit int) ([]MemoryEntry, error) {
	return s.QueryMemories(ctx, sessionID, nil, limit)
}

// GetMemory gets a specific memory by ID and updates access tracking.
// Returns nil, nil if not found.
func (s *SqliteStorage) GetMemory(ctx context.Context, id string) (*MemoryEntry, error) {
	var entry MemoryEntry
	var memTypeStr string
	var agentID, metadata sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, agent_id, memory_type, content, created_at, accessed_at, access_count, metadata
		FROM memories WHERE id = ?`,
		id).Scan(
		&entry.ID,
		&entry.SessionID,
		&agentID,
		&memTypeStr,
		&entry.Content,
		&entry.CreatedAt,
		&entry.AccessedAt,
		&entry.AccessCount,
		&metadata,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}

	// Update access tracking
	now := time.Now().Unix()
	_, updateErr := s.db.ExecContext(ctx,
		"UPDATE memories SET accessed_at = ?, access_count = access_count + 1 WHERE id = ?",
		now, id)
	if updateErr != nil {
		// Access tracking failed - return error since state would be inconsistent
		return nil, fmt.Errorf("failed to update access tracking: %w", updateErr)
	}

	// Update the entry with new access info (only after successful DB update)
	entry.AccessedAt = now
	entry.AccessCount++

	if agentID.Valid {
		entry.AgentID = agentID.String
	}
	if metadata.Valid {
		entry.Metadata = metadata.String
	}

	memType, err := ParseMemoryType(memTypeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid memory type %q in database: %w", memTypeStr, err)
	}
	entry.Type = memType

	return &entry, nil
}

// DeleteMemory deletes a specific memory.
func (s *SqliteStorage) DeleteMemory(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM memories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}
	return nil
}

// DeleteSessionMemories deletes all memories for a session.
func (s *SqliteStorage) DeleteSessionMemories(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM memories WHERE session_id = ?", sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session memories: %w", err)
	}
	return nil
}

// ContentStorage implementation

// StoreResult stores a content result.
func (s *SqliteStorage) StoreResult(ctx context.Context, result ContentResult) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO results
		(session_id, key, content_hash, content, summary, line_count, byte_size, created_at, accessed_at, access_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.SessionID,
		result.Key,
		result.ContentHash,
		result.Content,
		result.Summary,
		result.LineCount,
		result.ByteSize,
		result.CreatedAt,
		result.AccessedAt,
		result.AccessCount,
	)
	if err != nil {
		return fmt.Errorf("failed to store result: %w", err)
	}
	return nil
}

// LoadAllResults loads all results from storage.
func (s *SqliteStorage) LoadAllResults(ctx context.Context) ([]ContentResult, error) {
	return s.queryResults(ctx, `
		SELECT session_id, key, content_hash, content, summary, line_count, byte_size, created_at, accessed_at, access_count
		FROM results
		ORDER BY accessed_at DESC`)
}

// LoadResultsBySession loads results for a specific session.
func (s *SqliteStorage) LoadResultsBySession(ctx context.Context, sessionID string) ([]ContentResult, error) {
	return s.queryResults(ctx, `
		SELECT session_id, key, content_hash, content, summary, line_count, byte_size, created_at, accessed_at, access_count
		FROM results
		WHERE session_id = ?
		ORDER BY accessed_at DESC`, sessionID)
}

// queryResults executes a query and scans results into ContentResult slice.
func (s *SqliteStorage) queryResults(ctx context.Context, query string, args ...interface{}) ([]ContentResult, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var results []ContentResult
	for rows.Next() {
		var r ContentResult
		err := rows.Scan(
			&r.SessionID,
			&r.Key,
			&r.ContentHash,
			&r.Content,
			&r.Summary,
			&r.LineCount,
			&r.ByteSize,
			&r.CreatedAt,
			&r.AccessedAt,
			&r.AccessCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iteration failed: %w", err)
	}

	return results, nil
}

// UpdateResultAccess updates access timestamp and count for a result.
func (s *SqliteStorage) UpdateResultAccess(ctx context.Context, sessionID, key string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE results SET accessed_at = ?, access_count = access_count + 1 WHERE session_id = ? AND key = ?",
		time.Now().Unix(), sessionID, key)
	if err != nil {
		return fmt.Errorf("update access failed: %w", err)
	}
	return nil
}

// DeleteResult removes a specific result.
func (s *SqliteStorage) DeleteResult(ctx context.Context, sessionID, key string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM results WHERE session_id = ? AND key = ?", sessionID, key)
	if err != nil {
		return fmt.Errorf("failed to delete result: %w", err)
	}
	return nil
}

// DeleteSessionResults removes all results for a session.
func (s *SqliteStorage) DeleteSessionResults(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM results WHERE session_id = ?", sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session results: %w", err)
	}
	return nil
}

// Verify SqliteStorage implements all interfaces
var _ ConversationStorage = (*SqliteStorage)(nil)
var _ MemoryStorage = (*SqliteStorage)(nil)
var _ ContentStorage = (*SqliteStorage)(nil)
