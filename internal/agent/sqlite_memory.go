package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	_ "modernc.org/sqlite"
)

const sqliteSessionSchema = `
CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_key TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	metadata_json TEXT,
	tool_call_id TEXT,
	tool_calls_json TEXT,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_key, id);
CREATE TABLE IF NOT EXISTS session_heads (
	stable_key TEXT PRIMARY KEY,
	active_key TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS session_lineage (
	stable_key TEXT NOT NULL,
	backing_key TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (stable_key, backing_key)
);
CREATE INDEX IF NOT EXISTS idx_session_lineage_stable_key ON session_lineage(stable_key, backing_key);
`

// SQLiteSessionMemory stores one session's conversation in a shared SQLite DB.
type SQLiteSessionMemory struct {
	mu         sync.RWMutex
	db         *sql.DB
	sessionKey string
	messages   []interfaces.Message
}

// NewSQLiteSessionMemory opens dbPath and returns memory scoped to sessionKey.
func NewSQLiteSessionMemory(ctx context.Context, dbPath, sessionKey string) (*SQLiteSessionMemory, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create sessions directory: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sessions database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	mem := &SQLiteSessionMemory{db: db, sessionKey: sessionKey}
	if err := mem.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := mem.load(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return mem, nil
}

func (m *SQLiteSessionMemory) init(ctx context.Context) error {
	if _, err := m.db.ExecContext(ctx, sqliteSessionSchema); err != nil {
		return fmt.Errorf("initialize sessions database: %w", err)
	}
	return nil
}

func (m *SQLiteSessionMemory) load(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, `
SELECT role, content, metadata_json, tool_call_id, tool_calls_json
FROM messages
WHERE session_key = ?
ORDER BY id`, m.sessionKey)
	if err != nil {
		return fmt.Errorf("load session messages: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var messages []interfaces.Message
	for rows.Next() {
		var role, content, toolCallID string
		var metadataJSON, toolCallsJSON sql.NullString
		if err := rows.Scan(&role, &content, &metadataJSON, &toolCallID, &toolCallsJSON); err != nil {
			return fmt.Errorf("scan session message: %w", err)
		}
		msg, err := messageFromSQLite(role, content, metadataJSON.String, toolCallID, toolCallsJSON.String)
		if err != nil {
			return err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load session messages: %w", err)
	}

	m.mu.Lock()
	m.messages = messages
	m.mu.Unlock()
	return nil
}

// AddMessage appends a message to this session.
func (m *SQLiteSessionMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	metadataJSON, toolCallsJSON, err := messageJSON(message)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	_, err = m.db.ExecContext(ctx, `
INSERT INTO messages (session_key, role, content, metadata_json, tool_call_id, tool_calls_json)
VALUES (?, ?, ?, ?, ?, ?)`,
		m.sessionKey,
		string(message.Role),
		message.Content,
		metadataJSON,
		message.ToolCallID,
		toolCallsJSON,
	)
	if err != nil {
		return fmt.Errorf("append session message: %w", err)
	}
	m.messages = append(m.messages, message)
	return nil
}

// GetMessages returns all messages or filters by supported options.
func (m *SQLiteSessionMemory) GetMessages(_ context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return filterMessages(m.messages, options...), nil
}

// Clear removes all messages from this session.
func (m *SQLiteSessionMemory) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := clearSQLiteSession(ctx, m.db, m.sessionKey); err != nil {
		return err
	}
	m.messages = m.messages[:0]
	return nil
}

// Close closes the underlying SQLite handle.
func (m *SQLiteSessionMemory) Close() error {
	return m.db.Close()
}

func clearSQLiteSession(ctx context.Context, db *sql.DB, sessionKey string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM messages WHERE session_key = ?`, sessionKey); err != nil {
		return fmt.Errorf("clear session messages: %w", err)
	}
	return nil
}

func resolveActiveSessionKey(ctx context.Context, db *sql.DB, stableKey string) (string, error) {
	var activeKey string
	err := db.QueryRowContext(ctx, `SELECT active_key FROM session_heads WHERE stable_key = ?`, stableKey).Scan(&activeKey)
	if err == nil {
		return activeKey, nil
	}
	if err == sql.ErrNoRows {
		return stableKey, nil
	}
	return "", fmt.Errorf("resolve active session key: %w", err)
}

func setActiveSessionKey(ctx context.Context, db *sql.DB, stableKey, activeKey string) error {
	if err := ensureSessionLineage(ctx, db, stableKey, stableKey); err != nil {
		return err
	}
	if err := ensureSessionLineage(ctx, db, stableKey, activeKey); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO session_heads (stable_key, active_key)
VALUES (?, ?)
ON CONFLICT(stable_key) DO UPDATE SET active_key = excluded.active_key`,
		stableKey, activeKey,
	); err != nil {
		return fmt.Errorf("set active session key: %w", err)
	}
	return nil
}

func ensureSessionLineage(ctx context.Context, db *sql.DB, stableKey, backingKey string) error {
	if _, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO session_lineage (stable_key, backing_key)
VALUES (?, ?)`,
		stableKey, backingKey,
	); err != nil {
		return fmt.Errorf("ensure session lineage: %w", err)
	}
	return nil
}

func listSessionLineageKeys(ctx context.Context, db *sql.DB, stableKey string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT backing_key
FROM session_lineage
WHERE stable_key = ?
ORDER BY created_at, backing_key`, stableKey)
	if err != nil {
		return nil, fmt.Errorf("list session lineage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan session lineage: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list session lineage: %w", err)
	}
	if len(keys) == 0 {
		return []string{stableKey}, nil
	}
	return keys, nil
}

func clearSessionLineage(ctx context.Context, db *sql.DB, stableKey string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM session_heads WHERE stable_key = ?`, stableKey); err != nil {
		return fmt.Errorf("clear session heads: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM session_lineage WHERE stable_key = ?`, stableKey); err != nil {
		return fmt.Errorf("clear session lineage: %w", err)
	}
	return nil
}

func messageJSON(message interfaces.Message) (string, string, error) {
	metadata, err := json.Marshal(message.Metadata)
	if err != nil {
		return "", "", fmt.Errorf("marshal message metadata: %w", err)
	}
	toolCalls, err := json.Marshal(message.ToolCalls)
	if err != nil {
		return "", "", fmt.Errorf("marshal message tool calls: %w", err)
	}
	return string(metadata), string(toolCalls), nil
}

func messageFromSQLite(role, content, metadataJSON, toolCallID, toolCallsJSON string) (interfaces.Message, error) {
	msg := interfaces.Message{
		Role:       interfaces.MessageRole(role),
		Content:    content,
		ToolCallID: toolCallID,
	}
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &msg.Metadata); err != nil {
			return msg, fmt.Errorf("decode message metadata: %w", err)
		}
	}
	if toolCallsJSON != "" {
		if err := json.Unmarshal([]byte(toolCallsJSON), &msg.ToolCalls); err != nil {
			return msg, fmt.Errorf("decode message tool calls: %w", err)
		}
	}
	return msg, nil
}
