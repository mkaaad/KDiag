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
func Diag(ctx context.Context, c *config.Config, msg string) {
	// Choose agent type based on configuration.
	var agent agents.Agent
	if c.OpenAIFuncCall {
		agent = agents.NewOpenAIFunctionsAgent(c.LLM, c.Tools)
	} else {
		agent = agents.NewConversationalAgent(c.LLM, c.Tools)
	}
	// Create an executor with the configured iteration limit.
	executor := agents.NewExecutor(agent, agents.WithMaxIterations(c.MaxIterations))
	// Run the agent with the alert message. The input string is intentionally
	// empty here (TODO); it should be the alert message for diagnosis.
	answer, err := chains.Run(ctx, executor, "") //TODO: pass msg as input
	if err != nil {
		//TODO: log the error
		return
	}
	// TODO: route the diagnosis answer to a callback, logger, or output sink.
	_ = answer
}
