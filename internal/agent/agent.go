// Package agent provides the LLM agent logic for diagnosing Prometheus alerts.
// It creates and runs either an OpenAI function-calling agent or a conversational
// agent, using the system prompt defined in prompt.go and the tools registered
// in the config.
package agent

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/mkaaad/kdiag/config"
	"github.com/mkaaad/kdiag/internal/memory"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
)

// Diag runs the LLM agent to diagnose the given alert message. It creates the
// appropriate agent type based on config.OpenAIFuncCall, wraps it in an executor
// with the configured maximum iterations, and runs the agent with the alert
// message as input. The result is currently discarded pending proper logging.
func Diag(ctx context.Context, c *config.Config, msg string) string {
	// Build memory context from the alert and prepend to user message.
	if c.MemoryStore != nil {
		if tags := memory.ExtractTags(msg); len(tags) > 0 {
			if memCtx := memory.BuildMemoryContext(ctx, c.MemoryStore, tags); memCtx != "" {
				msg = memCtx + "\n\n---\n\n" + msg
			}
		}
	}

	// Choose agent type based on configuration.
	var agent agents.Agent
	if c.OpenAIFuncCall {
		sysMsgOpt := agents.NewOpenAIOption().
			WithSystemMessage(agentPrompt)
		agent = agents.NewOpenAIFunctionsAgent(c.LLM, c.Tools, sysMsgOpt)
	} else {
		agent = agents.NewConversationalAgent(c.LLM, c.Tools, agents.WithPromptPrefix(agentPrompt))
	}
	// Create an executor with the configured iteration limit.
	executor := agents.NewExecutor(agent, agents.WithMaxIterations(c.MaxIterations))
	answer, err := chains.Run(ctx, executor, msg)
	if err != nil {
		return ""
	}
	// Persist the diagnosis result if a store is configured.
	if c.Store != nil {
		// Derive a deterministic session ID from the alert content via SHA256.
		sessionID := fmt.Sprintf("alert-%x", sha256.Sum256([]byte(msg)))
		_ = c.Store.SaveDiagnosis(ctx, sessionID, msg, answer)
	}
	return answer
}
