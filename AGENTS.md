# KDiag - Agent Reference

## Overview

**KDiag** is a Go library that receives Prometheus Alertmanager webhooks and uses an LLM agent (via `github.com/tmc/langchaingo`) to diagnose alerts. It provides a `net/http` handler and a polling-based alert ingestion mechanism.

Module: `github.com/mkaaad/kdiag`  
Language: Go 1.25.0  
Entry point: `kdiag.go`

## Directory Structure

```
kdiag.go              # Public API: NewHanderFunc (HTTP handler), PollAlerts (background polling)
config/config.go      # Config struct with LLM model reference, tool list, language, Prometheus address, polling interval
internal/
  agent/agent.go      # Diag() function - creates and runs the LLM agent (currently incomplete)
  agent/prompt.go     # Long system prompt for the SRE/DevOps alert analysis agent
  client/client.go    # Prometheus API client factory (validates connection via Buildinfo)
  model/model.go      # Alertmanager webhook + Alert data structures (JSON tags)
  tool/tool.go        # 4 Prometheus query tools implementing tools.Tool interface
```

## Build & Test

- **Build**: `go build ./...`
- **Test**: `go test ./...` (currently no tests exist)
- **Run**: Not a standalone binary - library intended for import
- **Tidy dependencies**: `go mod tidy`

## Key Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/tmc/langchaingo` v0.1.14 | LLM agent framework (agents, llms, tools sub-packages) |
| `github.com/prometheus/client_golang` v1.23.2 | Prometheus API client (v1 sub-package) |
| `golang.org/x/text` v0.36.0 | Language tag (for localization) |

## Code Patterns & Conventions

### Package Structure
- Public API lives at the module root (`kdiag.go`, `config/`)
- Internal implementation lives under `internal/`
- Each internal package is self-contained with a single responsibility

### Naming
- PascalCase for exported identifiers
- camelCase for unexported
- Package names are short and lowercase (`agent`, `client`, `model`, `tool`)
- No `New` prefix on factory functions that return slices (e.g., `NewMetricsQueryTool` returns `[]tools.Tool`)

### Error Handling
- `panic(err)` on invalid config/Prometheus connection (top-level API)
- TODO markers in place of proper error handling in several places (search for `//TODO`)
- Tools return `(string, error)` per the `tools.Tool` interface

### Prometheus Tools Pattern
All 4 tools follow the same pattern:
1. Implement `tools.Tool` interface: `Name() string`, `Description() string`, `Call(ctx, input string) (string, error)`
2. Accept JSON-encoded `MetricsQuery` struct as `input`
3. Delegate to `prometheus/v1.API` methods
4. Serialize results to JSON with embedded warnings
5. Helper `queryValueToJSON` handles `model.Vector`, `model.Matrix`, `*model.Scalar`, `*model.String`

### LLM Agent Pattern
- `config.Config` holds the LLM model reference (`llms.Model`) and tool list (`[]tools.Tool`)
- Two agent modes based on `Config.OpenAIFuncCall`:
  - `true`: `agents.NewOpenAIFunctionsAgent(c.LLM, c.Tools)`
  - `false`: `agents.NewConversationalAgent(c.LLM, c.Tools)`
- The system prompt (`agent.prompt.go`) is a detailed SRE alert analysis prompt with a strict Markdown output format

### HTTP Handler Pattern
- `NewHanderFunc` returns `http.HandlerFunc`
- Reads full request body, responds immediately with `200 "ok"`, then processes asynchronously via `agent.Diag`
- Runs in background goroutine (does not block the HTTP response)

### Polling Pattern
- `PollAlerts` polls Prometheus alerts on a configurable interval (`Config.PollingInterval`)
- Defaults to 24h if interval is 0
- Runs alerts in separate goroutines

## Known Compilation Issues (as of initial commit)

1. **`kdiag.go:25`**: `NewMetricsQueryTool` returns `[]tools.Tool` but is appended as a single `tools.Tool` — type mismatch
2. **`agent/agent.go:13`**: `agent` variable declared but never used
3. **`agent/agent.go:17`**: `agents.NewConversationalAgent()` called with wrong args (needs `llms.Model, []tools.Tool` at minimum)
4. **`agent/agent.go:8-9`**: Unused imports (`llms`, `tools`)
5. **No tests exist** — any new code should include tests
6. **`//TODO` comments** in several places for error handling
7. **Typo**: `NewHanderFunc` (missing 'd' in "Handler") — existing public API name, should be kept for backward compatibility

## Gotchas

- `NewHanderFunc` has a typo — "Hander" not "Handler". This is the exported name.
- The `Diag` function in `agent/agent.go` is a stub — it creates an agent but never calls it.
- `agentDiag` runs via `go alert()` / `go func()` — errors are silently swallowed via empty `//TODO` blocks.
- `PollAlerts` has a nil ticker issue: `defer ticker.Stop()` runs immediately if `ticker` from `time.NewTicker` is not assigned before the defers in the if/else branches could cause a nil pointer dereference on the defer.
- The `Language` field in Config uses `golang.org/x/text/language.Tag` but is never read by the agent prompt.
- Prometheus address validation happens at startup via `Buildinfo` call with a 3-second timeout.
- Agent prompt is embedded as a Go constant string in `internal/agent/prompt.go`.
