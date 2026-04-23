// Package client provides a factory for creating and validating Prometheus
// API clients. It wraps the prometheus/client_golang library and performs a
// connectivity check via the /api/v1/buildinfo endpoint at creation time.
package client

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// NewClientPrometheus creates a new Prometheus v1 API client for the given
// address and validates the connection by calling the Buildinfo endpoint with
// a 3-second timeout. Returns the API handle on success, or an error if the
// address is unreachable or the response is invalid.
func NewClientPrometheus(address string) (v1.API, error) {
	// Create the underlying HTTP client for the Prometheus API.
	client, err := api.NewClient(api.Config{
		Address: address,
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
	return v1api, nil
}
