package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/AOSSIE-Org/ThruBox-Server/internal/model"
	"github.com/AOSSIE-Org/ThruBox-Server/internal/store"
)

// MaxPageSize is the largest limit a client may ask for when reading a mailbox.
const MaxPageSize = 1000

// MessageHandler holds dependencies for message-related HTTP handlers.
type MessageHandler struct {
	Store          store.Store
	TTLDays        int
	MaxPayloadSize int
}

// HandleCreate handles POST /api/messages.
// Validates the request body, derives the message ID from its content,
// computes the expiry time, and saves the message to the store.
//
// Responds 201 with the stored message. If an identical message is already
// stored (a retried send), nothing new is stored and it responds 200 with the
// existing message, so retrying a send is safe.
func (h *MessageHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	// Enforce max payload size
	r.Body = http.MaxBytesReader(w, r.Body, int64(h.MaxPayloadSize))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var req model.CreateMessageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if errMsg := req.Validate(); errMsg != "" {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	msg := &model.Message{
		ID:        model.ContentID(req.To, req.From, req.Payload),
		To:        req.To,
		From:      req.From,
		Payload:   req.Payload,
		CreatedAt: now,
	}

	// TTL of 0 means "forever" — the message never expires
	if h.TTLDays > 0 {
		exp := now.Add(time.Duration(h.TTLDays) * 24 * time.Hour)
		msg.ExpiresAt = &exp
	}
	// else: ExpiresAt stays nil, meaning "never expires"

	created, err := h.Store.SaveMessage(r.Context(), msg)
	if err != nil {
		slog.Error("failed to save message", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	status := http.StatusCreated
	if created {
		slog.Info("message stored", "id", msg.ID, "seq", msg.Seq)
	} else {
		status = http.StatusOK
		slog.Info("message already stored", "id", msg.ID, "seq", msg.Seq)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(msg); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

// HandleGetByAddress handles GET /api/messages/{address}.
// Returns the messages addressed to the given wallet address as a JSON array,
// in sequence order.
//
// Optional query parameters:
//   - after: only return messages with a seq greater than this. A client
//     passes the seq of the last message it has handled so it is not sent
//     the same messages again.
//   - limit: return at most this many messages (1 to MaxPageSize). Omitted,
//     all matching messages are returned. A full page means there may be
//     more; ask again with after set to the last seq received.
func (h *MessageHandler) HandleGetByAddress(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	if address == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		return
	}

	query := r.URL.Query()

	var after int64
	if v := query.Get("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			http.Error(w, "after must be a non-negative integer", http.StatusBadRequest)
			return
		}
		after = n
	}

	var limit int
	if v := query.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxPageSize {
			http.Error(w, "limit must be an integer from 1 to "+strconv.Itoa(MaxPageSize), http.StatusBadRequest)
			return
		}
		limit = n
	}

	messages, err := h.Store.GetMessagesByAddress(r.Context(), address, after, limit)
	if err != nil {
		slog.Error("failed to get messages", "error", err, "address", address)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(messages); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}
