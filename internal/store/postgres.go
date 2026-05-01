package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pgvector/pgvector-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PostgresStore implements the Store interface using GORM + PostgreSQL.
type PostgresStore struct {
	db *gorm.DB
}

// PostgresConfig holds the connection parameters for PostgreSQL.
type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

// DSN returns the PostgreSQL connection string.
func (c PostgresConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		c.Host, c.Port, c.User, c.Password, c.Database)
}

// diagnosisModel is the GORM model for the diagnoses table.
type diagnosisModel struct {
	ID        int64           `gorm:"primaryKey;autoIncrement"`
	SessionID string          `gorm:"uniqueIndex;not null;type:text"`
	AlertRaw  string          `gorm:"not null;type:text"`
	Diagnosis string          `gorm:"not null;default:'';type:text"`
	Embedding pgvector.Vector `gorm:"type:vector(1536)"`
	CreatedAt time.Time       `gorm:"autoCreateTime"`
	UpdatedAt time.Time       `gorm:"autoUpdateTime"`
}

// messageModel is the GORM model for the messages table.
type messageModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	SessionID string    `gorm:"index;not null;type:text"`
	Role      string    `gorm:"not null;type:text"`
	Content   string    `gorm:"not null;type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime;index"`
}

// treeNodeModel is the GORM model for the tree_nodes table.
type treeNodeModel struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	SessionID string `gorm:"index;not null;type:text"`
	NodeID    string `gorm:"not null;type:text"`
	Type      string `gorm:"not null;type:text"`
	Summary   string `gorm:"not null;type:text"`
	ParentID  string `gorm:"default:'';type:text"`
	Meta      string `gorm:"default:'';type:text"` // JSON-encoded map
	TraceID   string `gorm:"default:'';type:text;index"`
	SpanID    string `gorm:"default:'';type:text"`
	Service   string `gorm:"default:'';type:text"`
	Query     string `gorm:"default:'';type:text"`
}

// traceLinkModel maps trace IDs to diagnosis sessions for cross-alert correlation.
type traceLinkModel struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	TraceID   string `gorm:"index;not null;type:text"`
	SessionID string `gorm:"index;not null;type:text"`
}

// treePathModel stores the tree path text and its embedding for vector search.
type treePathModel struct {
	ID        int64           `gorm:"primaryKey;autoIncrement"`
	SessionID string          `gorm:"uniqueIndex;not null;type:text"`
	PathText  string          `gorm:"not null;type:text"`
	Embedding pgvector.Vector `gorm:"type:vector(1536)"`
	CreatedAt time.Time       `gorm:"autoCreateTime"`
}

// NewPostgresStore creates a new PostgresStore, connects to the database,
// and runs auto-migration.
func NewPostgresStore(ctx context.Context, cfg PostgresConfig) (*PostgresStore, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	// Verify connectivity.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying sql.DB: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	s := &PostgresStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate postgres: %w", err)
	}
	return s, nil
}

// migrate runs GORM auto-migration to create/update tables.
// It also enables the pgvector extension and creates an HNSW index
// on the embedding column for fast approximate nearest neighbor search.
func (s *PostgresStore) migrate() error {
	// Enable pgvector extension.
	if err := s.db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return fmt.Errorf("enable vector extension: %w", err)
	}
	if err := s.db.AutoMigrate(&diagnosisModel{}, &messageModel{}, &treeNodeModel{}, &traceLinkModel{}, &treePathModel{}); err != nil {
		return err
	}
	// Create HNSW index on the embedding column for cosine distance search.
	// The index is created IF NOT EXISTS to make migration idempotent.
	return s.db.Exec(
		"CREATE INDEX IF NOT EXISTS idx_diagnoses_embedding ON diagnoses USING hnsw (embedding vector_cosine_ops)",
	).Error
}

// SaveDiagnosis inserts or updates a diagnosis record using upsert.
// If embedding is non-nil, it is stored in the pgvector column for
// similarity search.
func (s *PostgresStore) SaveDiagnosis(ctx context.Context, sessionID, alertRaw, diagnosis string, embedding []float32) error {
	record := &diagnosisModel{
		SessionID: sessionID,
		AlertRaw:  alertRaw,
		Diagnosis: diagnosis,
	}
	if embedding != nil {
		record.Embedding = pgvector.NewVector(embedding)
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "session_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"diagnosis", "embedding", "updated_at"}),
		}).
		Create(record).Error
}

// SearchByVector finds diagnoses with similar embedding vectors using
// cosine distance (<=>). Requires pgvector extension on PostgreSQL.
// Returns up to limit results ordered by similarity ascending.
func (s *PostgresStore) SearchByVector(ctx context.Context, embedding []float32, limit int) ([]Diagnosis, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	if len(embedding) == 0 {
		return nil, nil
	}
	vec := pgvector.NewVector(embedding)
	var models []diagnosisModel
	err := s.db.WithContext(ctx).
		Raw("SELECT *, embedding <=> ? AS distance FROM diagnoses ORDER BY embedding <=> ? LIMIT ?", vec, vec, limit).
		Scan(&models).Error
	if err != nil {
		return nil, err
	}
	results := make([]Diagnosis, len(models))
	for i, m := range models {
		results[i] = *modelToDiagnosis(&m)
	}
	return results, nil
}


