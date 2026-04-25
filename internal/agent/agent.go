// Package agent provides the LLM agent logic for diagnosing Prometheus alerts.
// It creates and runs either an OpenAI function-calling agent or a conversational
// agent, using the system prompt defined in prompt.go and the tools registered
// in the config.
package agent

import (
	"context"

	"github.com/mkaaad/kdiag/config"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
)

// Diag runs the LLM agent to diagnose the given alert message. It creates the
// appropriate agent type based on config.OpenAIFuncCall, wraps it in an executor
// with the configured maximum iterations, and runs the agent with the alert
// message as input. The result is currently discarded pending proper logging.
func Diag(ctx context.Context, c *config.Config, msg string) string {
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
		//TODO: log the error
		return ""
	}
	// TODO: route the diagnosis answer to a callback, logger, or output sink.
	return answer
}
