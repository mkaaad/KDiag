package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ---------- node registry ----------

var (
	expandNodeReg   = map[string]*TreeNode{}
	expandNodeRegMu sync.RWMutex
)

// RegisterExpandNodes stores nodes from the correlation context so
// ExpandNodeTool can look them up by ID.
func RegisterExpandNodes(nodes []*TreeNode) {
	expandNodeRegMu.Lock()
	defer expandNodeRegMu.Unlock()
	for _, n := range nodes {
		expandNodeReg[n.ID] = n
	}
}

func lookupExpandNode(id string) *TreeNode {
	expandNodeRegMu.RLock()
	defer expandNodeRegMu.RUnlock()
	return expandNodeReg[id]
}

// SnapshotNodes returns a shallow copy of all registered tree nodes.
// The caller takes ownership of the returned slice.
func SnapshotNodes() []*TreeNode {
	expandNodeRegMu.RLock()
	defer expandNodeRegMu.RUnlock()
	nodes := make([]*TreeNode, 0, len(expandNodeReg))
	for _, n := range expandNodeReg {
		nodes = append(nodes, n)
	}
	return nodes
}

// ClearNodes removes all registered nodes from the registry.
func ClearNodes() {
	expandNodeRegMu.Lock()
	defer expandNodeRegMu.Unlock()
	clear(expandNodeReg)
}

// ---------- ExpandNode input ----------

// ExpandQuery is the JSON input expected by ExpandNodeTool.Call.
type ExpandQuery struct {
	NodeID string `json:"node_id"`
}

// ---------- ExpandNode tool ----------

// ExpandNodeTool allows the LLM agent to expand a node in the correlation
// information tree. Expanding a trace node fetches all its spans from Jaeger
// and their associated logs from Loki. Expanding a span node fetches its
// details and logs. Expanding a metric/log node returns the relevant data.
type ExpandNodeTool struct {
	name       string
	desc       string
	httpClient *http.Client
	jaegerAddr string
	lokiAddr   string
}

// NewExpandNodeTool creates an ExpandNodeTool that queries Jaeger and Loki
// at the given addresses when expanding nodes.
func NewExpandNodeTool(jaegerAddr, lokiAddr string) *ExpandNodeTool {
	return &ExpandNodeTool{
		name: "ExpandNode",
		desc: `Expand a node in the correlation information tree to see full details.
Input: JSON object with "node_id" (the node ID shown in brackets in the tree).
Examples:
- Expand a trace node → returns all spans with timing and associated logs
- Expand a span node → returns span details + matched log lines
- Expand a metric node → returns full Prometheus query results
- Expand a log stream node → returns matching log lines`,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		jaegerAddr: jaegerAddr,
		lokiAddr:   lokiAddr,
	}
}

func (e *ExpandNodeTool) Name() string       { return e.name }
func (e *ExpandNodeTool) Description() string { return e.desc }

func (e *ExpandNodeTool) Call(ctx context.Context, input string) (string, error) {
	var req ExpandQuery
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return fmt.Sprintf("invalid input: parse node_id failed: %s", err), nil
	}

	node := lookupExpandNode(req.NodeID)
	if node == nil {
		return fmt.Sprintf("node %q not found. It may have expired or the ID is incorrect.", req.NodeID), nil
	}

	switch node.Type {
	case NodeTrace:
		return e.expandTrace(ctx, node)
	case NodeSpan:
		return e.expandSpan(ctx, node)
	case NodeMetric:
		return e.expandMetric(node)
	case NodeLogs:
		return e.expandLogStream(ctx, node)
	default:
		return fmt.Sprintf("unknown node type: %s", node.Type), nil
	}
}

