package server

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/event/contract"
)

// wsUpstream represents a client-to-server WebSocket message.
type wsUpstream struct {
	Direction string `json:"direction"`
	Channel   string `json:"channel"`
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	Payload   any    `json:"payload,omitempty"`
}

// wsDownstream represents a server-to-client WebSocket message.
// Chat business events use typed v2_event envelopes (separate from this
// struct). Monitor events ride MonitorEvent. History hydrate uses v2 REST
// (tasks/turns/steps) or ListActivities (steps_v2 adapter).
type wsDownstream struct {
	Direction    string                 `json:"direction"`
	Channel      string                 `json:"channel"`
	Type         string                 `json:"type,omitempty"`
	Payload      any                    `json:"payload,omitempty"`
	MonitorEvent *contract.MonitorEvent `json:"monitor_event,omitempty"`
}

// wsProbeMode returns true when the client requests a lightweight probe connection.
func wsProbeMode(r *http.Request) bool {
	q := r.URL.Query()
	v := strings.TrimSpace(q.Get("probe"))
	if v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	return strings.TrimSpace(q.Get("health")) == "1"
}

// envInt reads a positive integer from the environment variable key,
// falling back to the provided default value.
func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
