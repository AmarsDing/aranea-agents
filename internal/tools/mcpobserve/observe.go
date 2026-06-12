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

// Observer holds the dependencies for MCP reconnect telemetry.
// Construct via NewObserver and use ObserverForServer method instead of
// the package-level SetBus/SetMetadataRecorder functions.
type Observer struct {
	bus              event.Bus
	metadataRecorder MetadataRecorder
}

// NewObserver creates an Observer with the given dependencies.
func NewObserver(bus event.Bus, rec MetadataRecorder) *Observer {
	return &Observer{bus: bus, metadataRecorder: rec}
}

// ObserverForServer returns a ReconnectObserver for the given MCP server key.
func (o *Observer) ObserverForServer(serverKey string) trpcmcp.ReconnectObserver {
	return o.buildObserver(serverKey)
}

func (o *Observer) buildObserver(serverKey string) trpcmcp.ReconnectObserver {
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

		if o.bus != nil {
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
			o.bus.Publish(ctx, env)
		}

		if o.metadataRecorder != nil {
			at := time.Now().UTC()
			key := name
			safego.Go(ctx, "mcp.reconnect_metadata", func() {
				metaCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				o.metadataRecorder(metaCtx, key, at)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Legacy package-level API (deprecated — use NewObserver + Observer.ObserverForServer instead)
// ---------------------------------------------------------------------------

var (
	mu               sync.RWMutex
	bus              event.Bus
	metadataRecorder MetadataRecorder
)

// SetBus configures the event bus used for MCP reconnect envelopes.
// Deprecated: Use NewObserver instead.
func SetBus(b event.Bus) {
	mu.Lock()
	bus = b
	mu.Unlock()
}

// SetMetadataRecorder configures optional persistence of last_reconnect_at / reconnect_count.
// Deprecated: Use NewObserver instead.
func SetMetadataRecorder(rec MetadataRecorder) {
	mu.Lock()
	metadataRecorder = rec
	mu.Unlock()
}

// ObserverForServer returns a ReconnectObserver for the given MCP server key.
// This is the legacy package-level function that uses global state.
// Deprecated: Use NewObserver().ObserverForServer() instead.
func ObserverForServer(serverKey string) trpcmcp.ReconnectObserver {
	mu.RLock()
	b := bus
	rec := metadataRecorder
	mu.RUnlock()
	o := &Observer{bus: b, metadataRecorder: rec}
	return o.buildObserver(serverKey)
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
	return time.Since(t) >= 0 && time.Since(t) < RecentReconnectWindow
}
