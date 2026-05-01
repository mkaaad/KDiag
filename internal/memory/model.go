// Package memory provides an autonomous memory system for KDiag, allowing the
// LLM agent to store, search, and retrieve structured environment intelligence
// across diagnosis sessions. The agent autonomously decides what facts are
// worth remembering, classifies them into predefined categories, and reads
// only summaries until it needs full details.
package memory

import (
	"context"
	"time"
)

// Category represents a preset classification for stored memories.
type Category string

const (
	CatServiceTopology  Category = "service_topology"   // 服务依赖与调用关系
	CatKnownIssue       Category = "known_issue"        // 已知问题/隐患/workaround
	CatRunbook          Category = "runbook"             // 应急处置步骤
	CatPeriodicPattern  Category = "periodic_pattern"    // 周期性规律（如"每周二14点CPU飙高"）
	CatConfigFact       Category = "config_fact"         // 配置事实（如"超时阈值200ms"）
	CatPastIncident     Category = "past_incident"       // 历史事故根因
	CatDeployChange     Category = "deploy_change"       // 部署变更记录
	CatResourceRelation Category = "resource_relation"   // 资源归属关系
)

// Memory represents a single piece of stored environment intelligence.
type Memory struct {
	ID         string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Category   Category  `gorm:"index;not null"`
	Summary    string    `gorm:"type:text;not null"`        // agent 搜索时看到的内容
	Detail     string    `gorm:"type:text;not null"`        // agent 展开阅读时看到的内容
	Tags       []string  `gorm:"type:jsonb;not null;default:'[]'"` // 用于匹配告警标签
	Confidence string    `gorm:"type:text;not null;default:medium"` // high / medium / low
	HitCount   int       `gorm:"default:0"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

// SummaryItem is the lightweight view returned by search (no Detail).
type SummaryItem struct {
	ID         string   `json:"id"`
	Category   Category `json:"category"`
	Summary    string   `json:"summary"`
	Confidence string   `json:"confidence"`
	HitCount   int      `json:"hit_count"`
	CreatedAt  string   `json:"created_at"`
}

// SearchInput is the JSON input for SearchMemory tool.
type SearchInput struct {
	Tags       []string   `json:"tags"`
	Categories []Category `json:"categories,omitempty"`
	Limit      int        `json:"limit,omitempty"`
}

// ReadInput is the JSON input for ReadMemory tool.
type ReadInput struct {
	ID string `json:"id"`
}

// RememberInput is the JSON input for Remember tool.
type RememberInput struct {
	Category   Category `json:"category"`
	Summary    string   `json:"summary"`
	Detail     string   `json:"detail"`
	Tags       []string `json:"tags"`
	Confidence string   `json:"confidence"`
	TTLHours   int      `json:"ttl_hours,omitempty"`
}

// Store defines the interface for persisting and retrieving memories.
type Store interface {
	Search(ctx context.Context, input SearchInput) ([]SummaryItem, error)
	Read(ctx context.Context, id string) (*Memory, error)
	Save(ctx context.Context, m *Memory) error
	Close() error
}
