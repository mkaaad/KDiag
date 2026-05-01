package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// AlertFingerprint computes a deterministic hash from alert labels.
// Labels are sorted by key before hashing, so the same alert with
// identical labels always produces the same fingerprint.
func AlertFingerprint(msg string) string {
	var raw struct {
		Labels  map[string]string `json:"labels"`
		Alerts  []struct {
			Labels map[string]string `json:"labels"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal([]byte(msg), &raw); err != nil {
		return ""
	}

	// Collect labels from both top-level and individual alerts.
	var allLabels []map[string]string
	if raw.Labels != nil {
		allLabels = append(allLabels, raw.Labels)
	}
	for _, a := range raw.Alerts {
		if a.Labels != nil {
			allLabels = append(allLabels, a.Labels)
		}
	}
	if len(allLabels) == 0 {
		return ""
	}

	// Normalize: sort keys, build canonical string.
	var parts []string
	for _, labels := range allLabels {
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
		}
	}
	canonical := strings.Join(parts, "&")
	h := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%x", h[:16]) // first 16 bytes = 32 hex chars
}

// AlertName extracts the alertname label from an Alertmanager JSON message.
func AlertName(msg string) string {
	var raw struct {
		Labels struct {
			AlertName string `json:"alertname"`
		} `json:"labels"`
		Alerts []struct {
			Labels struct {
				AlertName string `json:"alertname"`
			} `json:"labels"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal([]byte(msg), &raw); err != nil {
		return ""
	}
	if raw.Labels.AlertName != "" {
		return raw.Labels.AlertName
	}
	for _, a := range raw.Alerts {
		if a.Labels.AlertName != "" {
			return a.Labels.AlertName
		}
	}
	return ""
}
