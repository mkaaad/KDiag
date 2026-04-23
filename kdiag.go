// Package kdiag provides a net/http handler for receiving Prometheus Alertmanager
// webhooks and diagnosing alerts via an LLM agent. It also supports polling
// Prometheus alerts on a configurable interval.
package kdiag

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/mkaaad/kdiag/config"
	"github.com/mkaaad/kdiag/internal/agent"
	"github.com/mkaaad/kdiag/internal/client"
	"github.com/mkaaad/kdiag/internal/tool"
)

// NewHanderFunc creates a net/http handler that processes Alertmanager webhooks.
// It creates a Prometheus API client, registers the metrics query tools with
// the provided config, and returns an HTTP handler. The handler reads the
// request body, responds immediately with 200 "ok", and then runs the LLM
// agent diagnosis asynchronously in a background goroutine.
// It panics if the Prometheus address is unreachable or the config is invalid.
func NewHanderFunc(ctx context.Context, c *config.Config) http.HandlerFunc {
	// Create a Prometheus API client and validate the connection.
	api, err := client.NewClientPrometheus(c.PrometheusAddress)
	if err != nil {
		panic(err)
	}
	// Register the Prometheus query tools so the LLM agent can query metrics.
	mt := tool.NewMetricsQueryTool(api)
	c.Tools = append(c.Tools, mt...)
	return func(w http.ResponseWriter, r *http.Request) {
		// Read the full Alertmanager webhook payload from the request body.
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			//TODO: add some log
			return
		}
		// Acknowledge the webhook immediately to prevent Alertmanager
		// from retrying, then process the alert asynchronously.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		agent.Diag(ctx, c, string(raw))
	}

}

// PollAlerts periodically fetches active alerts from Prometheus and triggers
// LLM-based diagnosis for each alert batch. It uses the configured polling
// interval (defaults to 24 hours if not set).
// This function blocks indefinitely on the ticker channel; call it in a
// goroutine if non-blocking behavior is desired.
func PollAlerts(ctx context.Context, c *config.Config) {
	// Create a Prometheus API client and validate the connection.
	api, err := client.NewClientPrometheus(c.PrometheusAddress)
	if err != nil {
		panic(err)
	}
	// Register the Prometheus query tools for the LLM agent.
	mt := tool.NewMetricsQueryTool(api)
	c.Tools = append(c.Tools, mt...)

	// Set up a ticker with the configured interval, defaulting to 24h.
	var ticker *time.Ticker
	defer ticker.Stop()
	if c.PollingInterval == 0 {
		ticker = time.NewTicker(24 * time.Hour)
	} else {
		ticker = time.NewTicker(c.PollingInterval)
	}
	// alert fetches all current alerts from Prometheus and runs diagnosis.
	alert := func() {
		alertMsg, err := api.Alerts(ctx)
		if err != nil {
			//TODO: log the error
		}
		data, err := json.Marshal(alertMsg)
		if err != nil {
			//TODO: log the error
		}
		agent.Diag(ctx, c, string(data))
	}
	// Run an immediate alert check, then poll on the ticker interval.
	go alert()
	for range ticker.C {
		go alert()
	}
}
