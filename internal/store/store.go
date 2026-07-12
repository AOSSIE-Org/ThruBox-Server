package store

import (
	"context"

	"github.com/AOSSIE-Org/ThruBox-Server/internal/model"
)

// Store defines the storage interface for the relay server.
// All handler code talks to this interface — never to a concrete implementation.
// This makes it trivial to swap SQLite for PostgreSQL, GORM, or any other backend
// by implementing this interface in a new file.
type Store interface {
	// SaveMessage persists a new message to the store.
	SaveMessage(ctx context.Context, msg *model.Message) error

	// GetMessagesByAddress returns all messages addressed to the given wallet address.
	// Returns an empty slice (not nil) if no messages are found.
	GetMessagesByAddress(ctx context.Context, address string) ([]*model.Message, error)

	// DeleteMessage removes a message by its ID.
	// Returns nil if the message does not exist (idempotent delete).
	DeleteMessage(ctx context.Context, id string) error

	// PurgeExpired deletes all messages whose ExpiresAt is in the past.
	// Returns the number of deleted messages.
	// Called periodically by a background goroutine.
	PurgeExpired(ctx context.Context) (int64, error)

	// Close releases any resources held by the store (e.g., DB connections).
	Close() error
}
