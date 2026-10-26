package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AOSSIE-Org/ThruBox-Server/internal/model"
	"github.com/AOSSIE-Org/ThruBox-Server/internal/store"
)

// newTestServer wires the message routes to a fresh SQLite store, the same
// way cmd/relay does.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	db, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	h := &MessageHandler{Store: db, TTLDays: 7, MaxPayloadSize: 1 << 16}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/messages", h.HandleCreate)
	mux.HandleFunc("GET /api/messages/{address}", h.HandleGetByAddress)
	return mux
}

func post(t *testing.T, srv http.Handler, body string) (int, model.Message) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, r)

	var msg model.Message
	if rec.Code < 300 {
		if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
			t.Fatalf("decoding POST response %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, msg
}

func get(t *testing.T, srv http.Handler, target string) (int, []model.Message) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, r)

	var msgs []model.Message
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &msgs); err != nil {
			t.Fatalf("decoding GET response %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, msgs
}

func TestHandleCreate_ContentIDAndSeq(t *testing.T) {
	srv := newTestServer(t)

	code, msg := post(t, srv, `{"to":"0xalice","from":"0xbob","payload":"cipher"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201", code)
	}
	if want := model.ContentID("0xalice", "0xbob", "cipher"); msg.ID != want {
		t.Errorf("ID = %q, want ContentID %q", msg.ID, want)
	}
	if msg.Seq != 1 {
		t.Errorf("Seq = %d, want 1", msg.Seq)
	}
}

func TestHandleCreate_RetryIsIdempotent(t *testing.T) {
	srv := newTestServer(t)
	body := `{"to":"0xalice","from":"0xbob","payload":"cipher"}`

	_, first := post(t, srv, body)
	code, retry := post(t, srv, body)
	if code != http.StatusOK {
		t.Errorf("retried POST status = %d, want 200", code)
	}
	if retry.ID != first.ID || retry.Seq != first.Seq {
		t.Errorf("retried POST returned (%s, %d), want the original (%s, %d)",
			retry.ID, retry.Seq, first.ID, first.Seq)
	}

	_, msgs := get(t, srv, "/api/messages/0xalice")
	if len(msgs) != 1 {
		t.Errorf("mailbox has %d messages after a retry, want 1", len(msgs))
	}
}

func TestHandleGetByAddress_Cursor(t *testing.T) {
	srv := newTestServer(t)
	for _, p := range []string{"a", "b", "c", "d"} {
		if code, _ := post(t, srv, `{"to":"0xalice","from":"0xbob","payload":"`+p+`"}`); code != http.StatusCreated {
			t.Fatalf("POST %s status = %d", p, code)
		}
	}

	tests := []struct {
		target string
		want   []string
	}{
		{"/api/messages/0xalice", []string{"a", "b", "c", "d"}},
		{"/api/messages/0xalice?after=2", []string{"c", "d"}},
		{"/api/messages/0xalice?limit=3", []string{"a", "b", "c"}},
		{"/api/messages/0xalice?after=1&limit=2", []string{"b", "c"}},
		{"/api/messages/0xalice?after=4", []string{}},
		{"/api/messages/0xnobody", []string{}},
	}
	for _, tc := range tests {
		code, msgs := get(t, srv, tc.target)
		if code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", tc.target, code)
			continue
		}
		got := make([]string, len(msgs))
		for i, m := range msgs {
			got[i] = m.Payload
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("GET %s payloads = %v, want %v", tc.target, got, tc.want)
		}
	}
}

func TestHandleGetByAddress_EmptyMailboxIsArray(t *testing.T) {
	srv := newTestServer(t)

	r := httptest.NewRequest(http.MethodGet, "/api/messages/0xnobody", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, r)
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("empty mailbox body = %q, want []", body)
	}
}

func TestHandleGetByAddress_InvalidParams(t *testing.T) {
	srv := newTestServer(t)

	for _, q := range []string{
		"after=-1",
		"after=abc",
		"after=1.5",
		"limit=0",
		"limit=-3",
		"limit=1001",
		"limit=ten",
	} {
		code, _ := get(t, srv, "/api/messages/0xalice?"+q)
		if code != http.StatusBadRequest {
			t.Errorf("GET ?%s status = %d, want 400", q, code)
		}
	}
}

// TestNoDeleteEndpoint guards the removal of DELETE /api/messages/{id},
// which let anyone delete any user's messages. Messages leave the relay only
// by expiring.
func TestNoDeleteEndpoint(t *testing.T) {
	srv := newTestServer(t)
	_, msg := post(t, srv, `{"to":"0xalice","from":"0xbob","payload":"keep me"}`)

	r := httptest.NewRequest(http.MethodDelete, "/api/messages/"+msg.ID, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, r)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE status = %d, want 405", rec.Code)
	}

	if _, msgs := get(t, srv, "/api/messages/0xalice"); len(msgs) != 1 {
		t.Errorf("mailbox has %d messages after DELETE, want 1", len(msgs))
	}
}
