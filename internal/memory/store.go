package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PostgresStore implements the Store interface using GORM + PostgreSQL.
type PostgresStore struct {
	db *gorm.DB
}

// PostgresConfig holds connection parameters for the PostgreSQL memory store.
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

// NewPostgresStore creates a PostgresStore with auto-migration.
func NewPostgresStore(ctx context.Context, cfg PostgresConfig) (*PostgresStore, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
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

func (s *PostgresStore) migrate() error {
	return s.db.AutoMigrate(&Memory{})
}

// Search returns matching memory summaries (no Detail) ordered by hit count.
func (s *PostgresStore) Search(ctx context.Context, input SearchInput) ([]SummaryItem, error) {
	limit := input.Limit
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	q := s.db.WithContext(ctx).Model(&Memory{})

	// Filter by tags (JSONB containment: any of the input tags exist).
	if len(input.Tags) > 0 {
		// Build grouped OR condition for each tag using JSONB ?? operator.
		tagQ := s.db.Where("tags ?? ?", input.Tags[0])
		for _, tag := range input.Tags[1:] {
			tagQ = tagQ.Or("tags ?? ?", tag)
		}
		q = q.Where(tagQ)
	}

	if len(input.Categories) > 0 {
		q = q.Where("category IN ?", input.Categories)
	}

	var results []SummaryItem
	err := q.Order("hit_count DESC, updated_at DESC").
		Limit(limit).
		Select("id, category, summary, confidence, hit_count, created_at").
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// Read returns the full Memory (including Detail) by ID.
func (s *PostgresStore) Read(ctx context.Context, id string) (*Memory, error) {
	var m Memory
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	// Increment hit count asynchronously.
	_ = s.db.Model(&Memory{}).Where("id = ?", id).
		UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).Error
	return &m, nil
}

// Save creates or updates a memory record.
func (s *PostgresStore) Save(ctx context.Context, m *Memory) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"summary", "detail", "tags", "confidence", "updated_at"}),
		}).
		Create(m).Error
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
