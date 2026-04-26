package session

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // registers "sqlite" driver
)

// Store is the interface for session persistence.
type Store interface {
	CreateSession(agentType string) (string, error)
	AddMessage(sessionID, role, content, toolName, toolCalls, reasoning string) error
	GetMessages(sessionID string) ([]Message, error)
	Search(query string, limit int) ([]SearchResult, error)
	ListSessions() ([]Info, error)
	Stats() Stats
	Close() error
}

// Info holds session metadata.
type Info struct {
	ID        string
	AgentType string
	StartTime time.Time
	EndTime   time.Time
	MsgCount  int
	TokenEst  int
}

// Message is a stored message.
type Message struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	ToolName  string
	ToolCalls string
	Reasoning string
	Timestamp time.Time
}

// SearchResult is a full-text search hit.
type SearchResult struct {
	SessionID string
	Role      string
	Content   string
	Timestamp time.Time
}

// Stats holds store statistics.
type Stats struct {
	TotalSessions int
	TotalMessages int
}

// SQLiteStore implements Store using SQLite + FTS5.
type SQLiteStore struct {
	db       *sql.DB
	path     string
	maxSessions int
}

// NewStore creates or opens a SQLite session store.
func NewStore(path string, maxSessions int) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer

	s := &SQLiteStore{
		db:          db,
		path:        path,
		maxSessions: maxSessions,
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) CreateSession(agentType string) (string, error) {
	id := uuid.New().String()
	now := time.Now()
	_, err := s.db.Exec(
		"INSERT INTO sessions (id, agent_type, start_time) VALUES (?, ?, ?)",
		id, agentType, now,
	)
	if err != nil {
		return "", err
	}

	// Prune old sessions if over limit
	if s.maxSessions > 0 {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE id NOT IN (
			SELECT id FROM sessions ORDER BY start_time DESC LIMIT ?
		)`, s.maxSessions)
	}

	return id, nil
}

func (s *SQLiteStore) AddMessage(sessionID, role, content, toolName, toolCalls, reasoning string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`INSERT INTO messages (session_id, role, content, tool_name, tool_calls, reasoning, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionID, role, content, toolName, toolCalls, reasoning, now,
	)
	if err != nil {
		return err
	}

	// Update session msg_count
	_, _ = s.db.Exec(
		"UPDATE sessions SET msg_count = msg_count + 1, end_time = ? WHERE id = ?",
		now, sessionID,
	)
	return nil
}

func (s *SQLiteStore) GetMessages(sessionID string) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, role, content, COALESCE(tool_name,''), COALESCE(tool_calls,''), COALESCE(reasoning,''), timestamp
		 FROM messages WHERE session_id = ? ORDER BY id`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolName, &m.ToolCalls, &m.Reasoning, &m.Timestamp); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s *SQLiteStore) ListSessions() ([]Info, error) {
	rows, err := s.db.Query(
		"SELECT id, agent_type, start_time, COALESCE(end_time,0), msg_count, token_est FROM sessions ORDER BY start_time DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Info
	for rows.Next() {
		var si Info
		var et int64
		if err := rows.Scan(&si.ID, &si.AgentType, &si.StartTime, &et, &si.MsgCount, &si.TokenEst); err != nil {
			return nil, err
		}
		if et > 0 {
			si.EndTime = time.Unix(et, 0)
		}
		sessions = append(sessions, si)
	}
	return sessions, nil
}

func (s *SQLiteStore) Stats() Stats {
	var totalSessions, totalMessages int
	s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&totalSessions)
	s.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&totalMessages)
	return Stats{TotalSessions: totalSessions, TotalMessages: totalMessages}
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}