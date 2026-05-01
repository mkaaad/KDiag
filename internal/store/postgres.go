package store

import (
	"context"
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
	ID          int64           `gorm:"primaryKey;autoIncrement"`
	SessionID   string          `gorm:"uniqueIndex;not null;type:text"`
	AlertName   string          `gorm:"index;not null;default:'';type:text"`
	Fingerprint string          `gorm:"index;not null;default:'';type:text"`
	AlertRaw    string          `gorm:"not null;type:text"`
	Diagnosis   string          `gorm:"not null;default:'';type:text"`
	Embedding   pgvector.Vector `gorm:"type:vector(1536)"`
	CreatedAt   time.Time       `gorm:"autoCreateTime"`
	UpdatedAt   time.Time       `gorm:"autoUpdateTime"`
}

// messageModel is the GORM model for the messages table.
type messageModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	SessionID string    `gorm:"index;not null;type:text"`
	Role      string    `gorm:"not null;type:text"`
	Content   string    `gorm:"not null;type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime;index"`
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
	if err := s.db.AutoMigrate(&diagnosisModel{}, &messageModel{}); err != nil {
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
func (s *PostgresStore) SaveDiagnosis(ctx context.Context, sessionID, fingerprint, alertName, alertRaw, diagnosis string, embedding []float32) error {
	record := &diagnosisModel{
		SessionID:   sessionID,
		Fingerprint: fingerprint,
		AlertName:   alertName,
		AlertRaw:    alertRaw,
		Diagnosis:   diagnosis,
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
func (s *PostgresStore) SearchByFingerprint(ctx context.Context, fingerprint string, limit int) ([]Diagnosis, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var models []diagnosisModel
	err := s.db.WithContext(ctx).
		Where("fingerprint LIKE ?", fingerprint[:16]+"%").
		Order("created_at DESC").
		Limit(limit).
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
		ID:          m.ID,
		SessionID:   m.SessionID,
		AlertName:   m.AlertName,
		Fingerprint: m.Fingerprint,
		AlertRaw:    m.AlertRaw,
		Diagnosis:   m.Diagnosis,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}
