// Package agent provides the LLM agent logic for diagnosing Prometheus alerts.
// It creates and runs either an OpenAI function-calling agent or a conversational
// agent, using the system prompt defined in prompt.go and the tools registered
// in the config.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mkaaad/kdiag/config"
	"github.com/mkaaad/kdiag/internal/correlation"
	"github.com/mkaaad/kdiag/internal/memory"
	"github.com/mkaaad/kdiag/internal/store"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
)

// severityFromMsg extracts the severity label from an Alertmanager JSON message.
func severityFromMsg(msg string) string {
	var raw struct {
		Labels struct {
			Severity string `json:"severity"`
		} `json:"labels"`
		Alerts []struct {
			Labels struct {
				Severity string `json:"severity"`
			} `json:"labels"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal([]byte(msg), &raw); err != nil {
		return ""
	}
	if raw.Labels.Severity != "" {
		return raw.Labels.Severity
	}
	for _, a := range raw.Alerts {
		if a.Labels.Severity != "" {
			return a.Labels.Severity
		}
	}
	return ""
}

// depthInstruction returns a severity-adaptive instruction prepended to the agent input.
func depthInstruction(severity string) string {
	switch severity {
	case "critical":
		return "\n[诊断深度: 全面调查] 此告警为 Critical 级别，请进行最彻底的根因分析。使用所有可用工具深入调查，分析每一个可能的原因，并提供详细的修复步骤。\n"
	case "warning":
		return "\n[诊断深度: 标准调查] 此告警为 Warning 级别，请进行标准深度的分析。聚焦最可能的原因，提供清晰的解决步骤。\n"
	case "info":
		return "\n[诊断深度: 快速评估] 此告警为 Info 级别，请进行快速评估。简要分析可能原因即可，无需深入调查。\n"
	default:
		return "\n[诊断深度: 标准调查] 请按默认深度进行诊断分析。\n"
	}
}

// maxIterForSeverity returns the recommended max agent iterations for a severity level.
func maxIterForSeverity(severity string, configured int) int {
	if configured > 0 {
		return configured
	}
	switch severity {
	case "critical":
		return 15
	case "warning":
		return 10
	case "info":
		return 5
	default:
		return 10
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Diag runs the LLM agent to diagnose the given alert message. It creates the
// appropriate agent type based on config.OpenAIFuncCall, wraps it in an executor
// with the configured maximum iterations, and runs the agent with the alert
// message as input. The result is currently discarded pending proper logging.
func Diag(ctx context.Context, c *config.Config, msg string) string {
	// Determine alert severity and adapt diagnosis depth.
	severity := severityFromMsg(msg)

	// Build memory context from the alert and prepend to user message.
	if c.MemoryStore != nil {
		if tags := memory.ExtractTags(msg); len(tags) > 0 {
			if memCtx := memory.BuildMemoryContext(ctx, c.MemoryStore, tags); memCtx != "" {
				msg = memCtx + "\n\n---\n\n" + msg
			}
		}
	}

	// Inject depth instruction based on severity.
	msg = depthInstruction(severity) + "\n" + msg

	// Build time-anchored cross-datasource context and inject into message.
	if corrCtx := correlation.BuildContext(ctx, c, msg); corrCtx != "" {
		msg = msg + corrCtx
	}

	// Search for similar past diagnoses by fingerprint and inject into message.
	if c.Store != nil {
		if fp := store.AlertFingerprint(msg); fp != "" {
			if similar, err := c.Store.SearchByFingerprint(ctx, fp, 3); err == nil && len(similar) > 0 {
				var sb strings.Builder
				sb.WriteString("\n\n## 📋 相似历史案例（指纹匹配）\n以下是与当前告警指纹相似的历史诊断记录：\n\n")
				for _, d := range similar {
					fmt.Fprintf(&sb, "- **%s** (%s): %s\n", d.AlertName, d.CreatedAt.Format("2006-01-02 15:04"), truncate(d.Diagnosis, 120))
				}
				sb.WriteString("\n参考历史案例有助于更快定位根因。\n")
				msg = msg + sb.String()
			}
		}
		// Semantic search via vector embedding if an embedder is configured.
		if c.Embedder != nil {
			vecF64, err := c.Embedder.EmbedQuery(ctx, msg)
			if err == nil && len(vecF64) > 0 {
				vec := make([]float32, len(vecF64))
				for i, v := range vecF64 {
					vec[i] = float32(v)
				}
				if similar, err := c.Store.SearchByVector(ctx, vec, 3); err == nil && len(similar) > 0 {
					var sb strings.Builder
					sb.WriteString("\n\n## 🔍 语义相似历史案例\n以下是与当前告警语义相似的历史诊断记录（基于向量检索）：\n\n")
					for _, d := range similar {
						dist := ""
						if d.Distance > 0 {
							dist = fmt.Sprintf(" (距离: %.4f)", d.Distance)
						}
						fmt.Fprintf(&sb, "- **%s** (%s)%s: %s\n", d.AlertName, d.CreatedAt.Format("2006-01-02 15:04"), dist, truncate(d.Diagnosis, 120))
					}
					sb.WriteString("\n语义相似的案例可能涉及不同告警但根因相同，值得参考。\n")
					msg = msg + sb.String()
				}
			}
		}
	}

	// Choose agent type based on configuration.
	var agent agents.Agent
	if c.OpenAIFuncCall {
		sysMsgOpt := agents.NewOpenAIOption().
			WithSystemMessage(AgentPrompt(c.Language.String()))
		agent = agents.NewOpenAIFunctionsAgent(c.LLM, c.Tools, sysMsgOpt)
	} else {
		agent = agents.NewConversationalAgent(c.LLM, c.Tools, agents.WithPromptPrefix(AgentPrompt(c.Language.String())))
	}
	// Create an executor with the adaptive iteration limit.
	maxIter := maxIterForSeverity(severity, c.MaxIterations)
	executor := agents.NewExecutor(agent, agents.WithMaxIterations(maxIter))
	answer, err := chains.Run(ctx, executor, msg)
	if err != nil {
		return ""
	}
	// Persist the diagnosis result if a store is configured.
	if c.Store != nil {
		// Derive a deterministic session ID from the alert content via SHA256.
		sessionID := fmt.Sprintf("alert-%x", sha256.Sum256([]byte(msg)))
		fp := store.AlertFingerprint(msg)
		an := store.AlertName(msg)

		// Compute embedding of the diagnosis output for future vector search.
		var emb []float32
		if c.Embedder != nil {
			vecF64, err := c.Embedder.EmbedQuery(ctx, answer)
			if err == nil && len(vecF64) > 0 {
				emb = make([]float32, len(vecF64))
				for i, v := range vecF64 {
					emb[i] = float32(v)
				}
			}
		}
		_ = c.Store.SaveDiagnosis(ctx, sessionID, fp, an, msg, answer, emb)
	}
	return answer
}
