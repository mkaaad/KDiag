// Package client provides a factory for creating and validating Prometheus
// API clients. It wraps the prometheus/client_golang library and performs a
// connectivity check via the /api/v1/buildinfo endpoint at creation time.
package client

import (
	"context"
	"net/http"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/mkaaad/kdiag/config"
	"github.com/mkaaad/kdiag/internal/tool"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// NewPrometheusClient creates a new Prometheus v1 API client for the given
// address and validates the connection by calling the Buildinfo endpoint with
// a 3-second timeout. Returns the API handle on success, or an error if the
// address is unreachable or the response is invalid.
func NewPrometheusClient(c *config.Config) (v1.API, error) {
	// Create the underlying HTTP client for the Prometheus API.
	client, err := api.NewClient(api.Config{
		Address: c.PrometheusAddress,
	})
	if err != nil {
		return nil, err
	}
	// Build the v1 API wrapper and validate connectivity.
	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = v1api.Buildinfo(ctx)

	if err != nil {
		return nil, err
	}
	tools := tool.NewMetricsQueryTool(v1api)
	c.Tools = append(c.Tools, tools...)
	return v1api, err
}
// NewGiteaClient creates a new Gitea API client for the given server URL and
// token, and validates the connection by fetching the authenticated user's info.
// On success the Gitea query tools are registered with the config. Returns an
// error if the server is unreachable or credentials are invalid.
func NewGiteaClient(c *config.Config) error {
	client, err := gitea.NewClient(c.GiteaConfig.ServerURL, gitea.SetToken(c.GiteaConfig.Token))
	if err != nil {
		return err
	}
	_, _, err = client.GetMyUserInfo()
	if err != nil {
		return err
	}
	tools := tool.NewGiteaQueryTool(client)
	c.Tools = append(c.Tools, tools...)
	return nil
}

// NewJaegerClient creates a Jaeger query API client for the given address and
// registers the Jaeger query tools with the config. If JaegerAddress is empty,
// it skips registration and returns nil. Returns an error if the address is
// unreachable.
func NewJaegerClient(c *config.Config) error {
	if c.JaegerAddress == "" {
		return nil
	}
	httpClient := &http.Client{Timeout: 5 * time.Second}
	jaegerClient := tool.NewJaegerClient(httpClient, c.JaegerAddress)
	// Validate connectivity by fetching services list.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := jaegerClient.DoGet(ctx, "/api/services", nil)
	if err != nil {
		return err
	}
	tools := tool.NewJaegerQueryTool(jaegerClient)
	c.Tools = append(c.Tools, tools...)
	return nil
}

// NewLokiClient creates a Loki query API client for the given address and
// registers the Loki log query tools with the config. If LokiAddress is empty,
// it skips registration and returns nil. Returns an error if the address is
// unreachable.
func NewLokiClient(c *config.Config) error {
	if c.LokiAddress == "" {
		return nil
	}
	httpClient := &http.Client{Timeout: 5 * time.Second}
	lokiClient := tool.NewLokiClient(httpClient, c.LokiAddress)
	// Validate connectivity by fetching label names.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := lokiClient.DoGet(ctx, "/loki/api/v1/label", nil)
	if err != nil {
		return err
	}
	tools := tool.NewLokiQueryTool(lokiClient)
	c.Tools = append(c.Tools, tools...)
	return nil
}
