package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AOSSIE-Org/ThruBox-Server/internal/model"
)

// newTestStore opens a fresh database in a temp dir.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newMessage builds an unsaved message with a content ID.
func newMessage(to, payload string) *model.Message {
	return &model.Message{
		ID:        model.ContentID(to, "0xsender", payload),
		To:        to,
		From:      "0xsender",
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
}

// save stores msg and fails the test if it was not newly created.
func save(t *testing.T, s *SQLiteStore, msg *model.Message) {
	t.Helper()
	created, err := s.SaveMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("SaveMessage() error = %v", err)
	}
	if !created {
		t.Fatalf("SaveMessage(%q) created = false, want true", msg.Payload)
	}
}

// seqs returns the sequence numbers of msgs, in order.
func seqs(msgs []*model.Message) []int64 {
	out := make([]int64, len(msgs))
	for i, m := range msgs {
		out[i] = m.Seq
	}
	return out
}

func equalSeqs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSaveMessage_SeqIsPerMailbox(t *testing.T) {
	s := newTestStore(t)

	for i, tc := range []struct {
		to   string
		want int64
	}{
		{"0xalice", 1},
		{"0xalice", 2},
		{"0xbob", 1},
		{"0xalice", 3},
		{"0xbob", 2},
	} {
		msg := newMessage(tc.to, fmt.Sprintf("payload-%d", i))
		save(t, s, msg)
		if msg.Seq != tc.want {
			t.Errorf("message %d to %s: Seq = %d, want %d", i, tc.to, msg.Seq, tc.want)
		}
	}
}

// TestSaveMessage_SeqNotReusedAfterPurge guards against handing out a
// number again once the newest message is gone. A client whose cursor is at
// that number would otherwise never see the new message.
func TestSaveMessage_SeqNotReusedAfterPurge(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first := newMessage("0xalice", "one")
	save(t, s, first)
	second := newMessage("0xalice", "two")
	past := time.Now().UTC().Add(-time.Hour)
	second.ExpiresAt = &past
	save(t, s, second)

	if n, err := s.PurgeExpired(ctx); err != nil || n != 1 {
		t.Fatalf("PurgeExpired() = %d, %v; want 1, nil", n, err)
	}

	third := newMessage("0xalice", "three")
	save(t, s, third)
	if third.Seq != 3 {
		t.Errorf("Seq after purging the newest message = %d, want 3", third.Seq)
	}

	got, err := s.GetMessagesByAddress(ctx, "0xalice", second.Seq, 0)
	if err != nil {
		t.Fatalf("GetMessagesByAddress() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != third.ID {
		t.Errorf("messages after seq %d = %v, want only the third message", second.Seq, seqs(got))
	}
}

func TestSaveMessage_DuplicateReturnsExisting(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	original := newMessage("0xalice", "same")
	save(t, s, original)

	retry := newMessage("0xalice", "same")
	retry.CreatedAt = original.CreatedAt.Add(time.Minute)
	created, err := s.SaveMessage(ctx, retry)
	if err != nil {
		t.Fatalf("SaveMessage() error = %v", err)
	}
	if created {
		t.Error("created = true for a duplicate, want false")
	}
	if retry.Seq != original.Seq {
		t.Errorf("duplicate Seq = %d, want the original's %d", retry.Seq, original.Seq)
	}
	if !retry.CreatedAt.Equal(original.CreatedAt.Truncate(time.Second)) {
		t.Errorf("duplicate CreatedAt = %v, want the stored %v", retry.CreatedAt, original.CreatedAt)
	}

	got, err := s.GetMessagesByAddress(ctx, "0xalice", 0, 0)
	if err != nil {
		t.Fatalf("GetMessagesByAddress() error = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("mailbox has %d messages, want 1", len(got))
	}

	// The duplicate did not use up a sequence number.
	next := newMessage("0xalice", "next")
	save(t, s, next)
	if next.Seq != 2 {
		t.Errorf("Seq after a duplicate = %d, want 2", next.Seq)
	}
}

// TestSaveMessage_ExpiredDuplicateIsReplaced covers a resend of a message
// that has expired but not been purged yet: it must be stored again rather
// than matched against a copy the recipient can no longer see.
func TestSaveMessage_ExpiredDuplicateIsReplaced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	old := newMessage("0xalice", "again")
	past := time.Now().UTC().Add(-time.Hour)
	old.ExpiresAt = &past
	save(t, s, old)

	resent := newMessage("0xalice", "again")
	save(t, s, resent)
	if resent.Seq != 2 {
		t.Errorf("resent Seq = %d, want 2 so clients past seq 1 still see it", resent.Seq)
	}

	got, err := s.GetMessagesByAddress(ctx, "0xalice", 0, 0)
	if err != nil {
		t.Fatalf("GetMessagesByAddress() error = %v", err)
	}
	if !equalSeqs(seqs(got), []int64{2}) {
		t.Errorf("visible seqs = %v, want [2]", seqs(got))
	}
}

func TestGetMessagesByAddress_AfterAndLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		save(t, s, newMessage("0xalice", fmt.Sprintf("p%d", i)))
	}
	save(t, s, newMessage("0xbob", "not alice's"))

	tests := []struct {
		after int64
		limit int
		want  []int64
	}{
		{0, 0, []int64{1, 2, 3, 4, 5}},
		{2, 0, []int64{3, 4, 5}},
		{0, 2, []int64{1, 2}},
		{2, 2, []int64{3, 4}},
		{5, 0, []int64{}},
		{99, 10, []int64{}},
	}
	for _, tc := range tests {
		got, err := s.GetMessagesByAddress(ctx, "0xalice", tc.after, tc.limit)
		if err != nil {
			t.Fatalf("GetMessagesByAddress(after=%d, limit=%d) error = %v", tc.after, tc.limit, err)
		}
		if got == nil {
			t.Errorf("after=%d, limit=%d: got nil, want an empty slice", tc.after, tc.limit)
		}
		if !equalSeqs(seqs(got), tc.want) {
			t.Errorf("after=%d, limit=%d: seqs = %v, want %v", tc.after, tc.limit, seqs(got), tc.want)
		}
	}
}

