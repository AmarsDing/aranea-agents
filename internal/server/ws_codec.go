package server

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
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
//
// Phase 5 Blocker A: the legacy Envelope field used for in-memory replay
// has been removed. Chat/system events are now carried by ActivityEvent;
// monitor events by MonitorEvent. Clients that previously relied on the
// "replay" type and the envelope field should call ListActivities RPC
// (GET /v1/sessions/{session_id}/activities) to backfill history on
// reconnect.
type wsDownstream struct {
	Direction     string                 `json:"direction"`
	Channel       string                 `json:"channel"`
	Type          string                 `json:"type,omitempty"`
	Payload       any                    `json:"payload,omitempty"`
	ActivityEvent *biz.ActivityEvent     `json:"activity_event,omitempty"`
	MonitorEvent  *contract.MonitorEvent `json:"monitor_event,omitempty"`
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
