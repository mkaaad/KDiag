package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/tools"
)

// ensure *SearchMemoryTool implements tools.Tool.
var _ tools.Tool = (*SearchMemoryTool)(nil)
var _ tools.Tool = (*ReadMemoryTool)(nil)
var _ tools.Tool = (*RememberTool)(nil)

// ---------- SearchMemoryTool ----------

// SearchMemoryTool allows the agent to search memory summaries by tags.
type SearchMemoryTool struct {
	store Store
}

func (t *SearchMemoryTool) Name() string {
	return "SearchMemory"
}

func (t *SearchMemoryTool) Description() string {
	return `Search stored environment intelligence by tags and optional category filters. Returns brief summaries only — call ReadMemory for full details.

Input JSON:
- "tags" ([]string, required): search terms, e.g. ["payment-api", "production"]
- "categories" ([]string, optional): filter by category, e.g. ["known_issue", "runbook"]
- "limit" (int, optional): max results (default 10, max 20)

Example: {"tags": ["payment-api", "production"], "categories": ["known_issue"], "limit": 5}

Output: JSON array of summary items with id, category, summary, confidence, hit_count, created_at.

Use this tool to discover relevant historical knowledge before diving into details.`
}

func (t *SearchMemoryTool) Call(ctx context.Context, input string) (string, error) {
	var si SearchInput
	if err := json.Unmarshal([]byte(input), &si); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if len(si.Tags) == 0 {
		return "", fmt.Errorf("tags is required")
	}
	items, err := t.store.Search(ctx, si)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- ReadMemoryTool ----------

// ReadMemoryTool allows the agent to read the full detail of a stored memory.
type ReadMemoryTool struct {
	store Store
}

func (t *ReadMemoryTool) Name() string {
	return "ReadMemory"
}

func (t *ReadMemoryTool) Description() string {
	return `Read the full detail of a stored memory by its ID. Use this after SearchMemory returns a promising summary.

Input JSON:
- "id" (string, required): memory ID from SearchMemory results

Example: {"id": "a1b2c3d4-..."}

Output: JSON object with id, category, summary, detail, tags, confidence, hit_count, created_at, updated_at.

Use this tool when a memory summary looks relevant and you need the complete information.`
}

func (t *ReadMemoryTool) Call(ctx context.Context, input string) (string, error) {
	var ri ReadInput
	if err := json.Unmarshal([]byte(input), &ri); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if ri.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	m, err := t.store.Read(ctx, ri.ID)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- RememberTool ----------

// RememberTool allows the agent to store new environment intelligence.
type RememberTool struct {
	store Store
}

func (t *RememberTool) Name() string {
	return "Remember"
}

func (t *RememberTool) Description() string {
	return `Store a new piece of environment intelligence that may help future diagnoses. Only store stable, verified facts — not guesses.

Available categories:
- "service_topology": service dependencies and call relationships
- "known_issue": known problems, quirks, and workarounds
- "runbook": emergency response procedures
- "periodic_pattern": recurring patterns (e.g. "CPU spikes every Tuesday 2pm")
- "config_fact": configuration facts (e.g. "timeout is 200ms")
- "past_incident": historical incident root cause
- "deploy_change": deployment and release change records
- "resource_relation": resource ownership (e.g. "pod X belongs to app Y")

Input JSON:
- "category" (string, required): one of the categories above
- "summary" (string, required): brief headline (<80 chars), shown in search results
- "detail" (string, required): full context — troubleshooting steps, evidence, links
- "tags" ([]string, required): keywords for later search, include service/resource names
- "confidence" (string, optional): "high", "medium", or "low" (default "medium")
- "ttl_hours" (int, optional): estimated hours this fact stays relevant (default 720 = 30 days)

Example:
{"category":"known_issue","summary":"payment-api OOM at >2000rps","detail":"JVM heap 512MB, OOMKill at peak traffic. Temp fix: increase memory limit. Root fix: JDK21 upgrade scheduled.","tags":["payment-api","production","OOM"],"confidence":"high","ttl_hours":2160}

Output: {"status":"saved","id":"<uuid>"}

Use this tool when you discover a stable, verifiable fact during diagnosis that would help future investigations.`
}

func (t *RememberTool) Call(ctx context.Context, input string) (string, error) {
	var ri RememberInput
	if err := json.Unmarshal([]byte(input), &ri); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if ri.Category == "" || ri.Summary == "" || ri.Detail == "" || len(ri.Tags) == 0 {
		return "", fmt.Errorf("category, summary, detail, and tags are required")
	}
	if ri.Confidence == "" {
		ri.Confidence = "medium"
	}
	m := &Memory{
		Category:   ri.Category,
		Summary:    ri.Summary,
		Detail:     ri.Detail,
		Tags:       ri.Tags,
		Confidence: ri.Confidence,
	}
	if err := t.store.Save(ctx, m); err != nil {
		return "", err
	}
	result := fmt.Sprintf(`{"status":"saved","id":"%s"}`, m.ID)
	return result, nil
}

// ---------- factory ----------

// NewTools creates and returns the three memory tools.
func NewTools(s Store) []tools.Tool {
	return []tools.Tool{
		&SearchMemoryTool{store: s},
		&ReadMemoryTool{store: s},
		&RememberTool{store: s},
	}
}

// ---------- alert label extractor ----------

// ExtractTags attempts to extract tags from an Alertmanager-style JSON message.
// It looks for a "labels" object and collects all its values as tags.
func ExtractTags(msg string) []string {
	var raw struct {
		Labels  map[string]string `json:"labels"`
		Status  string            `json:"status"`
		Alerts  []struct {
			Labels map[string]string `json:"labels"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal([]byte(msg), &raw); err != nil {
		return nil
	}
	tagSet := make(map[string]struct{})
	if raw.Labels != nil {
		for _, v := range raw.Labels {
			tagSet[v] = struct{}{}
		}
	}
	for _, alert := range raw.Alerts {
		for _, v := range alert.Labels {
			tagSet[v] = struct{}{}
		}
	}
	if len(tagSet) == 0 {
		return nil
	}
	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	return tags
}

// BuildMemoryContext searches memory for the given tags and formats the
// results as a context block to be injected into the agent prompt.
func BuildMemoryContext(ctx context.Context, store Store, tags []string) string {
	if store == nil || len(tags) == 0 {
		return ""
	}
	items, err := store.Search(ctx, SearchInput{
		Tags:  tags,
		Limit: 8,
	})
	if err != nil || len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## 🧠 环境记忆 (Environment Memory)\n")
	b.WriteString("以下是与本次告警相关的历史情报摘要。使用 SearchMemory / ReadMemory 工具查看更多：\n\n")
	for _, item := range items {
		conf := fmt.Sprintf("%-6s", item.Confidence)
		fmt.Fprintf(&b, "- [%s] [%s] %s (hits:%d)\n",
			conf, item.Category, item.Summary, item.HitCount)
	}
	b.WriteString("\n若发现新的稳定情报，使用 Remember 工具存入以帮助未来诊断。\n")
	return b.String()
}
