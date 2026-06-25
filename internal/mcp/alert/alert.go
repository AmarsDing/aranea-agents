package alert

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
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
	monitorBus contract.MonitorBus
	uc         *biz.MCPServerUsecase
	lg         loggateway.Logger
}

func NewPublisher(monitorBus contract.MonitorBus, uc *biz.MCPServerUsecase, lg loggateway.Logger) *Publisher {
	return &Publisher{monitorBus: monitorBus, uc: uc, lg: lg}
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
	if p == nil || p.monitorBus == nil {
		return
	}
	// Only hard failures and auth_required warnings trigger alert logic.
	// OK=true with status other than auth_required means healthy — skip.
	if result.OK && result.Status != "auth_required" {
		return
	}
	meta := metadata.Parse(srv.MetadataJSON)
	now := time.Now().UTC()
	if !metadata.ShouldEmitHealthAlert(meta, now, SustainedErrorAfter()) {
		return
	}
	// WBPF: persist debounce marker BEFORE emitting event to prevent
	// duplicate alerts if the process crashes between publish and persist.
	if p.uc != nil {
		if err := p.uc.MarkHealthAlertEmitted(ctx, srv.ID, now); err != nil {
			p.lg.Warn("MCP 健康告警持久化失败",
				loggateway.StepID("mcp.health_alert_persist_fail"),
				loggateway.Str("server_key", srv.Key),
				loggateway.Err(err),
			)
			// Abort: if we can't persist the debounce marker, don't emit
			// or we risk duplicate alerts on restart.
			return
		}
	}
	meta = metadata.MarkHealthAlert(meta, now)
	healthAlertTotal.WithLabelValues(srv.Key).Inc()
	metrics.AlertNotifyTotal.WithLabelValues("mcp_health", "ok").Inc()

	ev := contract.NewMonitorEvent(contract.MonitorEventTypeMCPHealthAlert, "mcp")
	ev.Message = result.Message
	ev.Metadata = map[string]any{
		"server_key":          srv.Key,
		"server_id":           srv.ID,
		"health_status":       result.Status,
		"message":             result.Message,
		"health_error_since":  metadata.ErrorSince(meta),
		"sustained_after_sec": int(SustainedErrorAfter().Seconds()),
	}
	p.monitorBus.Publish(ctx, ev)
}
