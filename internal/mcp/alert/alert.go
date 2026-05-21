// Package alert publishes sustained MCP health degradation events for Monitor.
package alert

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/mcp/metadata"
	"aranea-agents/internal/mcp/probe"
	"aranea-agents/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var healthAlertTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "aranea_mcp_health_alert_total",
	Help: "MCP sustained health alerts emitted by server_key.",
}, []string{"server_key"})

// DefaultSustainedErrorAfter is how long health_status=error must persist before alerting.
const DefaultSustainedErrorAfter = 5 * time.Minute

// Publisher emits monitor events when MCP servers stay unhealthy.
type Publisher struct {
	bus event.Bus
	uc  *biz.MCPServerUsecase
}

func NewPublisher(bus event.Bus, uc *biz.MCPServerUsecase) *Publisher {
	return &Publisher{bus: bus, uc: uc}
}

func SustainedErrorAfter() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MCP_HEALTH_ALERT_AFTER"))
	if raw == "" {
		return DefaultSustainedErrorAfter
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return DefaultSustainedErrorAfter
	}
	return d
}

// MaybeEmitAfterHealth persists sustained-error alerts after probe + PersistHealth.
func (p *Publisher) MaybeEmitAfterHealth(ctx context.Context, srv biz.MCPServer, result probe.TestResult) {
	if p == nil || p.bus == nil || result.OK {
		return
	}
	meta := metadata.Parse(srv.MetadataJSON)
	now := time.Now().UTC()
	if !metadata.ShouldEmitHealthAlert(meta, now, SustainedErrorAfter()) {
		return
	}
	metadata.MarkHealthAlert(meta, now)
	healthAlertTotal.WithLabelValues(srv.Key).Inc()
	metrics.AlertNotifyTotal.WithLabelValues("mcp_health", "ok").Inc()

	env := event.NewEnvelope(event.EnvelopeTypeMCPHealthAlert, "mcp", "")
	env.Channel = "monitor"
	env.Metadata = map[string]any{
		"server_key":          srv.Key,
		"server_id":           srv.ID,
		"health_status":       result.Status,
		"message":             result.Message,
		"health_error_since":  metadata.ErrorSince(meta),
		"sustained_after_sec": int(SustainedErrorAfter().Seconds()),
	}
	if env.Content == nil {
		env.Content = &event.EnvelopeContent{Text: result.Message}
	}
	p.bus.Publish(ctx, env)
	if p.uc != nil {
		_ = p.uc.MarkHealthAlertEmitted(ctx, srv.ID, now)
	}
}
