package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/AOSSIE-Org/ThruBox-Server/internal/model"
	"github.com/AOSSIE-Org/ThruBox-Server/internal/store"
)

// MessageHandler holds dependencies for message-related HTTP handlers.
type MessageHandler struct {
	Store          store.Store
	TTLDays        int
	MaxPayloadSize int
}

// HandleCreate handles POST /api/messages.
// Validates the request body, generates a UUID, computes the expiry time,
// and saves the message to the store.
func (h *MessageHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	// Enforce max payload size
	r.Body = http.MaxBytesReader(w, r.Body, int64(h.MaxPayloadSize))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
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
		ID:        uuid.New().String(),
		To:        req.To,
		From:      req.From,
		Payload:   req.Payload,
		CreatedAt: now,
	}

	// TTL of 0 means "forever" — message never auto-expires, only manual delete removes it
	if h.TTLDays > 0 {
		exp := now.Add(time.Duration(h.TTLDays) * 24 * time.Hour)
		msg.ExpiresAt = &exp
	}
	// else: ExpiresAt stays nil, meaning "never expires"

	if err := h.Store.SaveMessage(msg); err != nil {
		slog.Error("failed to save message", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("message stored", "id", msg.ID, "to", msg.To, "from", msg.From)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

// HandleGetByAddress handles GET /api/messages/{address}.
// Returns all messages addressed to the given wallet address as a JSON array.
func (h *MessageHandler) HandleGetByAddress(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	if address == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		return
	}

	messages, err := h.Store.GetMessagesByAddress(address)
	if err != nil {
		slog.Error("failed to get messages", "error", err, "address", address)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// HandleDelete handles DELETE /api/messages/{id}.
// Deletes a specific message by its ID.
func (h *MessageHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "message id is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteMessage(id); err != nil {
		slog.Error("failed to delete message", "error", err, "id", id)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("message deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}
