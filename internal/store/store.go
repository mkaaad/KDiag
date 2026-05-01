// Package store provides storage interfaces and types for persisting KDiag
// diagnosis results, sessions, and related data.
package store

import (
	"context"
	"time"
)

// Diagnosis represents a single alert diagnosis record.
type Diagnosis struct {
	ID          int64     `json:"id"`
	SessionID   string    `json:"session_id"`
	AlertName   string    `json:"alert_name"`
	Fingerprint string    `json:"fingerprint"`
	AlertRaw    string    `json:"alert_raw"`
	Diagnosis   string    `json:"diagnosis"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Distance    float64   `json:"distance,omitempty"` // cosine distance from vector search, 0 otherwise
}

// Message represents a single message in a diagnosis session.
type Message struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Store defines the interface for persisting diagnosis data.
type Store interface {
	// SaveDiagnosis stores or updates a diagnosis result for a session.
	// If embedding is non-nil, it is stored in a pgvector column for
	// similarity search.
	SaveDiagnosis(ctx context.Context, sessionID, fingerprint, alertName, alertRaw, diagnosis string, embedding []float32) error

	// GetDiagnosis retrieves a diagnosis by session ID.
	GetDiagnosis(ctx context.Context, sessionID string) (*Diagnosis, error)

	// ListDiagnoses returns recent diagnoses with pagination.
	ListDiagnoses(ctx context.Context, limit, offset int) ([]Diagnosis, error)

	// AppendMessage adds a message to a session's conversation history.
	AppendMessage(ctx context.Context, sessionID, role, content string) error

	// GetMessages retrieves all messages for a session, ordered by creation time.
	GetMessages(ctx context.Context, sessionID string) ([]Message, error)

	// SearchByFingerprint finds diagnoses with a similar fingerprint prefix.
	SearchByFingerprint(ctx context.Context, fingerprint string, limit int) ([]Diagnosis, error)

	// SearchByVector finds diagnoses with similar embedding vectors using
	// cosine distance (<=>). Requires pgvector extension on PostgreSQL.
	// Returns up to limit results ordered by similarity ascending.
	SearchByVector(ctx context.Context, embedding []float32, limit int) ([]Diagnosis, error)

	// Close cleans up store resources (e.g., closes the database connection).
	Close() error
}
