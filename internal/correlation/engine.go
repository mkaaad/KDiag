// Package correlation provides time-anchored cross-datasource query orchestration.
// It pre-fetches relevant context from Prometheus, Jaeger, and Loki around the
// alert time window and injects the results into the agent's input message.
package correlation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mkaaad/kdiag/config"
	"github.com/mkaaad/kdiag/internal/tool"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// timeRange represents a time window around an alert.
type timeRange struct {
	start time.Time
	end   time.Time
}

// parseTimeRange extracts the alert time window from an Alertmanager JSON message.
// It looks for startsAt in labels or individual alerts and builds a window
// of 30 minutes before to 15 minutes after.
func parseTimeRange(msg string) *timeRange {
	var raw struct {
		StartsAt string `json:"startsAt"`
		Labels   struct {
			Alertname string `json:"alertname"`
		} `json:"labels"`
		Alerts []struct {
			StartsAt string `json:"startsAt"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal([]byte(msg), &raw); err != nil {
		return nil
	}

	var t time.Time
	if raw.StartsAt != "" {
		t, _ = time.Parse(time.RFC3339, raw.StartsAt)
	}
	if t.IsZero() && len(raw.Alerts) > 0 {
		t, _ = time.Parse(time.RFC3339, raw.Alerts[0].StartsAt)
	}
	if t.IsZero() {
		return nil
	}

	return &timeRange{
		start: t.Add(-30 * time.Minute),
		end:   t.Add(15 * time.Minute),
	}
}

// queryPrometheusRange fetches the top 5 metric time series around the alert window.
func queryPrometheusRange(ctx context.Context, addr string, tr *timeRange) string {
	if addr == "" || tr == nil {
		return ""
	}

	client, err := api.NewClient(api.Config{Address: addr})
	if err != nil {
		return ""
	}
	v1api := v1.NewAPI(client)

	// Query common SRE metrics with a 1m step around the alert window.
	queries := []string{
		`rate(node_cpu_seconds_total{mode="user"}[5m])`,
		`node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes * 100`,
		`rate(node_network_receive_bytes_total[5m])`,
		`node_filesystem_avail_bytes / node_filesystem_size_bytes * 100`,
		`rate(node_disk_io_time_seconds_total[5m])`,
	}

	r := v1.Range{
		Start: tr.start,
		End:   tr.end,
		Step:  60 * time.Second,
	}

	var results []string
	for _, q := range queries {
		result, _, err := v1api.QueryRange(ctx, q, r)
		if err != nil {
			continue
		}
		summary := summarizePromResult(q, result)
		if summary != "" {
			results = append(results, summary)
		}
	}
	if len(results) == 0 {
		return ""
	}
	return "### Prometheus 指标（告警时间窗口）\n" + strings.Join(results, "\n")
}

func summarizePromResult(query string, result model.Value) string {
	switch v := result.(type) {
	case model.Matrix:
		if len(v) == 0 {
			return ""
		}
		// Take first 3 series.
		n := min(3, len(v))
		var parts []string
		for _, ss := range v[:n] {
			labels := ss.Metric.String()
			if len(ss.Values) == 0 {
				continue
			}
			first := ss.Values[0].Value
			last := ss.Values[len(ss.Values)-1].Value
			parts = append(parts, fmt.Sprintf("  - `%s`: avg=%.1f, last=%.1f", labels, float64(first+last)/2, float64(last)))
		}
		if len(parts) == 0 {
			return ""
		}
		return fmt.Sprintf("- Query: `%s`\n%s", query, strings.Join(parts, "\n"))
	default:
		return ""
	}
}

// queryJaegerTraces fetches recent error traces from Jaeger around the alert window.
func queryJaegerTraces(ctx context.Context, addr string, tr *timeRange) string {
	if addr == "" || tr == nil {
		return ""
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	jc := tool.NewJaegerClient(httpClient, addr)

	// First, get all services.
	svcData, err := jc.DoGet(ctx, "/api/services", nil)
	if err != nil {
		return ""
	}
	var svcResp struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(svcData, &svcResp); err != nil || len(svcResp.Data) == 0 {
		return ""
	}

	// Search for error traces in up to 3 services.
	n := min(3, len(svcResp.Data))
	var results []string
	for _, svc := range svcResp.Data[:n] {
		params := url.Values{}
		params.Set("service", svc)
		params.Set("start", fmt.Sprintf("%d", tr.start.UnixMicro()))
		params.Set("end", fmt.Sprintf("%d", tr.end.UnixMicro()))
		params.Set("limit", "5")
		params.Set("tags", `{"error":"true"}`)
		data, err := jc.DoGet(ctx, "/api/traces", params)
		if err != nil {
			continue
		}
		var traceResp struct {
			Data []any `json:"data"`
		}
		if err := json.Unmarshal(data, &traceResp); err != nil {
			continue
		}
		if len(traceResp.Data) > 0 {
			results = append(results, fmt.Sprintf("- `%s`: %d 条错误 Trace", svc, len(traceResp.Data)))
		}
	}
	if len(results) == 0 {
		return ""
	}
	return "### Jaeger 链路追踪（告警时间窗口）\n" + strings.Join(results, "\n")
}

// queryLokiLogs fetches error logs from Loki around the alert window.
func queryLokiLogs(ctx context.Context, addr string, tr *timeRange) string {
	if addr == "" || tr == nil {
		return ""
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	lc := tool.NewLokiClient(httpClient, addr)

	// First, get label names to discover available log streams.
	labelData, err := lc.DoGet(ctx, "/loki/api/v1/label", nil)
	if err != nil {
		return ""
	}
	var labelResp struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(labelData, &labelResp); err != nil || len(labelResp.Data) == 0 {
		return ""
	}

	// Query error logs with a simple {job=~".+"} |= "error" query.
	params := url.Values{}
	params.Set("query", `{job=~".+"} |= "error"`)
	params.Set("start", fmt.Sprintf("%d", tr.start.UnixNano()))
	params.Set("end", fmt.Sprintf("%d", tr.end.UnixNano()))
	params.Set("limit", "20")
	params.Set("direction", "backward")

	data, err := lc.DoGet(ctx, "/loki/api/v1/query_range", params)
	if err != nil {
		return ""
	}
	var logResp struct {
		Data struct {
			Result []any `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &logResp); err != nil {
		return ""
	}
	if len(logResp.Data.Result) == 0 {
		return ""
	}
	return fmt.Sprintf("### Loki 日志（告警时间窗口，含 \"error\"）\n共 %d 条日志流匹配，建议在 Agent 中使用 Loki Query Range Tool 详细查看。", len(logResp.Data.Result))
}

// BuildContext queries all configured datasources around the alert time window
// and returns a formatted context string to inject into the agent message.
func BuildContext(ctx context.Context, c *config.Config, msg string) string {
	tr := parseTimeRange(msg)
	if tr == nil {
		return ""
	}

	var sections []string

	if promCtx := queryPrometheusRange(ctx, c.PrometheusAddress, tr); promCtx != "" {
		sections = append(sections, promCtx)
	}
	if jaegerCtx := queryJaegerTraces(ctx, c.JaegerAddress, tr); jaegerCtx != "" {
		sections = append(sections, jaegerCtx)
	}
	if lokiCtx := queryLokiLogs(ctx, c.LokiAddress, tr); lokiCtx != "" {
		sections = append(sections, lokiCtx)
	}

	if len(sections) == 0 {
		return ""
	}
	return "\n\n## 📡 时空关联上下文（告警时间锚定）\n" + strings.Join(sections, "\n\n") + "\n"
}