// GetDiagnosis retrieves a diagnosis by session ID.
func (s *PostgresStore) GetDiagnosis(ctx context.Context, sessionID string) (*Diagnosis, error) {
	var m diagnosisModel
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&m).Error
	if err != nil {
		return nil, err
	}
	return modelToDiagnosis(&m), nil
}

// ListDiagnoses returns recent diagnoses ordered by creation time descending.
func (s *PostgresStore) ListDiagnoses(ctx context.Context, limit, offset int) ([]Diagnosis, error) {
	if limit <= 0 {
		limit = 20
	}
	var models []diagnosisModel
	err := s.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	results := make([]Diagnosis, len(models))
	for i, m := range models {
		results[i] = *modelToDiagnosis(&m)
	}
	return results, nil
}

// AppendMessage adds a message to a session's conversation history.
func (s *PostgresStore) AppendMessage(ctx context.Context, sessionID, role, content string) error {
	m := &messageModel{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	}
	return s.db.WithContext(ctx).Create(m).Error
}

// GetMessages retrieves all messages for a session ordered by creation time.
func (s *PostgresStore) GetMessages(ctx context.Context, sessionID string) ([]Message, error) {
	var models []messageModel
	err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	results := make([]Message, len(models))
	for i, m := range models {
		results[i] = Message{
			ID:        m.ID,
			SessionID: m.SessionID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		}
	}
	return results, nil
}

// SaveTreeNodes persists all tree nodes for a session, replacing any existing
// nodes first.
func (s *PostgresStore) SaveTreeNodes(ctx context.Context, sessionID string, nodes []TreeNodeData) error {
	if len(nodes) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).Delete(&treeNodeModel{}).Error; err != nil {
			return err
		}
		models := make([]treeNodeModel, len(nodes))
		for i, n := range nodes {
			meta := ""
			if len(n.Meta) > 0 {
				b, _ := json.Marshal(n.Meta)
				meta = string(b)
			}
			models[i] = treeNodeModel{
				SessionID: sessionID,
				NodeID:    n.NodeID,
				Type:      n.Type,
				Summary:   n.Summary,
				ParentID:  n.ParentID,
				Meta:      meta,
				TraceID:   n.TraceID,
				SpanID:    n.SpanID,
				Service:   n.Service,
				Query:     n.Query,
			}
		}
		return tx.CreateInBatches(models, 100).Error
	})
}

// SaveTraceLinks persists trace_id → session_id mappings, replacing existing
// links for this session.
func (s *PostgresStore) SaveTraceLinks(ctx context.Context, sessionID string, traceIDs []string) error {
	if len(traceIDs) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).Delete(&traceLinkModel{}).Error; err != nil {
			return err
		}
		models := make([]traceLinkModel, len(traceIDs))
		for i, tid := range traceIDs {
			models[i] = traceLinkModel{TraceID: tid, SessionID: sessionID}
		}
		return tx.CreateInBatches(models, 100).Error
	})
}

// SearchByTraceID finds all diagnosis sessions that share the given trace ID,
// excluding the current session.
func (s *PostgresStore) SearchByTraceID(ctx context.Context, traceID, excludeSessionID string) ([]Diagnosis, error) {
	var results []Diagnosis
	err := s.db.WithContext(ctx).
		Raw(`
			SELECT d.* FROM diagnoses d
			INNER JOIN trace_links tl ON tl.session_id = d.session_id
			WHERE tl.trace_id = ? AND d.session_id != ?
			ORDER BY d.created_at DESC
			LIMIT 10
		`, traceID, excludeSessionID).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// SaveTreePath stores the tree path text and its embedding.
func (s *PostgresStore) SaveTreePath(ctx context.Context, sessionID, pathText string, embedding []float32) error {
	if len(embedding) == 0 {
		return nil
	}
	record := &treePathModel{
		SessionID: sessionID,
		PathText:  pathText,
		Embedding: pgvector.NewVector(embedding),
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "session_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"path_text", "embedding"}),
		}).
		Create(record).Error
}

// SearchTreeByVector finds sessions with similar tree path embeddings.
func (s *PostgresStore) SearchTreeByVector(ctx context.Context, embedding []float32, limit int) ([]Diagnosis, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	if len(embedding) == 0 {
		return nil, nil
	}
	vec := pgvector.NewVector(embedding)
	var results []Diagnosis
	err := s.db.WithContext(ctx).
		Raw(`
			SELECT d.*, tp.embedding <=> ? AS distance
			FROM diagnoses d
			INNER JOIN tree_paths tp ON tp.session_id = d.session_id
			ORDER BY tp.embedding <=> ?
			LIMIT ?
		`, vec, vec, limit).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func modelToDiagnosis(m *diagnosisModel) *Diagnosis {
	return &Diagnosis{
		ID:        m.ID,
		SessionID: m.SessionID,
		AlertRaw:  m.AlertRaw,
		Diagnosis: m.Diagnosis,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
