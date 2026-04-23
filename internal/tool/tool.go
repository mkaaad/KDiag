// Package tool provides Prometheus Metrics query tools that implement
// the langchaingo tools.Tool interface, enabling LLM agents to query
// Prometheus metrics, label names, and label values.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/tmc/langchaingo/tools"
)

// MetricsQueryTool allows an LLM agent to query Prometheus instant vectors
// at a specific timestamp via the /api/v1/query endpoint.
type MetricsQueryTool struct {
	api v1.API
}

// MetricsQueryRangeTool allows an LLM agent to query Prometheus range vectors
// over a time window via the /api/v1/query_range endpoint.
type MetricsQueryRangeTool struct {
	api v1.API
}

// MetricsLabelNameTool allows an LLM agent to retrieve Prometheus label names
// within an optional time window via the /api/v1/labels endpoint.
type MetricsLabelNameTool struct {
	api v1.API
}

// MetricsLabelValueTool allows an LLM agent to retrieve values for a given
// Prometheus label within an optional time window via the /api/v1/label/.../values endpoint.
type MetricsLabelValueTool struct {
	api v1.API
}

// MetricsQuery is the input parameter struct that LLM agents must JSON-encode
// when calling any of the Prometheus metrics tools. Not all fields are used
// by every tool; unused fields are silently ignored.
type MetricsQuery struct {
	// PromQL is the PromQL query expression (used by Query and QueryRange tools).
	PromQL string `json:"prom_ql"`
	// TS is the evaluation timestamp for instant queries (used by Query tool).
	TS time.Time `json:"ts"`
	// StartTime is the start of the query range (used by QueryRange, LabelName, LabelValue tools).
	StartTime time.Time `json:"start_time"`
	// EndTime is the end of the query range (used by QueryRange, LabelName, LabelValue tools).
	EndTime time.Time `json:"end_time"`
	// Step is the resolution step duration for range queries (used by QueryRange tool).
	Step time.Duration `json:"step"`
	// LabelNames is an optional filter to restrict label names returned (used by LabelName tool).
	LabelNames []string `json:"label_names"`
	// Label is the label name whose values are queried (used by LabelValue tool).
	Label string `json:"label"`
	// LabelValues is an optional filter to restrict label values returned (used by LabelValue tool).
	LabelValues []string `json:"label_values"`
}

// queryValueToJSON serializes a Prometheus query result (model.Value) together
// with any warnings into a JSON byte slice. It handles Vector, Matrix, Scalar,
// and String value types; other types return an error.
func queryValueToJSON(value model.Value, warning v1.Warnings) ([]byte, error) {
	switch val := value.(type) {
	case model.Vector:
		return json.Marshal(struct {
			Value   model.Vector `json:"value"`
			Warning v1.Warnings  `json:"warning"`
		}{
			val,
			warning,
		})
	case model.Matrix:
		return json.Marshal(struct {
			Value   model.Matrix `json:"value"`
			Warning v1.Warnings  `json:"warning"`
		}{
			val,
			warning,
		})
	case *model.Scalar:
		return json.Marshal(struct {
			Value   *model.Scalar `json:"value"`
			Warning v1.Warnings   `json:"warning"`
		}{
			val,
			warning,
		})
	case *model.String:
		return json.Marshal(struct {
			Value   *model.String `json:"value"`
			Warning v1.Warnings   `json:"warning"`
		}{
			val,
			warning,
		})
	default:
		return nil, errors.New("undefined type")
	}
}

// NewMetricsQueryTool creates and returns the set of Prometheus metrics tools
// that will be registered with the LLM agent.
func NewMetricsQueryTool(api v1.API) []tools.Tool {
	return []tools.Tool{
		&MetricsQueryTool{api: api},
		&MetricsQueryRangeTool{api: api},
		&MetricsLabelNameTool{api: api},
		&MetricsLabelValueTool{api: api},
	}
}

// Name returns the display name of the MetricsQueryTool.
func (m *MetricsQueryTool) Name() string {
	return "Metrics Query Tool"
}

// Description returns a description of what the MetricsQueryTool does.
func (m *MetricsQueryTool) Description() string {
	return `Prometheus instant query tool: evaluates a PromQL expression at a single point in time.

Endpoint: /api/v1/query

Input JSON fields:
- "prom_ql" (string, required): PromQL expression, e.g. "up{job=\"node\"}", "rate(http_requests_total[5m])"
- "ts" (string, required): Evaluation timestamp in RFC3339 format, e.g. "2025-03-15T10:30:00Z"

Example input: {"prom_ql": "avg by(instance) (rate(cpu_seconds_total[5m]))", "ts": "2025-03-15T10:30:00Z"}

Output: JSON object with "value" (instant vector) and "warning" (array of warning strings).

Use this tool to check current metric values, verify alert thresholds, or inspect labels for a specific metric at a given timestamp.`
}

