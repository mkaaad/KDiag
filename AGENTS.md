# KDiag - Agent Reference

## Overview

**KDiag** is an intelligent alert diagnosis engine that integrates Prometheus, Jaeger, Loki, and Gitea into a unified LLM-driven root cause analysis system — instantly correlating metrics, traces, logs, and code changes to resolve incidents within seconds.

Module: `github.com/mkaaad/kdiag`  
Language: Go 1.26  
Entry point: `kdiag.go`

## Directory Structure

```
kdiag.go              # Public API: NewHandlerFunc (HTTP handler), PollAlerts (background polling)
config/
  config.go           # Config struct with LLM model, tools, addresses, polling interval
  config_test.go      # Unit tests for config defaults and set values
internal/
  agent/
    agent.go          # Diag() - creates and runs the LLM agent with tools; adaptive depth, correlation injection, fingerprint-based similar case retrieval
    prompt.go         # SRE/DevOps alert analysis system prompt (Markdown) with Mermaid fault tree output
  client/
    client.go         # Factory functions for Prometheus, Gitea, Jaeger, Loki clients
  correlation/
    engine.go         # Time-anchored cross-datasource context builder (Prometheus range, Jaeger traces, Loki logs)
  memory/
    model.go          # Memory struct, Category enum (8 categories), Store interface, input/output types
    store.go          # PostgresStore implementing Store with GORM + PostgreSQL
    tools.go          # SearchMemoryTool, ReadMemoryTool, RememberTool + ExtractTags/BuildMemoryContext
  store/
    store.go          # Diagnosis store interface (SaveDiagnosis, SearchByFingerprint, etc.) with Diagnosis/Message types
    postgres.go       # PostgresStore implementing Store interface with GORM + PostgreSQL
    fingerprint.go    # AlertFingerprint (SHA256 of sorted labels) + AlertName extraction
  tool/
    metrics.go        # 4 Prometheus PromQL query tools (Query, QueryRange, LabelName, LabelValue)
    gitea.go          # 8 Gitea API tools (ListOrgs, ListOrgRepos, ListRepoBranches, SearchRepos, GetTree, GetRawFile, ListRepoCommits, GetCommitDiff)
    jaeger.go         # 4 Jaeger trace query tools (GetServices, GetOperations, FindTraces, GetTrace)
    loki.go           # 4 Loki log query tools (QueryRange, QueryInstant, LabelName, LabelValue)
  notify/
    notify.go         # Notification dispatch stub (interface only, no implementations)
```

## Build & Test

- **Build**: `go build ./...`
- **Test**: `go test ./...` (config tests only)
- **Tidy**: `go mod tidy`

## Key Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/tmc/langchaingo` v0.1.14 | LLM agent framework |
| `github.com/prometheus/client_golang` v1.23.2 | Prometheus API client (metrics + correlation engine) |
| `github.com/prometheus/common` v0.62.0 | Prometheus model types (Matrix, Value for correlation) |
| `code.gitea.io/sdk/gitea` v0.24.1 | Gitea API client |
| `golang.org/x/text` v0.36.0 | Language tag (localization) |
| `github.com/google/uuid` v1.6.0 | UUID generation for memory records |
| `gorm.io/gorm` v1.31.1 | PostgreSQL ORM (memory + diagnosis store) |
| `gorm.io/driver/postgres` v1.6.0 | PostgreSQL driver via GORM |

## Tool Inventory

| Package | Tools | Factory |
|---|---|---|
| Prometheus Metrics | `MetricsQueryTool`, `MetricsQueryRangeTool`, `MetricsLabelNameTool`, `MetricsLabelValueTool` | `NewMetricsQueryTool(v1.API)` |
| Gitea | `GiteaListOrgsTool`, `GiteaListOrgReposTool`, `GiteaListRepoBranchesTool`, `GiteaSearchReposTool`, `GiteaGetTreeTool`, `GiteaGetRawFileTool`, `GiteaListRepoCommitsTool`, `GiteaGetCommitDiffTool` | `NewGiteaQueryTool(*gitea.Client)` |
| Jaeger | `JaegerGetServicesTool`, `JaegerGetOperationsTool`, `JaegerFindTracesTool`, `JaegerGetTraceTool` | `NewJaegerQueryTool(*JaegerClient)` |
| Loki | `LokiQueryRangeTool`, `LokiQueryInstantTool`, `LokiLabelNameTool`, `LokiLabelValueTool` | `NewLokiQueryTool(*LokiClient)` |

## Code Patterns & Conventions

### Package Structure
- Public API at module root (`kdiag.go`, `config/`)
- Internal implementation under `internal/`, each with single responsibility

### Naming
- PascalCase for exported, camelCase for unexported
- Short lowercase package names (`agent`, `client`, `tool`)
- Factory functions returning `[]tools.Tool` use no `New` prefix in name (e.g., `NewMetricsQueryTool` returns `[]tools.Tool`)

### Tool Pattern
All tools follow the same pattern:
1. Implement `tools.Tool` interface: `Name()`, `Description()`, `Call(ctx, input)`
2. Accept JSON-encoded query struct as `input`
3. Delegate to underlying API client
4. Return JSON string result or error

