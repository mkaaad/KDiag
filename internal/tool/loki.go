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

// LokiQuery is the input parameter struct that LLM agents must JSON-encode
// when calling any of the Loki tools. Not all fields are used by every tool;
// unused fields are silently ignored.
type LokiQuery struct {
	LogQL     string            `json:"logql"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time"`
	Time      time.Time         `json:"time"`
	Step      string            `json:"step"`
	Limit     int               `json:"limit"`
	Label     string            `json:"label"`
	Direction string            `json:"direction"` // "forward" or "backward"
	Regexp    string            `json:"regexp"`
	Labels    map[string]string `json:"labels"`
}

// ---------- Loki HTTP client ----------

// LokiClient is a simple HTTP client for the Loki query frontend API.
type LokiClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewLokiClient creates a new LokiClient pointing at the given base URL.
func NewLokiClient(httpClient *http.Client, baseURL string) *LokiClient {
	return &LokiClient{httpClient: httpClient, baseURL: baseURL}
}

// DoGet performs a GET request against the Loki API and returns the raw JSON
// response body.
func (l *LokiClient) DoGet(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
	u, err := url.Parse(l.baseURL + endpoint)
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
	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki API returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// ---------- Query Range ----------

// LokiQueryRangeTool allows an LLM agent to run a LogQL range query over a
// time window.
type LokiQueryRangeTool struct {
	client *LokiClient
}

func (t *LokiQueryRangeTool) Name() string {
	return "Loki Query Range Tool"
}

func (t *LokiQueryRangeTool) Description() string {
	return `Loki tool: run a LogQL range query over a time window.

Endpoint: GET /loki/api/v1/query_range

Input JSON fields:
- "logql" (string, required): LogQL query expression, e.g. '{job="nginx"} |= "error"', 'rate({job="nginx"}[5m])'
- "start_time" (string, optional): Query range start in RFC3339 format
- "end_time" (string, optional): Query range end in RFC3339 format
- "step" (string, optional): Query resolution step, e.g. "15s", "1m", "5m"
- "limit" (int, optional): Maximum number of log entries to return (default 100)
- "direction" (string, optional): Log sorting direction — "forward" or "backward" (default "backward")
- "regexp" (string, optional): Additional regexp filter for log line content

Example input: {"logql": "{job=\"nginx\",namespace=\"production\"} |= \"500\"", "start_time": "2025-03-15T10:00:00Z", "end_time": "2025-03-15T11:00:00Z", "limit": 50}

Output: JSON object with log query results including timestamps, log lines, and labels.

Use this tool to search logs by labels, filter by content, and analyze log patterns over a time window.`
}

func (t *LokiQueryRangeTool) Call(ctx context.Context, input string) (string, error) {
	var q LokiQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Set("query", q.LogQL)
	if !q.StartTime.IsZero() {
		params.Set("start", fmt.Sprintf("%d", q.StartTime.UnixNano()))
	}
	if !q.EndTime.IsZero() {
		params.Set("end", fmt.Sprintf("%d", q.EndTime.UnixNano()))
	}
	if q.Step != "" {
		params.Set("step", q.Step)
	}
	if q.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", q.Limit))
	}
	if q.Direction != "" {
		params.Set("direction", q.Direction)
	}
	if q.Regexp != "" {
		params.Set("regexp", q.Regexp)
	}
	data, err := t.client.DoGet(ctx, "/loki/api/v1/query_range", params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Query Instant ----------

// LokiQueryInstantTool allows an LLM agent to run an instant LogQL query at a
// specific point in time.
type LokiQueryInstantTool struct {
	client *LokiClient
}

func (t *LokiQueryInstantTool) Name() string {
	return "Loki Query Instant Tool"
}

func (t *LokiQueryInstantTool) Description() string {
	return `Loki tool: run an instant LogQL query at a specific point in time.

Endpoint: GET /loki/api/v1/query

Input JSON fields:
- "logql" (string, required): LogQL query expression, e.g. '{job="nginx"} |= "error"'
- "time" (string, optional): Evaluation timestamp in RFC3339 format; defaults to current time
- "limit" (int, optional): Maximum number of log entries to return (default 100)
- "direction" (string, optional): Log sorting direction — "forward" or "backward" (default "backward")

Example input: {"logql": "{job=\"nginx\"} |= \"500\"", "limit": 50}

Output: JSON object with instant log query results.

Use this tool to quickly check current logs matching specific labels and filters.`
}

func (t *LokiQueryInstantTool) Call(ctx context.Context, input string) (string, error) {
	var q LokiQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Set("query", q.LogQL)
	if !q.Time.IsZero() {
		params.Set("time", fmt.Sprintf("%d", q.Time.UnixNano()))
	}
	if q.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", q.Limit))
	}
	if q.Direction != "" {
		params.Set("direction", q.Direction)
	}
	data, err := t.client.DoGet(ctx, "/loki/api/v1/query", params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Label Names ----------

// LokiLabelNameTool allows an LLM agent to retrieve label names from Loki
// within an optional time window.
type LokiLabelNameTool struct {
	client *LokiClient
}

func (t *LokiLabelNameTool) Name() string {
	return "Loki Label Name Tool"
}

func (t *LokiLabelNameTool) Description() string {
	return `Loki tool: retrieve all label names within an optional time window.

Endpoint: GET /loki/api/v1/label

Input JSON fields:
- "start_time" (string, optional): Query range start in RFC3339 format
- "end_time" (string, optional): Query range end in RFC3339 format

Example input: {"start_time": "2025-03-15T10:00:00Z", "end_time": "2025-03-15T11:00:00Z"}

Output: JSON object with "data" array of label names.

Use this tool to discover available Loki label names before constructing LogQL queries.`
}

func (t *LokiLabelNameTool) Call(ctx context.Context, input string) (string, error) {
	var q LokiQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	if !q.StartTime.IsZero() {
		params.Set("start", fmt.Sprintf("%d", q.StartTime.UnixNano()))
	}
	if !q.EndTime.IsZero() {
		params.Set("end", fmt.Sprintf("%d", q.EndTime.UnixNano()))
	}
	data, err := t.client.DoGet(ctx, "/loki/api/v1/label", params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Label Values ----------

// LokiLabelValueTool allows an LLM agent to retrieve values for a given label
// within an optional time window.
type LokiLabelValueTool struct {
	client *LokiClient
}

func (t *LokiLabelValueTool) Name() string {
	return "Loki Label Value Tool"
}

func (t *LokiLabelValueTool) Description() string {
	return `Loki tool: retrieve values for a given label name within an optional time window.

Endpoint: GET /loki/api/v1/label/{name}/values

Input JSON fields:
- "label" (string, required): Label name whose values to query, e.g. "job", "namespace", "container"
- "start_time" (string, optional): Query range start in RFC3339 format
- "end_time" (string, optional): Query range end in RFC3339 format

Example input: {"label": "job", "start_time": "2025-03-15T10:00:00Z", "end_time": "2025-03-15T11:00:00Z"}

Output: JSON object with "data" array of label values.

Use this tool to discover valid label values (e.g., job names, instance names) for constructing precise LogQL queries.`
}

func (t *LokiLabelValueTool) Call(ctx context.Context, input string) (string, error) {
	var q LokiQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	if !q.StartTime.IsZero() {
		params.Set("start", fmt.Sprintf("%d", q.StartTime.UnixNano()))
	}
	if !q.EndTime.IsZero() {
		params.Set("end", fmt.Sprintf("%d", q.EndTime.UnixNano()))
	}
	data, err := t.client.DoGet(ctx, "/loki/api/v1/label/"+url.PathEscape(q.Label)+"/values", params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- factory ----------

// NewLokiQueryTool creates and returns the set of Loki log query tools that
// will be registered with the LLM agent.
func NewLokiQueryTool(client *LokiClient) []tools.Tool {
	return []tools.Tool{
		&LokiQueryRangeTool{client: client},
		&LokiQueryInstantTool{client: client},
		&LokiLabelNameTool{client: client},
		&LokiLabelValueTool{client: client},
	}
}
