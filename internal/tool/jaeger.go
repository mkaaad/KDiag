package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/tmc/langchaingo/tools"
)

// ---------- query input struct ----------

// JaegerQuery is the input parameter struct that LLM agents must JSON-encode
// when calling any of the Jaeger tools. Not all fields are used by every tool;
// unused fields are silently ignored.
type JaegerQuery struct {
	Service    string            `json:"service"`
	Operation  string            `json:"operation"`
	TraceID    string            `json:"trace_id"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time"`
	Limit      int               `json:"limit"`
	Lookback   string            `json:"lookback"`
	MaxDuration string           `json:"max_duration"`
	MinDuration string           `json:"min_duration"`
	Tags       map[string]string `json:"tags"`
}

// ---------- Jaeger HTTP client ----------

// JaegerClient is a simple HTTP client for the Jaeger query service API.
type JaegerClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewJaegerClient creates a new JaegerClient pointing at the given base URL.
func NewJaegerClient(httpClient *http.Client, baseURL string) *JaegerClient {
	return &JaegerClient{httpClient: httpClient, baseURL: baseURL}
}

// DoGet performs a GET request against the Jaeger query API and returns the
// raw JSON response body.
func (j *JaegerClient) DoGet(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
	u, err := url.Parse(j.baseURL + endpoint)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jaeger API returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// ---------- Get Services ----------

// JaegerGetServicesTool allows an LLM agent to list all services registered in
// Jaeger.
type JaegerGetServicesTool struct {
	client *JaegerClient
}

func (t *JaegerGetServicesTool) Name() string {
	return "Jaeger Get Services Tool"
}

func (t *JaegerGetServicesTool) Description() string {
	return `Jaeger tool: list all services that have registered traces.

Input: no fields required (send an empty JSON object {}).

Example input: {}

Output: JSON array of service names.

Use this tool to discover which services are tracked by Jaeger before querying traces.`
}

func (t *JaegerGetServicesTool) Call(ctx context.Context, input string) (string, error) {
	data, err := t.client.DoGet(ctx, "/api/services", nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Get Operations ----------

// JaegerGetOperationsTool allows an LLM agent to list operations for a given
// service.
type JaegerGetOperationsTool struct {
	client *JaegerClient
}

func (t *JaegerGetOperationsTool) Name() string {
	return "Jaeger Get Operations Tool"
}

func (t *JaegerGetOperationsTool) Description() string {
	return `Jaeger tool: list all operations for a given service.

Input JSON fields:
- "service" (string, required): Service name, e.g. "frontend", "checkout-service"

Example input: {"service": "frontend"}

Output: JSON array of operation objects with name and spanKind fields.

Use this tool to discover available operations for a service before filtering traces by operation.`
}

func (t *JaegerGetOperationsTool) Call(ctx context.Context, input string) (string, error) {
	var q JaegerQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	data, err := t.client.DoGet(ctx, "/api/services/"+url.PathEscape(q.Service)+"/operations", nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Find Traces ----------

// JaegerFindTracesTool allows an LLM agent to search for traces by service,
// time range, tags, and other filters.
type JaegerFindTracesTool struct {
	client *JaegerClient
}

func (t *JaegerFindTracesTool) Name() string {
	return "Jaeger Find Traces Tool"
}

func (t *JaegerFindTracesTool) Description() string {
	return `Jaeger tool: search for traces with filters.

Input JSON fields:
- "service" (string, required): Service name to search traces for
- "operation" (string, optional): Operation name to filter by
- "start_time" (string, optional): Search window start in RFC3339 format
- "end_time" (string, optional): Search window end in RFC3339 format
- "limit" (int, optional): Maximum number of traces to return (default 20)
- "lookback" (string, optional): Lookback duration relative to now (e.g., "1h", "30m")
- "max_duration" (string, optional): Maximum trace duration (e.g., "2s", "500ms")
- "min_duration" (string, optional): Minimum trace duration (e.g., "1s", "100ms")
- "tags" (object, optional): Key-value pairs of span tags to filter (e.g., {"error":"true","http.status_code":"500"})

At least "service" must be provided. If start_time/end_time is empty and lookback is set, it searches the lookback window.

Example input: {"service": "frontend", "operation": "GET /api/checkout", "limit": 10, "tags": {"error": "true"}, "lookback": "1h"}

Output: JSON object containing trace search results with trace IDs, service names, operation names, durations, and span counts.

Use this tool to find traces matching specific criteria, error conditions, or time windows.`
}

func (t *JaegerFindTracesTool) Call(ctx context.Context, input string) (string, error) {
	var q JaegerQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Set("service", q.Service)
	if q.Operation != "" {
		params.Set("operation", q.Operation)
	}
	if q.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", q.Limit))
	}
	if q.Lookback != "" {
		params.Set("lookback", q.Lookback)
	}
	if q.MaxDuration != "" {
		params.Set("maxDuration", q.MaxDuration)
	}
	if q.MinDuration != "" {
		params.Set("minDuration", q.MinDuration)
	}
	if !q.StartTime.IsZero() {
		params.Set("start", fmt.Sprintf("%d", q.StartTime.UnixMicro()))
	}
	if !q.EndTime.IsZero() {
		params.Set("end", fmt.Sprintf("%d", q.EndTime.UnixMicro()))
	}
	if len(q.Tags) > 0 {
		tagStr, err := json.Marshal(q.Tags)
		if err != nil {
			return "", err
		}
		params.Set("tags", string(tagStr))
	}
	data, err := t.client.DoGet(ctx, "/api/traces", params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Get Trace ----------

// JaegerGetTraceTool allows an LLM agent to retrieve the full detail of a
// single trace by its trace ID.
type JaegerGetTraceTool struct {
	client *JaegerClient
}

func (t *JaegerGetTraceTool) Name() string {
	return "Jaeger Get Trace Tool"
}

func (t *JaegerGetTraceTool) Description() string {
	return `Jaeger tool: retrieve the full detail of a single trace by trace ID.

Input JSON fields:
- "trace_id" (string, required): Trace ID (hex string), e.g. "abc123def4567890"

Example input: {"trace_id": "abc123def4567890"}

Output: JSON object with the full trace spans, service names, operation names, timings, and tags.

Use this tool to inspect the complete span tree of a specific trace to understand request paths and latency breakdowns.`
}

func (t *JaegerGetTraceTool) Call(ctx context.Context, input string) (string, error) {
	var q JaegerQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	data, err := t.client.DoGet(ctx, "/api/traces/"+url.PathEscape(q.TraceID), nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- factory ----------

// NewJaegerQueryTool creates and returns the set of Jaeger query tools that
// will be registered with the LLM agent.
func NewJaegerQueryTool(client *JaegerClient) []tools.Tool {
	return []tools.Tool{
		&JaegerGetServicesTool{client: client},
		&JaegerGetOperationsTool{client: client},
		&JaegerFindTracesTool{client: client},
		&JaegerGetTraceTool{client: client},
	}
}
