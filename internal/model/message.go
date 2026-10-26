package model

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"time"
)

// Message represents an encrypted message stored in the relay.
// The server treats the Payload as an opaque blob — all encryption/decryption
// happens client-side. The relay is intentionally "dumb".
type Message struct {
	ID        string     `json:"id"`        // ContentID of to, from and payload, computed by server
	Seq       int64      `json:"seq"`       // Position in the recipient's mailbox, assigned by server
	To        string     `json:"to"`        // Recipient wallet address (the lookup key)
	From      string     `json:"from"`      // Sender wallet address
	Payload   string     `json:"payload"`   // Encrypted data (base64 string, opaque to server)
	CreatedAt time.Time  `json:"createdAt"` // Timestamp when message was stored
	ExpiresAt *time.Time `json:"expiresAt"` // Auto-purge time (nil = forever, only manual delete)
}

// ContentID returns a message's ID: the hex SHA-256 of its recipient, sender
// and payload. The recipient and sender are included so the same payload sent
// to two mailboxes gets two IDs. Each field is length-prefixed, so moving bytes
// from one field into its neighbour always changes the hash.
func ContentID(to, from, payload string) string {
	h := sha256.New()
	for _, field := range []string{to, from, payload} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(field)))
		h.Write(n[:])
		io.WriteString(h, field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// CreateMessageRequest is the expected JSON body for POST /api/messages.
type CreateMessageRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Payload string `json:"payload"`
}

// Validate checks that all required fields are present and non-empty.
func (r *CreateMessageRequest) Validate() string {
	if r.To == "" {
		return "field 'to' is required"
	}
	if r.From == "" {
		return "field 'from' is required"
	}
	if r.Payload == "" {
		return "field 'payload' is required"
	}
	return ""
}
