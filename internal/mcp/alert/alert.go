package alert

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/mcp"
	"aranea-agents/internal/mcp/metadata"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var healthAlertTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "aranea_mcp_health_alert_total",
	Help: "MCP sustained health alerts emitted by server_key.",
}, []string{"server_key"})

const DefaultSustainedErrorAfter = mcp.DefaultSustainedErrorAfter

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
		return mcp.DefaultSustainedErrorAfter
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return mcp.DefaultSustainedErrorAfter
	}
	return d
}

func (p *Publisher) MaybeEmitAfterHealth(ctx context.Context, srv biz.MCPServer, result biz.MCPTestResult) {
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
		if err := p.uc.MarkHealthAlertEmitted(ctx, srv.ID, now); err != nil {
			loggateway.Global().Warn("MCP 健康告警持久化失败",
				loggateway.StepID("system.mcp.health_alert_persist_fail"),
				loggateway.Str("server_key", srv.Key),
				loggateway.Err(err),
			)
		}
	}
}
