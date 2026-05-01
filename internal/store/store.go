// Package store provides storage interfaces and types for persisting KDiag
// diagnosis results, sessions, and related data.
package store

import (
	"context"
	"time"
)

// Diagnosis represents a single alert diagnosis record.
type Diagnosis struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	AlertRaw  string    `json:"alert_raw"`
	Diagnosis string    `json:"diagnosis"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Distance  float64   `json:"distance,omitempty"` // cosine distance from vector search, 0 otherwise
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
	SaveDiagnosis(ctx context.Context, sessionID, alertRaw, diagnosis string, embedding []float32) error

	// GetDiagnosis retrieves a diagnosis by session ID.
	GetDiagnosis(ctx context.Context, sessionID string) (*Diagnosis, error)

	// ListDiagnoses returns recent diagnoses with pagination.
	ListDiagnoses(ctx context.Context, limit, offset int) ([]Diagnosis, error)

	// AppendMessage adds a message to a session's conversation history.
	AppendMessage(ctx context.Context, sessionID, role, content string) error

	// GetMessages retrieves all messages for a session, ordered by creation time.
	GetMessages(ctx context.Context, sessionID string) ([]Message, error)

	// SearchByVector finds diagnoses with similar embedding vectors using
	// cosine distance (<=>). Requires pgvector extension on PostgreSQL.
	// Returns up to limit results ordered by similarity ascending.
	SearchByVector(ctx context.Context, embedding []float32, limit int) ([]Diagnosis, error)

	// SaveTreeNodes persists all tree nodes for a diagnosis session.
	// It replaces any existing nodes for the same session.
	SaveTreeNodes(ctx context.Context, sessionID string, nodes []TreeNodeData) error

	// SaveTraceLinks persists trace_id → session_id mappings for
	// cross-alert correlation. It replaces any existing links for the session.
	SaveTraceLinks(ctx context.Context, sessionID string, traceIDs []string) error

	// SearchByTraceID finds all diagnosis sessions that share the given
	// trace ID, excluding the current session.
	SearchByTraceID(ctx context.Context, traceID, excludeSessionID string) ([]Diagnosis, error)

	// SaveTreePath stores the tree path text and its embedding for
	// vector-based tree structure similarity search.
	SaveTreePath(ctx context.Context, sessionID, pathText string, embedding []float32) error

	// SearchTreeByVector finds sessions with similar tree path embeddings
	// using cosine distance. Returns up to limit results.
	SearchTreeByVector(ctx context.Context, embedding []float32, limit int) ([]Diagnosis, error)

	// Close cleans up store resources (e.g., closes the database connection).
	Close() error
}

// TreeNodeData is a flattened representation of a tree node for persistence.
type TreeNodeData struct {
	NodeID   string            `json:"node_id"`
	Type     string            `json:"type"`
	Summary  string            `json:"summary"`
	ParentID string            `json:"parent_id,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
	TraceID  string            `json:"trace_id,omitempty"`
	SpanID   string            `json:"span_id,omitempty"`
	Service  string            `json:"service,omitempty"`
	Query    string            `json:"query,omitempty"`
}