Each datasource has a query input struct (`MetricsQuery`, `GiteaQuery`, `JaegerQuery`, `LokiQuery`) with optional fields silently ignored when not used.

### Client Factory Pattern
- `NewPrometheusClient(c)` — panics on failure (core dependency)
- `NewGiteaClient(c)` / `NewJaegerClient(c)` / `NewLokiClient(c)` — silently skip on error (non-critical)
- `NewJaegerClient` and `NewLokiClient` return `nil` immediately if the respective address is empty
- Each factory validates connectivity and appends tools to `c.Tools`

### LLM Agent Pattern
- `Config.Tools` accumulates tools across all client factories
- Two agent modes based on `Config.OpenAIFuncCall`:
  - `true`: `agents.NewOpenAIFunctionsAgent(c.LLM, c.Tools, ...)`
  - `false`: `agents.NewConversationalAgent(c.LLM, c.Tools, ...)`
- System prompt (`AgentPrompt()` in `prompt.go`) defines SRE alert analysis workflow with strict Markdown output, including a `## 🔗 Fault Tree` Mermaid flowchart section
- **Adaptive depth**: `severityFromMsg()` parses severity from Alertmanager JSON; `depthInstruction()` prepends severity-appropriate guidance; `maxIterForSeverity()` sets dynamic iteration limits (critical=15, warning=10, info=5)
- Before each diagnosis, `Diag()` calls `memory.ExtractTags()` to parse alert labels, then `memory.BuildMemoryContext()` to inject relevant memory summaries at the top of the user message
- After memory context, `correlation.BuildContext()` queries Prometheus range, Jaeger traces, and Loki logs around the alert time window and injects results
- Then `store.SearchByFingerprint()` finds similar past diagnoses and injects as a "相似历史案例" context block
- After agent execution, the result is persisted via `c.Store.SaveDiagnosis()` with fingerprint and alert name

### Correlation Engine Pattern
- `internal/correlation/engine.go` uses ad-hoc HTTP clients (not stored on Config)
- `parseTimeRange(msg)` extracts `startsAt` from Alertmanager JSON, builds a 45-minute window (30min before, 15min after)
- `queryPrometheusRange()` queries 5 common SRE metrics with `v1.API.QueryRange()` 1m step
- `queryJaegerTraces()` fetches services list, then searches error traces in top 3 services
- `queryLokiLogs()` discovers label names, then queries `{job=~".+"} |= "error"` with limit 20
- `BuildContext()` orchestrates all three and returns a formatted Markdown section injected into the agent input

### Fingerprint & Store Pattern
- `AlertFingerprint()` computes SHA256 from sorted label key-value pairs (first 16 bytes → 32 hex chars)
- `AlertName()` extracts `alertname` label from top-level or individual alerts
- `Diagnosis` struct includes `AlertName` and `Fingerprint` fields
- `SearchByFingerprint()` uses `LIKE 'prefix%'` for prefix matching on the stored fingerprint
- `SaveDiagnosis()` upserts by `session_id`

### HTTP Handler Pattern
- `NewHandlerFunc` returns `http.HandlerFunc`
- Reads request body, responds immediately with `200 "ok"`, processes async via `agent.Diag`

### Polling Pattern
- `PollAlerts` polls Prometheus alerts on configurable interval (default 24h)
- Creates all clients (Prometheus, Gitea, Jaeger, Loki) and runs alert checks in goroutines

## Known Issues & Gotchas

- `NewHanderFunc` has a typo (missing 'd') — kept for backward compatibility; use `NewHandlerFunc` (correct spelling) for new code
- `Diag()` in `agent/agent.go` runs but output is not routed to any callback/logger (TODO)
- `PollAlerts` has a nil ticker race: `defer ticker.Stop()` executes before `ticker` is assigned in the if/else branches
- Jaeger/Loki/Gitea errors are silently swallowed with `//TODO` comments
- Agent prompt `Language` field in Config is never consumed by the prompt
- `internal/notify/notify.go` is a stub with only the package declaration
- No tests exist for tool implementations or the agent
- `memory.Store` and `memory.PostgresConfig` duplicate `store.PostgresConfig` fields — potential deduplication target
- `Search()` uses PostgreSQL JSONB `??|` operator (requires PG 12+) for tag matching
- LSP may show stale warning for `google/uuid` as indirect after `go mod tidy` — reload clears it
- **Correlation engine creates ad-hoc clients per call** — each `queryPrometheusRange/queryJaegerTraces/queryLokiLogs` creates new HTTP clients. No connection reuse between calls.
- **`queryPrometheusRange` uses hardcoded queries** specific to node_exporter metrics. Won't match container, database, or application-level alerts out of the box.
- **`queryJaegerTraces` uses a hardcoded `{"error":"true"}` tag filter** — may miss non-error traces that are still relevant (e.g., latency, timeout).
- **`queryLokiLogs` uses hardcoded `|= "error"`** — misses non-English error terms (e.g., "异常", "fehler") or events without the literal string "error".
- **`SearchByFingerprint` uses `LIKE` with first 16 chars** — relies on SHA256 prefix; fingerprints shorter than 16 hex chars cause a panic (index out of range).
- **`truncate()` in agent.go returns `s[:n] + "..."`** — can split multi-byte UTF-8 characters (not `[]rune`-safe).
