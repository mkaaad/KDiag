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
    agent.go          # Diag() - creates and runs the LLM agent with tools
    prompt.go         # SRE/DevOps alert analysis system prompt (Markdown)
  client/
    client.go         # Factory functions for Prometheus, Gitea, Jaeger, Loki clients
  memory/
    model.go          # Memory struct, Category enum (8 categories), Store interface, input/output types
    store.go          # PostgresStore implementing Store with GORM + PostgreSQL
    tools.go          # SearchMemoryTool, ReadMemoryTool, RememberTool + ExtractTags/BuildMemoryContext
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
| `github.com/prometheus/client_golang` v1.23.2 | Prometheus API client |
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
- System prompt (`agentPrompt` in `prompt.go`) defines SRE alert analysis workflow with strict Markdown output
- Before each diagnosis, `Diag()` calls `memory.ExtractTags()` to parse alert labels, then `memory.BuildMemoryContext()` to inject relevant memory summaries at the top of the user message

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
