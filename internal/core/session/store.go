// Package session owns conversation persistence: a tiny SQLite store so chat
// sessions survive restarts (crush-style). Pure-Go driver, no CGO.
package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Message is one turn in a conversation.
type Message struct {
	Role    string // user | assistant | tool
	Content string
	At      time.Time
}

// SessionMeta is a session row for listings (no message bodies).
type SessionMeta struct {
	ID        int64
	Title     string
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
	MsgCount  int
}

// Store is the SQLite-backed session store. A nil *Store is valid and
// no-ops every write — tabs keep working without persistence.
type Store struct {
	db *sql.DB
}

// defaultDBPath is ~/.local/share/charmulet/charmulet.db (XDG data home
// convention); env CHARMULET_DB overrides.
func defaultDBPath() (string, error) {
	if v := os.Getenv("CHARMULET_DB"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "charmulet", "charmulet.db"), nil
}

// Open connects to the default database, creating it if needed.
func Open() (*Store, error) {
	p, err := defaultDBPath()
	if err != nil {
		return nil, err
	}
	return OpenPath(p)
}

// OpenPath connects to an explicit database file (used by tests).
func OpenPath(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("session store dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open session store: %w", err)
	}
	// Single-writer pragmas: WAL for concurrent reads during streaming.
	for _, q := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			title      TEXT NOT NULL,
			model      TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL REFERENCES sessions(id),
			role       TEXT NOT NULL,
			content    TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`,
	} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("session store migrate: %w", err)
		}
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Create inserts a new session and returns its meta.
func (s *Store) Create(title, model string) (SessionMeta, error) {
	if s == nil || s.db == nil {
		return SessionMeta{}, nil
	}
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO sessions (title, model, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		title, model, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return SessionMeta{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return SessionMeta{}, err
	}
	return SessionMeta{ID: id, Title: title, Model: model, CreatedAt: now, UpdatedAt: now}, nil
}

// Append adds a message to a session and touches its updated_at.
func (s *Store) Append(sid int64, role, content string) error {
	if s == nil || s.db == nil || sid == 0 {
		return nil
	}
	if content == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`INSERT INTO messages (session_id, role, content, created_at) VALUES (?, ?, ?, ?)`,
		sid, role, content, now,
	); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, now, sid)
	return err
}

// List returns sessions newest-first with message counts.
func (s *Store) List() ([]SessionMeta, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT s.id, s.title, s.model, s.created_at, s.updated_at,
		       (SELECT COUNT(*) FROM messages m WHERE m.session_id = s.id)
		FROM sessions s
		ORDER BY s.updated_at DESC
		LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionMeta
	for rows.Next() {
		var m SessionMeta
		var created, updated string
		if err := rows.Scan(&m.ID, &m.Title, &m.Model, &created, &updated, &m.MsgCount); err != nil {
			return nil, err
		}
		if m.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
			return nil, err
		}
		if m.UpdatedAt, err = time.Parse(time.RFC3339, updated); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Messages loads the full transcript of a session, oldest-first.
func (s *Store) Messages(sid int64) ([]Message, error) {
	if s == nil || s.db == nil || sid == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT role, content, created_at FROM messages WHERE session_id = ? ORDER BY id`,
		sid,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var at string
		if err := rows.Scan(&m.Role, &m.Content, &at); err != nil {
			return nil, err
		}
		m.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, m)
	}
	return out, rows.Err()
}
