package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"github.com/AOSSIE-Org/ThruBox-Server/internal/model"
)

// SQLiteStore implements the Store interface using SQLite via mattn/go-sqlite3.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at the given path,
// enables WAL mode for better concurrent read performance, and creates
// the messages table if it doesn't exist.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Verify the connection is alive
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	// Create table and indexes
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// initSchema creates the messages table and indexes if they don't already exist.
func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id         TEXT PRIMARY KEY,
		to_addr    TEXT NOT NULL,
		from_addr  TEXT NOT NULL,
		payload    TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_messages_to_addr ON messages(to_addr);
	CREATE INDEX IF NOT EXISTS idx_messages_expires_at ON messages(expires_at);
	`
	_, err := db.Exec(schema)
	return err
}

// SaveMessage inserts a new message into the database.
func (s *SQLiteStore) SaveMessage(msg *model.Message) error {
	query := `INSERT INTO messages (id, to_addr, from_addr, payload, created_at, expires_at)
	           VALUES (?, ?, ?, ?, ?, ?)`

	// Store empty string for nil ExpiresAt (means "forever" — never auto-expires)
	expiresAt := ""
	if msg.ExpiresAt != nil {
		expiresAt = msg.ExpiresAt.UTC().Format(time.RFC3339)
	}

	_, err := s.db.Exec(query,
		msg.ID,
		msg.To,
		msg.From,
		msg.Payload,
		msg.CreatedAt.UTC().Format(time.RFC3339),
		expiresAt,
	)
	if err != nil {
		return fmt.Errorf("saving message: %w", err)
	}
	return nil
}

// GetMessagesByAddress returns all messages addressed to the given wallet address.
// Returns an empty slice (not nil) if no messages are found.
func (s *SQLiteStore) GetMessagesByAddress(address string) ([]*model.Message, error) {
	query := `SELECT id, to_addr, from_addr, payload, created_at, expires_at
	           FROM messages WHERE to_addr = ? ORDER BY created_at ASC`

	rows, err := s.db.Query(query, address)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	messages := make([]*model.Message, 0)
	for rows.Next() {
		msg := &model.Message{}
		var createdAt, expiresAt string

		if err := rows.Scan(&msg.ID, &msg.To, &msg.From, &msg.Payload, &createdAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("scanning message row: %w", err)
		}

		msg.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}

		// Empty expires_at means "forever" — leave ExpiresAt as nil
		if expiresAt != "" {
			t, err := time.Parse(time.RFC3339, expiresAt)
			if err != nil {
				return nil, fmt.Errorf("parsing expires_at: %w", err)
			}
			msg.ExpiresAt = &t
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message rows: %w", err)
	}

	return messages, nil
}

// DeleteMessage removes a message by its ID.
// Returns nil even if the message does not exist (idempotent).
func (s *SQLiteStore) DeleteMessage(id string) error {
	query := `DELETE FROM messages WHERE id = ?`

	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("deleting message: %w", err)
	}
	return nil
}

// PurgeExpired deletes all messages whose ExpiresAt is in the past.
// Messages with empty expires_at ("forever") are never purged — only manual delete removes them.
// Returns the number of deleted messages.
func (s *SQLiteStore) PurgeExpired() (int64, error) {
	// Only purge messages that have an expiry set (non-empty) and are past their expiry
	query := `DELETE FROM messages WHERE expires_at != '' AND expires_at < ?`

	result, err := s.db.Exec(query, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("purging expired messages: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting purge count: %w", err)
	}

	return count, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
