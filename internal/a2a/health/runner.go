package health

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	gatewayHealthy = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aranea_a2a_gateway_healthy",
		Help: "A2A remote gateway health (1=ok, 0=fail) by registry id.",
	}, []string{"registry_id", "workspace"})

	probeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_a2a_health_probe_total",
		Help: "A2A remote health probes by registry id and status.",
	}, []string{"registry_id", "status"})
)

type Deps struct {
	A2A *biz.A2AUsecase
}

type Runner struct {
	deps Deps
	mu   sync.Mutex
	lg   loggateway.Logger
}

func NewRunner(deps Deps, lg loggateway.Logger) *Runner {
	return &Runner{deps: deps, lg: lg}
}

func DefaultInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("A2A_HEALTH_INTERVAL"))
	if raw == "" {
		return 10 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 10 * time.Minute
	}
	return d
}

func (r *Runner) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval()
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
	if r == nil || r.deps.A2A == nil {
		return
	}
	if !r.mu.TryLock() {
		return
	}
	defer r.mu.Unlock()

	agents, err := r.deps.A2A.ListRemoteAgents(ctx, "")
	if err != nil {
		r.lg.Error("A2A 远程网关列表失败", loggateway.StepID("a2a.health_list_fail"), loggateway.Err(err))
		return
	}
	for _, ag := range agents {
		if ctx.Err() != nil {
			return
		}
		if !ag.Enabled {
			continue
		}
		item := ag
		safego.Go(ctx, "a2a.health.probe."+item.ID, func() {
			r.probeOne(ctx, item)
		})
	}
}

func (r *Runner) probeOne(ctx context.Context, ag biz.A2ARemoteAgent) {
	ok := false
	status := "error"
	defer func() {
		probeTotal.WithLabelValues(ag.ID, status).Inc()
		val := 0.0
		if ok {
			val = 1
		}
		gatewayHealthy.WithLabelValues(ag.ID, ag.Workspace).Set(val)
	}()

	_, err := r.deps.A2A.DiscoverRemoteAgent(ctx, biz.RemoteCardDiscoverInput{
		RemoteURL:      ag.RemoteURL,
		AuthType:       ag.AuthType,
		AuthConfigJSON: ag.AuthConfigJSON,
	})
	ok = err == nil
	if ok {
		status = "ok"
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	if persistErr := r.deps.A2A.PersistRemoteHealth(ctx, ag.ID, ok, errMsg); persistErr != nil {
		r.lg.Error("A2A 健康状态保存失败",
			loggateway.StepID("a2a.health_persist_fail"),
			loggateway.Str("registry_id", ag.ID),
			loggateway.Err(persistErr),
		)
	}
}
