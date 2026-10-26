package store

import (
	"context"
	"database/sql"
	"errors"
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
//
// Transactions start with BEGIN IMMEDIATE (_txlock=immediate) so a write
// transaction takes the write lock up front. Two concurrent SaveMessage calls
// then run one after the other instead of both reading the same state.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000&_txlock=immediate")
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

// initSchema creates the tables and indexes if they don't already exist, and
// upgrades databases created before messages had sequence numbers.
func initSchema(db *sql.DB) error {
	tables := `
	CREATE TABLE IF NOT EXISTS messages (
		id         TEXT PRIMARY KEY,
		to_addr    TEXT NOT NULL,
		from_addr  TEXT NOT NULL,
		payload    TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		expires_at DATETIME,
		seq        INTEGER NOT NULL DEFAULT 0
	);

	-- The last sequence number handed out per mailbox. Kept apart from the
	-- messages so that purging the newest (expired) message cannot cause its
	-- number to be handed out again.
	CREATE TABLE IF NOT EXISTS mailboxes (
		to_addr  TEXT PRIMARY KEY,
		last_seq INTEGER NOT NULL
	);
	`
	if _, err := db.Exec(tables); err != nil {
		return err
	}

	if err := migrateAddSeq(db); err != nil {
		return fmt.Errorf("adding sequence numbers: %w", err)
	}

	indexes := `
	CREATE INDEX IF NOT EXISTS idx_messages_to_addr ON messages(to_addr);
	CREATE INDEX IF NOT EXISTS idx_messages_expires_at ON messages(expires_at);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_to_addr_seq ON messages(to_addr, seq);
	`
	_, err := db.Exec(indexes)
	return err
}

// migrateAddSeq adds the seq column to a messages table created before it
// existed. Existing messages are numbered 1, 2, 3, ... per mailbox in the
// order they arrived (created_at, then rowid: created_at only has one-second
// precision and old IDs are random UUIDs), and each mailbox's counter starts
// after its last one.
// It does nothing if the column is already there.
func migrateAddSeq(db *sql.DB) error {
	var hasSeq bool
	if err := db.QueryRow(
		`SELECT COUNT(*) > 0 FROM pragma_table_info('messages') WHERE name = 'seq'`,
	).Scan(&hasSeq); err != nil {
		return err
	}
	if hasSeq {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	steps := []string{
		`ALTER TABLE messages ADD COLUMN seq INTEGER NOT NULL DEFAULT 0`,
		`UPDATE messages SET seq = (
			SELECT n FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY to_addr ORDER BY created_at, rowid) AS n
				FROM messages
			) numbered
			WHERE numbered.id = messages.id
		)`,
		`INSERT OR REPLACE INTO mailboxes (to_addr, last_seq)
			SELECT to_addr, MAX(seq) FROM messages GROUP BY to_addr`,
	}
	for _, step := range steps {
		if _, err := tx.Exec(step); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// messageColumns is the column list scanMessage expects, in order.
const messageColumns = `id, seq, to_addr, from_addr, payload, created_at, expires_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanMessage reads one row selected with messageColumns.
func scanMessage(row rowScanner) (*model.Message, error) {
	msg := &model.Message{}
	var createdAt string
	var expiresAt sql.NullString

	if err := row.Scan(&msg.ID, &msg.Seq, &msg.To, &msg.From, &msg.Payload, &createdAt, &expiresAt); err != nil {
		return nil, err
	}

	var err error
	msg.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	// NULL expires_at means "forever" — leave ExpiresAt as nil
	if expiresAt.Valid {
		t, err := time.Parse(time.RFC3339, expiresAt.String)
		if err != nil {
			return nil, fmt.Errorf("parsing expires_at: %w", err)
		}
		msg.ExpiresAt = &t
	}

	return msg, nil
}

// SaveMessage inserts a new message into the database, assigning it the next
// sequence number in its recipient's mailbox. See Store.SaveMessage.
func (s *SQLiteStore) SaveMessage(ctx context.Context, msg *model.Message) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("saving message: %w", err)
	}
	defer tx.Rollback()

	// The ID is a hash of the content, so a retried send arrives with the ID
	// of the message it already stored. Hand that one back.
	existing, err := scanMessage(tx.QueryRowContext(ctx,
		`SELECT `+messageColumns+` FROM messages
		 WHERE id = ? AND (expires_at IS NULL OR expires_at >= ?)`,
		msg.ID, now,
	))
	switch {
	case err == nil:
		*msg = *existing
		return false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return false, fmt.Errorf("checking for existing message: %w", err)
	}

	// An expired copy the purge has not reached yet would block the insert.
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, msg.ID); err != nil {
		return false, fmt.Errorf("removing expired message: %w", err)
	}

	if err := tx.QueryRowContext(ctx,
		`INSERT INTO mailboxes (to_addr, last_seq) VALUES (?, 1)
		 ON CONFLICT (to_addr) DO UPDATE SET last_seq = last_seq + 1
		 RETURNING last_seq`,
		msg.To,
	).Scan(&msg.Seq); err != nil {
		return false, fmt.Errorf("assigning sequence number: %w", err)
	}

	// Store NULL for nil ExpiresAt (means "forever" — never auto-expires)
	var expiresAt sql.NullString
	if msg.ExpiresAt != nil {
		expiresAt = sql.NullString{
			String: msg.ExpiresAt.UTC().Format(time.RFC3339),
			Valid:  true,
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO messages (id, seq, to_addr, from_addr, payload, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.ID,
		msg.Seq,
		msg.To,
		msg.From,
		msg.Payload,
		msg.CreatedAt.UTC().Format(time.RFC3339),
		expiresAt,
	); err != nil {
		return false, fmt.Errorf("saving message: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("saving message: %w", err)
	}
	return true, nil
}

// GetMessagesByAddress returns the messages addressed to the given wallet
// address with a sequence number greater than after, oldest first. A limit of
// 0 returns all of them.
// Returns an empty slice (not nil) if no messages are found.
func (s *SQLiteStore) GetMessagesByAddress(ctx context.Context, address string, after int64, limit int) ([]*model.Message, error) {
	query := `SELECT ` + messageColumns + ` FROM messages
	           WHERE to_addr = ? AND seq > ? AND (expires_at IS NULL OR expires_at >= ?)
	           ORDER BY seq ASC`
	args := []any{address, after, time.Now().UTC().Format(time.RFC3339)}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	messages := make([]*model.Message, 0)
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning message row: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message rows: %w", err)
	}

	return messages, nil
}

// PurgeExpired deletes all messages whose ExpiresAt is in the past.
// Messages with NULL expires_at ("forever") are never purged.
// Returns the number of deleted messages.
func (s *SQLiteStore) PurgeExpired(ctx context.Context) (int64, error) {
	// Only purge messages that have an expiry set (NOT NULL) and are past their expiry
	query := `DELETE FROM messages WHERE expires_at IS NOT NULL AND expires_at < ?`

	result, err := s.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339))
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