// expandTrace fetches the full trace from Jaeger and associated logs from Loki.
func (e *ExpandNodeTool) expandTrace(ctx context.Context, node *TreeNode) (string, error) {
	if e.jaegerAddr == "" || node.TraceID == "" {
		return "trace expansion requires Jaeger address and trace ID", nil
	}

	jc := NewJaegerClient(e.httpClient, e.jaegerAddr)
	data, err := jc.DoGet(ctx, "/api/traces/"+node.TraceID, nil)
	if err != nil {
		return fmt.Sprintf("failed to fetch trace %s: %s", node.TraceID, err), nil
	}

	var traceResp struct {
		Data []struct {
			TraceID string `json:"traceID"`
			Spans   []struct {
				SpanID        string `json:"spanID"`
				OperationName string `json:"operationName"`
				StartTime     int64  `json:"startTime"`
				Duration      int64  `json:"duration"`
				ProcessID     string `json:"processID"`
				Tags          []struct {
					Key   string `json:"key"`
					Value any    `json:"value"`
				} `json:"tags"`
				Logs []struct {
					Timestamp int64 `json:"timestamp"`
					Fields    []struct {
						Key   string `json:"key"`
						Value any    `json:"value"`
					} `json:"fields"`
				} `json:"logs"`
			} `json:"spans"`
			Processes map[string]struct {
				ServiceName string `json:"serviceName"`
			} `json:"processes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &traceResp); err != nil {
		return fmt.Sprintf("failed to parse trace %s: %s", node.TraceID, err), nil
	}
	if len(traceResp.Data) == 0 {
		return fmt.Sprintf("trace %s not found", node.TraceID), nil
	}

	trace := traceResp.Data[0]
	var b strings.Builder
	fmt.Fprintf(&b, "## Trace: %s\n", trace.TraceID)
	fmt.Fprintf(&b, "Spans: %d\n\n", len(trace.Spans))

	for _, span := range trace.Spans {
		svcName := node.Service
		if p, ok := trace.Processes[span.ProcessID]; ok {
			svcName = p.ServiceName
		}
		fmt.Fprintf(&b, "### Span: %s.%s\n", svcName, span.OperationName)
		fmt.Fprintf(&b, "- Duration: %dμs (%.2fms)\n", span.Duration, float64(span.Duration)/1000)
		fmt.Fprintf(&b, "- SpanID: %s\n", span.SpanID)

		for _, tag := range span.Tags {
			if tag.Key == "error" {
				fmt.Fprintf(&b, "- error: %v\n", tag.Value)
			}
		}

		if logs := e.queryLogsForSpan(ctx, node.TraceID, span.SpanID); logs != "" {
			b.WriteString("- 关联日志:\n")
			b.WriteString(logs)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// expandSpan returns span details and associated logs.
func (e *ExpandNodeTool) expandSpan(ctx context.Context, node *TreeNode) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "## Span: %s\n", node.Summary)
	if node.Service != "" {
		fmt.Fprintf(&b, "- Service: %s\n", node.Service)
	}
	if node.Operation != "" {
		fmt.Fprintf(&b, "- Operation: %s\n", node.Operation)
	}
	if node.TraceID != "" {
		fmt.Fprintf(&b, "- TraceID: %s\n", node.TraceID)
	}
	if node.SpanID != "" {
		fmt.Fprintf(&b, "- SpanID: %s\n", node.SpanID)
	}
	if logs := e.queryLogsForSpan(ctx, node.TraceID, node.SpanID); logs != "" {
		b.WriteString("- 关联日志:\n")
		b.WriteString(logs)
	}
	return b.String(), nil
}

// expandMetric returns the metric query text.
func (e *ExpandNodeTool) expandMetric(node *TreeNode) (string, error) {
	if node.Query == "" {
		return "metric node has no query", nil
	}
	return fmt.Sprintf("## Metric: %s\nQuery: `%s`\n\nUse MetricsQueryRange tool with this PromQL to get full results.\n",
		node.Summary, node.Query), nil
}

// expandLogStream fetches log lines from Loki.
func (e *ExpandNodeTool) expandLogStream(ctx context.Context, node *TreeNode) (string, error) {
	if node.LokiQuery == "" {
		return "log stream node has no query", nil
	}
	if e.lokiAddr == "" {
		return "Loki address not configured", nil
	}

	lc := NewLokiClient(e.httpClient, e.lokiAddr)
	params := url.Values{}
	params.Set("query", node.LokiQuery)
	if node.StartMicro > 0 {
		params.Set("start", fmt.Sprintf("%d", node.StartMicro*1000))
	}
	if node.EndMicro > 0 {
		params.Set("end", fmt.Sprintf("%d", node.EndMicro*1000))
	}
	params.Set("limit", "50")
	params.Set("direction", "backward")

	data, err := lc.DoGet(ctx, "/loki/api/v1/query_range", params)
	if err != nil {
		return fmt.Sprintf("failed to query logs: %s", err), nil
	}

	var logResp struct {
		Data struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &logResp); err != nil {
		return fmt.Sprintf("failed to parse log response: %s", err), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Log Stream: %s\n\n", node.LokiQuery)
	total := 0
	for _, stream := range logResp.Data.Result {
		for _, val := range stream.Values {
			total++
			fmt.Fprintf(&b, "- %s %s\n", val[0], val[1])
			if total >= 50 {
				break
			}
		}
		if total >= 50 {
			break
		}
	}
	if total == 0 {
		b.WriteString("(no matching log lines)\n")
	} else {
		fmt.Fprintf(&b, "\n(%d lines shown)\n", total)
	}
	return b.String(), nil
}

// queryLogsForSpan queries Loki for logs matching traceID and optionally spanID.
func (e *ExpandNodeTool) queryLogsForSpan(ctx context.Context, traceID, spanID string) string {
	if e.lokiAddr == "" || traceID == "" {
		return ""
	}

	lc := NewLokiClient(e.httpClient, e.lokiAddr)
	params := url.Values{}
	q := fmt.Sprintf(`{job=~".+"} |= "%s"`, traceID)
	if spanID != "" {
		q = fmt.Sprintf(`{job=~".+"} |= "%s" |= "%s"`, traceID, spanID)
	}
	params.Set("query", q)
	params.Set("limit", "10")
	params.Set("direction", "backward")

	data, err := lc.DoGet(ctx, "/loki/api/v1/query_range", params)
	if err != nil {
		return ""
	}

	var logResp struct {
		Data struct {
			Result []struct {
				Values [][]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &logResp); err != nil {
		return ""
	}

	var lines []string
	for _, stream := range logResp.Data.Result {
		for _, val := range stream.Values {
			lines = append(lines, fmt.Sprintf("  > %s: %s", val[0], val[1]))
			if len(lines) >= 10 {
				break
			}
		}
		if len(lines) >= 10 {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
