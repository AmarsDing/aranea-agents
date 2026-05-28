// Package mcpobserve wires MCP session reconnect telemetry into EventBus and Prometheus.
package mcpobserve

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/event"
	mcpdefaults "aranea-agents/internal/mcp"
	mcpconfig "aranea-agents/internal/mcp/config"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/safego"

	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

// MetadataRecorder persists reconnect timestamps into mcp_server.metadata_json.
type MetadataRecorder func(ctx context.Context, serverKey string, at time.Time)

var (
	mu               sync.RWMutex
	bus              event.Bus
	metadataRecorder MetadataRecorder
)

// SetBus configures the event bus used for MCP reconnect envelopes.
func SetBus(b event.Bus) {
	mu.Lock()
	bus = b
	mu.Unlock()
}

// SetMetadataRecorder configures optional persistence of last_reconnect_at / reconnect_count.
func SetMetadataRecorder(rec MetadataRecorder) {
	mu.Lock()
	metadataRecorder = rec
	mu.Unlock()
}

func currentBus() event.Bus {
	mu.RLock()
	defer mu.RUnlock()
	return bus
}

func currentMetadataRecorder() MetadataRecorder {
	mu.RLock()
	defer mu.RUnlock()
	return metadataRecorder
}

// ObserverForServer returns a ReconnectObserver for the given MCP server key.
func ObserverForServer(serverKey string) trpcmcp.ReconnectObserver {
	serverKey = strings.TrimSpace(serverKey)
	return func(ctx context.Context, ev trpcmcp.ReconnectEvent) {
		outcome := "failed"
		if ev.Success {
			outcome = "success"
		} else if ev.Attempt >= ev.MaxAttempts && ev.MaxAttempts > 0 {
			outcome = "exhausted"
		}
		name := strings.TrimSpace(ev.ServerName)
		if name == "" {
			name = serverKey
		}
		metrics.MCPSessionReconnectTotal.WithLabelValues(name, outcome).Inc()

		b := currentBus()
		if b != nil {
			env := event.NewEnvelope(event.EnvelopeTypeMCPSessionReconnect, "mcp", "")
			env.Channel = "monitor"
			env.Metadata = map[string]any{
				"server_key":   name,
				"attempt":      ev.Attempt,
				"max_attempts": ev.MaxAttempts,
				"success":      ev.Success,
				"outcome":      outcome,
			}
			if ev.Err != nil {
				env.Metadata["error"] = ev.Err.Error()
			}
			b.Publish(ctx, env)
		}

		if rec := currentMetadataRecorder(); rec != nil {
			at := time.Now().UTC()
			key := name
			safego.Go(ctx, "mcp.reconnect_metadata", func() {
				rec(context.WithoutCancel(ctx), key, at)
			})
		}
	}
}

// DefaultSessionReconnectMax returns the default max reconnect attempts for network transports.
// Uses the canonical mcpconfig.NormalizeTransport so all transport aliases are handled
// consistently (TPM-P1-10).
func DefaultSessionReconnectMax(transport string) int {
	normalized := mcpconfig.NormalizeTransport(transport)
	switch normalized {
	case string(mcpconfig.TransportSSE), string(mcpconfig.TransportStreamable):
		return mcpdefaults.DefaultSessionReconnectMax
	default:
		return 0
	}
}

// EffectiveSessionReconnectMax picks configured max or transport default.
func EffectiveSessionReconnectMax(transport string, configured int) int {
	if configured > 0 {
		return configured
	}
	return DefaultSessionReconnectMax(transport)
}

// RecentReconnectWindow is used by the frontend to show a "recent reconnect" chip.
const RecentReconnectWindow = mcpdefaults.RecentReconnectWindow

// IsRecentReconnect reports whether lastReconnectAt (RFC3339) is within RecentReconnectWindow.
func IsRecentReconnect(lastReconnectAt string) bool {
	lastReconnectAt = strings.TrimSpace(lastReconnectAt)
	if lastReconnectAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, lastReconnectAt)
	if err != nil {
		return false
	}
	return time.Since(t) < RecentReconnectWindow
}
