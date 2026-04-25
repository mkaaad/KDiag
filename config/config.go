// Package config defines shared configuration types for the KDiag alert
// diagnosis library, including LLM model reference, tool registry, and
// Prometheus connection settings.
package config

import (
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
	"golang.org/x/text/language"
)

// Config holds the configuration for the Alertmanager webhook handler.
type Config struct {
	// LLM is the language model instance used by the agent for diagnosis.
	LLM llms.Model
	// OpenAIFuncCall selects the agent type: true for OpenAI function-calling
	// agent, false for conversational agent.
	OpenAIFuncCall bool
	// Tools is the list of tools registered with the LLM agent (e.g., Prometheus
	// query tools). Tools are appended by NewHandlerFunc and PollAlerts.
	Tools []tools.Tool
	// Language specifies the preferred language for agent output.
	Language language.Tag
	// PrometheusAddress is the base URL of the Prometheus server (e.g.,
	// "http://localhost:9090").
	PrometheusAddress string
	// PollingInterval controls how often PollAlerts fetches alerts from
	// Prometheus. If zero, defaults to 24 hours.
	PollingInterval time.Duration
	// MaxIterations limits the number of LLM agent reasoning iterations.
	MaxIterations int
	// GiteaConfig is the configuration for the Gitea client, including
	// the server URL and authentication token.
	GiteaConfig GiteaConfig
	// JaegerAddress is the base URL of the Jaeger query service (e.g.,
	// "http://localhost:16686"). If empty, Jaeger tools are not registered.
	JaegerAddress string
	// LokiAddress is the base URL of the Loki query frontend (e.g.,
	// "http://localhost:3100"). If empty, Loki tools are not registered.
	LokiAddress string
}

// GiteaConfig holds the connection details for the Gitea API client.
type GiteaConfig struct {
	// ServerURL is the base URL of the Gitea server (e.g., "https://gitea.example.com").
	ServerURL string
	// Token is the Gitea API authentication token.
	Token string
}
