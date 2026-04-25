package config

import (
	"testing"
	"time"

	"golang.org/x/text/language"
)

func TestConfig_DefaultValues(t *testing.T) {
	c := &Config{}
	if c.LLM != nil {
		t.Error("LLM should be nil by default")
	}
	if c.OpenAIFuncCall {
		t.Error("OpenAIFuncCall should be false by default")
	}
	if c.Tools != nil {
		t.Error("Tools should be nil by default")
	}
	if c.Language.String() != "und" {
		t.Error("Language should be zero value by default")
	}
	if c.PrometheusAddress != "" {
		t.Error("PrometheusAddress should be empty by default")
	}
	if c.PollingInterval != 0 {
		t.Error("PollingInterval should be 0 by default")
	}
	if c.MaxIterations != 0 {
		t.Error("MaxIterations should be 0 by default")
	}
}

func TestConfig_SetValues(t *testing.T) {
	c := &Config{
		OpenAIFuncCall:    true,
		PrometheusAddress: "http://localhost:9090",
		PollingInterval:   30 * time.Second,
		MaxIterations:     5,
		Language:          language.English,
	}
	if !c.OpenAIFuncCall {
		t.Error("OpenAIFuncCall should be true")
	}
	if c.PrometheusAddress != "http://localhost:9090" {
		t.Errorf("PrometheusAddress = %q", c.PrometheusAddress)
	}
	if c.PollingInterval != 30*time.Second {
		t.Errorf("PollingInterval = %v", c.PollingInterval)
	}
	if c.MaxIterations != 5 {
		t.Errorf("MaxIterations = %d", c.MaxIterations)
	}
	if c.Language.String() != language.English.String() {
		t.Errorf("Language = %v", c.Language)
	}
}