// Call executes a Prometheus instant query at the specified timestamp.
// The input must be a JSON-encoded MetricsQuery struct with prom_ql and ts fields.
// Returns the query result as a JSON string, or an error on failure.
func (m *MetricsQueryTool) Call(ctx context.Context, input string) (string, error) {
	var mq MetricsQuery
	err := json.Unmarshal([]byte(input), &mq)
	if err != nil {
		return "", err
	}
	data, warning, err := m.api.Query(ctx, mq.PromQL, mq.TS)
	if err != nil {
		return "", err
	}
	result, err := queryValueToJSON(data, warning)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

// Name returns the display name of the MetricsQueryRangeTool.
func (m *MetricsQueryRangeTool) Name() string {
	return "Metrics Query Range Tool"
}

// Description returns a description of what the MetricsQueryRangeTool does.
func (m *MetricsQueryRangeTool) Description() string {
	return `Prometheus range query tool: evaluates a PromQL expression over a time window.

Endpoint: /api/v1/query_range

Input JSON fields:
- "prom_ql" (string, required): PromQL expression, e.g. "up{job=\"node\"}", "rate(http_requests_total[5m])"
- "start_time" (string, required): Start of query range in RFC3339 format
- "end_time" (string, required): End of query range in RFC3339 format
- "step" (string, required): Resolution step, e.g. "15s", "1m", "5m"

Example input: {"prom_ql": "avg by(instance) (rate(cpu_seconds_total[5m]))", "start_time": "2025-03-15T10:00:00Z", "end_time": "2025-03-15T11:00:00Z", "step": "1m"}

Output: JSON object with "value" (range matrix) and "warning" (array of warning strings).

Use this tool to retrieve time-series data over a period, analyze trends, compare current vs past metrics, or generate data for graphs.`
}

// Call executes a Prometheus range query over the specified time window.
// The input must be a JSON-encoded MetricsQuery struct with prom_ql, start_time,
// end_time, and step fields. Returns the query result as a JSON string.
func (m *MetricsQueryRangeTool) Call(ctx context.Context, input string) (string, error) {
	var mq MetricsQuery
	err := json.Unmarshal([]byte(input), &mq)
	if err != nil {
		return "", err
	}
	data, warning, err := m.api.QueryRange(ctx, mq.PromQL, v1.Range{
		Start: mq.StartTime,
		End:   mq.EndTime,
		Step:  mq.Step,
	})
	if err != nil {
		return "", err
	}
	result, err := queryValueToJSON(data, warning)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

// Name returns the display name of the MetricsLabelNameTool.
func (m *MetricsLabelNameTool) Name() string {
	return "Metrics Label Name Tool"
}

// Description returns a description of what the MetricsLabelNameTool does.
func (m *MetricsLabelNameTool) Description() string {
	return `Prometheus label name discovery tool: retrieves all label names or restricts to a given set.

Endpoint: /api/v1/labels

Input JSON fields:
- "start_time" (string, optional): Query range start in RFC3339 format
- "end_time" (string, optional): Query range end in RFC3339 format
- "label_names" ([]string, optional): Filter to return only these specific label names if they exist

Example input: {"start_time": "2025-03-15T10:00:00Z", "end_time": "2025-03-15T11:00:00Z"}

Output: JSON object with "label_names" ([]string) and "warning" (array of warning strings).

Use this tool to discover available Prometheus label names, which helps construct more precise PromQL queries.`
}

// Call retrieves Prometheus label names, optionally filtered by a set of
// label names and a time window. The input must be a JSON-encoded MetricsQuery
// struct. Returns label names as a JSON string.
func (m *MetricsLabelNameTool) Call(ctx context.Context, input string) (string, error) {
	var mq MetricsQuery
	err := json.Unmarshal([]byte(input), &mq)
	if err != nil {
		return "", err
	}
	data, warning, err := m.api.LabelNames(ctx, mq.LabelNames, mq.StartTime, mq.EndTime)
	if err != nil {
		return "", err
	}
	combine := struct {
		LabelNames []string    `json:"label_names"`
		Warning    v1.Warnings `json:"warning"`
	}{
		data,
		warning,
	}
	result, err := json.Marshal(combine)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// Name returns the display name of the MetricsLabelValueTool.
func (m *MetricsLabelValueTool) Name() string {
	return "Metrics Label Value Tool"
}

// Description returns a description of what the MetricsLabelValueTool does.
func (m *MetricsLabelValueTool) Description() string {
	return `Prometheus label value discovery tool: retrieves all values for a given label name.

Endpoint: /api/v1/label/:name/values

Input JSON fields:
- "label" (string, required): Label name whose values to query, e.g. "job", "instance", "namespace"
- "start_time" (string, optional): Query range start in RFC3339 format
- "end_time" (string, optional): Query range end in RFC3339 format
- "label_values" ([]string, optional): Filter to return only these specific values if they exist

Example input: {"label": "job", "start_time": "2025-03-15T10:00:00Z", "end_time": "2025-03-15T11:00:00Z"}

Output: JSON object with "label_values" ([]string) and "warning" (array of warning strings).

Use this tool to discover valid label values (e.g., job names, instance names) for constructing more precise PromQL queries.`
}

// Call retrieves values for a given Prometheus label, optionally filtered by
// specific values and a time window. The input must be a JSON-encoded
// MetricsQuery struct with at least the label field. Returns label values as
// a JSON string.
func (m *MetricsLabelValueTool) Call(ctx context.Context, input string) (string, error) {
	var mq MetricsQuery
	err := json.Unmarshal([]byte(input), &mq)
	if err != nil {
		return "", err
	}
	data, warning, err := m.api.LabelValues(ctx, mq.Label, mq.LabelValues, mq.StartTime, mq.EndTime)
	if err != nil {
		return "", err
	}
	combine := struct {
		model.LabelValues `json:"label_values"`
		v1.Warnings       `json:"warning"`
	}{
		data,
		warning,
	}
	result, err := json.Marshal(combine)
	if err != nil {
		return "", err
	}
	return string(result), nil
}