func TestSaveMessage_ConcurrentSeqsAreUnique(t *testing.T) {
	s := newTestStore(t)

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.SaveMessage(context.Background(), newMessage("0xalice", fmt.Sprintf("p%d", i))); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent SaveMessage() error = %v", err)
	}

	got, err := s.GetMessagesByAddress(context.Background(), "0xalice", 0, 0)
	if err != nil {
		t.Fatalf("GetMessagesByAddress() error = %v", err)
	}
	want := make([]int64, n)
	for i := range want {
		want[i] = int64(i + 1)
	}
	if !equalSeqs(seqs(got), want) {
		t.Errorf("seqs = %v, want 1..%d with no gaps or repeats", seqs(got), n)
	}
}

// TestNewSQLiteStore_MigratesOldSchema opens a database written before
// messages had sequence numbers and checks the existing messages are
// numbered in arrival order and new ones carry on from there.
func TestNewSQLiteStore_MigratesOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.db")

	old, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("opening old database: %v", err)
	}
	_, err = old.Exec(`
	CREATE TABLE messages (
		id         TEXT PRIMARY KEY,
		to_addr    TEXT NOT NULL,
		from_addr  TEXT NOT NULL,
		payload    TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		expires_at DATETIME
	);
	INSERT INTO messages VALUES
		('uuid-c', '0xalice', '0xs', 'third',  '2026-01-01T00:00:03Z', NULL),
		('uuid-a', '0xalice', '0xs', 'first',  '2026-01-01T00:00:01Z', NULL),
		('uuid-b', '0xalice', '0xs', 'second', '2026-01-01T00:00:02Z', NULL),
		('uuid-x', '0xbob',   '0xs', 'bob',    '2026-01-01T00:00:01Z', NULL),
		-- Same second, inserted in this order, with IDs that sort the other way.
		('uuid-z', '0xalice', '0xs', 'fourth', '2026-01-01T00:00:04Z', NULL),
		('uuid-d', '0xalice', '0xs', 'fifth',  '2026-01-01T00:00:04Z', NULL);
	`)
	old.Close()
	if err != nil {
		t.Fatalf("writing old schema: %v", err)
	}

	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore() on old schema error = %v", err)
	}
	defer s.Close()

	got, err := s.GetMessagesByAddress(context.Background(), "0xalice", 0, 0)
	if err != nil {
		t.Fatalf("GetMessagesByAddress() error = %v", err)
	}
	wantOrder := []string{"first", "second", "third", "fourth", "fifth"}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d messages, want %d", len(got), len(wantOrder))
	}
	for i, m := range got {
		if m.Payload != wantOrder[i] || m.Seq != int64(i+1) {
			t.Errorf("message %d = (%q, seq %d), want (%q, seq %d)", i, m.Payload, m.Seq, wantOrder[i], i+1)
		}
	}

	next := newMessage("0xalice", "sixth")
	save(t, s, next)
	if next.Seq != 6 {
		t.Errorf("first Seq after migration = %d, want 6", next.Seq)
	}

	bob := newMessage("0xbob", "bob 2")
	save(t, s, bob)
	if bob.Seq != 2 {
		t.Errorf("bob's first Seq after migration = %d, want 2", bob.Seq)
	}

	// Opening the migrated database again must not renumber anything.
	s.Close()
	again, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopening migrated database: %v", err)
	}
	defer again.Close()
	got, err = again.GetMessagesByAddress(context.Background(), "0xalice", 0, 0)
	if err != nil {
		t.Fatalf("GetMessagesByAddress() after reopen error = %v", err)
	}
	if !equalSeqs(seqs(got), []int64{1, 2, 3, 4, 5, 6}) {
		t.Errorf("seqs after reopen = %v, want [1 2 3 4 5 6]", seqs(got))
	}
}
