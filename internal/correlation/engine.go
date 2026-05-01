package correlation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mkaaad/kdiag/config"
)

// timeRange represents a time window around an alert.
type timeRange struct {
	start time.Time
	end   time.Time
}

// parseTimeRange extracts the alert time window from an Alertmanager JSON message.
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

// BuildContext extracts the alert time window and injects structured metadata
// into the agent message. The agent uses this time anchor to decide which
// metrics, traces, and logs to query — no data is prefetched.
func BuildContext(ctx context.Context, c *config.Config, msg string) string {
	tr := parseTimeRange(msg)
	if tr == nil {
		return ""
	}

	var tools []string
	if c.PrometheusAddress != "" {
		tools = append(tools, "MetricsQueryRange")
	}
	if c.JaegerAddress != "" {
		tools = append(tools, "JaegerFindTraces")
	}
	if c.LokiAddress != "" {
		tools = append(tools, "LokiQueryRange")
	}
	if c.GiteaConfig.ServerURL != "" {
		tools = append(tools, "GiteaListRepoCommits")
	}

	return fmt.Sprintf(`

## ⏰ 时空关联窗口
| 字段 | 值 |
|---|---|
| 告警触发 | %s |
| 查询起点 | %s (告警前 30 分钟) |
| 查询终点 | %s (告警后 15 分钟) |
| 可用工具 | %s |

**策略建议：**
- 根据 alertname、instance、service、namespace 等标签选择对应的指标查询
- 使用 MetricsQueryRange 查询相关指标在窗口内的趋势
- 使用 JaegerFindTraces 搜索受影响服务的错误 traces
- 使用 LokiQueryRange 搜索窗口内的错误/警告日志
- 使用 AddNode 将发现整理到关联信息树中
`,
		tr.start.Format(time.RFC3339),
		tr.start.Format(time.RFC3339),
		tr.end.Format(time.RFC3339),
		joinTools(tools))
}

func joinTools(tools []string) string {
	if len(tools) == 0 {
		return "（无）"
	}
	quoted := make([]string, len(tools))
	for i, t := range tools {
		quoted[i] = "`" + t + "`"
	}
	return strings.Join(quoted, ", ")
}
