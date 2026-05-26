package health

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/mcp/alert"
	"aranea-agents/pkg/safego"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	probeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_mcp_health_probe_total",
		Help: "Number of MCP server health probes by server_key and status.",
	}, []string{"server_key", "status"})

	probeDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_mcp_health_probe_duration_seconds",
		Help:    "Duration of MCP server health probes.",
		Buckets: []float64{0.1, 0.5, 1, 5, 10, 30},
	}, []string{"server_key"})
)

type Deps struct {
	MCP    biz.MCPServerRepo
	UC     *biz.MCPServerUsecase
	Alerts *alert.Publisher
}

type Runner struct {
	deps Deps
	mu   sync.Mutex
}

func NewRunner(deps Deps) *Runner {
	return &Runner{deps: deps}
}

func DefaultInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MCP_HEALTH_INTERVAL"))
	if raw == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 5 * time.Minute
	}
	return d
}

func (r *Runner) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	r.probeAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.probeAll(ctx)
		}
	}
}

func (r *Runner) probeAll(ctx context.Context) {
	if !r.mu.TryLock() {
		return
	}
	defer r.mu.Unlock()

	servers, err := r.deps.MCP.ListMCPServers(ctx)
	if err != nil {
		event.SysLogError("system.mcp.health_list_fail", "MCP 健康检查列表失败", event.P("error", err))
		return
	}
	for _, srv := range servers {
		if ctx.Err() != nil {
			return
		}
		if !srv.Enabled || strings.TrimSpace(srv.DeletedAt) != "" {
			continue
		}
		safego.Go(ctx, "mcp.health.probe."+srv.Key, func() {
			r.probeOne(ctx, srv)
		})
	}
}

func (r *Runner) probeOne(ctx context.Context, srv biz.MCPServer) {
	start := time.Now()
	result, err := r.deps.UC.TestMCPServer(ctx, srv.ID)
	if err != nil {
		event.SysLogError("system.mcp.health_probe_fail", "MCP 健康探测失败", event.P("server_key", srv.Key), event.P("error", err))
		return
	}
	elapsed := time.Since(start)

	// TPM-P1-09: "auth_required" means the server is network-reachable but requires
	// OAuth / API-key credentials that the probe does not inject. Treat it as a distinct
	// status rather than "error" so the health dashboard shows the correct reason and
	// alert rules are not triggered for servers that are actually healthy at the network
	// layer. Only hard failures (OK=false, Status≠"auth_required") raise an alert.
	metricStatus := result.Status
	if metricStatus == "" {
		if result.OK {
			metricStatus = "ok"
		} else {
			metricStatus = "error"
		}
	}
	probeTotal.WithLabelValues(srv.Key, metricStatus).Inc()
	probeDuration.WithLabelValues(srv.Key).Observe(elapsed.Seconds())

	isHardFailure := !result.OK && result.Status != "auth_required"
	if isHardFailure {
		updated, err := r.deps.MCP.GetMCPServer(ctx, srv.ID)
		if err == nil {
			srv = updated
		}
		if r.deps.Alerts != nil {
			r.deps.Alerts.MaybeEmitAfterHealth(ctx, srv, result)
		}
	}
}
