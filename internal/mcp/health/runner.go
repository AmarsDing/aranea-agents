package health

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/mcp"
	"aranea-agents/internal/mcp/lifecycle"
	mcpmetadata "aranea-agents/internal/mcp/metadata"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const maxConcurrentProbes = 8

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
	MCP    biz.MCPServerReader
	UC     *biz.MCPServerUsecase
	Alerts AlertEmitter
	// FlowBus emits user-visible flow logs (流程日志) for system.mcp.* health
	// events; nil disables emission (tests / minimal setups).
	FlowBus contract.MonitorBus
	// OnServerRecovered, when non-nil, fires on the DOWN→UP recovery edge
	// (prev lifecycle state error/auth_required/degraded → ok). Used by P0-3
	// to invalidate cached agents whose MCP toolset went stale while the
	// server was down. Invoked asynchronously; must be goroutine-safe.
	// Unknown→ok (first probe after boot) is NOT a recovery edge.
	OnServerRecovered func(ctx context.Context, srv biz.MCPServer)
}

// isRecoveryEdge reports the DOWN→UP transition: the server was previously in
// a known-bad state and is now healthy. StateUnknown→StateOK is the first
// probe after boot (agents built while the server was up never went stale),
// so it is deliberately excluded.
func isRecoveryEdge(prev, next lifecycle.State) bool {
	if next != lifecycle.StateOK {
		return false
	}
	switch prev {
	case lifecycle.StateError, lifecycle.StateAuthRequired, lifecycle.StateDegraded:
		return true
	default:
		return false
	}
}

// AlertEmitter is the contract used by the health runner to emit alerts after
// a probe completes. *alert.Publisher satisfies this interface in production;
// tests inject a fake implementation to verify alert behavior.
type AlertEmitter interface {
	MaybeEmitAfterHealth(ctx context.Context, srv biz.MCPServer, result biz.MCPTestResult)
}

type Runner struct {
	deps Deps
	mu   sync.Mutex
	lg   loggateway.Logger
}

func NewRunner(deps Deps, lg loggateway.Logger) *Runner {
	return &Runner{deps: deps, lg: lg}
}

// logFlowError emits a user-visible flow log (流程日志) for MCP health
// failures. Nil-safe: skipped when the monitor bus is not wired.
func (r *Runner) logFlowError(ctx context.Context, step, message string, err error, pairs ...event.Pair) {
	if r == nil || r.deps.FlowBus == nil {
		return
	}
	lg := r.lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSystem,
		LG:     lg,
		Infra:  event.NewInfraFromBus(r.deps.FlowBus),
	})
	flow.LogError(step, message, append(pairs, event.P("error", err.Error()))...)
}

// healthStatusOf extracts the normalized lifecycle health status from
// mcp_server.metadata_json ("" / unknown when absent).
func healthStatusOf(metadataJSON string) lifecycle.State {
	raw, _ := mcpmetadata.Parse(metadataJSON)[mcpmetadata.KeyHealthStatus].(string)
	return lifecycle.Normalize(raw)
}

// isProbeTimeout reports whether a probe failure message indicates a timeout.
func isProbeTimeout(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "timeout") || strings.Contains(m, "deadline exceeded")
}

func DefaultInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MCP_HEALTH_INTERVAL"))
	if raw == "" {
		return mcp.DefaultHealthInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return mcp.DefaultHealthInterval
	}
	return d
}

// discoveryInterval is the fallback cadence (P2) for refreshing tool metadata
// on servers whose probe_mode does not run a real handshake (connectivity /
// auth_aware). MCP_DISCOVERY_INTERVAL overrides; defaults to
// mcp.DefaultDiscoveryInterval.
func discoveryInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MCP_DISCOVERY_INTERVAL"))
	if raw == "" {
		return mcp.DefaultDiscoveryInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return mcp.DefaultDiscoveryInterval
	}
	return d
}

func (r *Runner) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = mcp.DefaultHealthInterval
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

	servers, err := r.deps.MCP.ListMCPServers(ctx, biz.MCPListQuery{})
	if err != nil {
		r.lg.Error("MCP 健康检查列表失败", loggateway.StepID("mcp.health_list_fail"), loggateway.Err(err))
		r.logFlowError(ctx, "system.mcp.health_list_fail", "MCP 健康检查列表失败", err)
		return
	}
	sem := make(chan struct{}, maxConcurrentProbes)
	var wg sync.WaitGroup
loop:
	for _, srv := range servers {
		if ctx.Err() != nil {
			break loop
		}
		// ListMCPServers already filters deleted rows; only skip disabled ones.
		if !srv.Enabled {
			continue
		}
		srv := srv
		// Use select so ctx cancellation can preempt a blocked semaphore send.
		// Without this, a full semaphore would block until an in-flight probe
		// completes (up to ProbeTimeout), delaying shutdown.
		select {
		case sem <- struct{}{}:
			wg.Add(1)
			safego.Go(ctx, "mcp.health.probe."+srv.Key, func() {
				defer func() { <-sem; wg.Done() }()
				r.probeOne(ctx, srv)
			})
		case <-ctx.Done():
			break loop
		}
	}
	wg.Wait()
}

func (r *Runner) probeOne(ctx context.Context, srv biz.MCPServer) {
	prevState := healthStatusOf(srv.MetadataJSON)
	start := time.Now()
	res, err := r.deps.UC.TestMCPServer(ctx, srv.ID)
	if err != nil {
		r.lg.Error("MCP 健康探测失败", loggateway.StepID("mcp.health_probe_fail"), loggateway.Str("server_key", srv.Key), loggateway.Err(err))
		// 探测失败本身不产生 err（体现在 result 中）；TestMCPServer 的
		// error 返回主要来自 persistHealth 持久化失败（其次为 Get/解密失败）。
		r.logFlowError(ctx, "system.mcp.health_persist_fail", "MCP 健康状态保存失败", err,
			event.P("server_id", srv.ID),
			event.P("server_key", srv.Key))
		return
	}
	result := res.Result
	elapsed := time.Since(start)

	// K5：检查结果状态翻转 Info；K2：连续失败 Warn；probe 超时 Warn。
	newState := healthStatusOf(res.Server.MetadataJSON)
	if prevState != newState {
		r.lg.Info("MCP 健康状态翻转",
			loggateway.StepID("mcp.health_status_flip"),
			loggateway.Str("server_id", srv.ID),
			loggateway.Str("server_key", srv.Key),
			loggateway.Str("old_status", string(prevState)),
			loggateway.Str("new_status", string(newState)))
		// P0-3：DOWN→UP 恢复边沿 → 让缓存中的 agent 重建，摘掉掉线期间
		// 装配的陈旧 MCP toolset。异步执行，不阻塞探测循环；翻转只发生
		// 一次（下一轮 prevState 已是 ok），无需额外防抖。
		if isRecoveryEdge(prevState, newState) && r.deps.OnServerRecovered != nil {
			cb := r.deps.OnServerRecovered
			recovered := res.Server
			safego.Go(ctx, "mcp.health.recovered."+srv.Key, func() {
				cb(ctx, recovered)
			})
		}
	} else if !result.OK && prevState == lifecycle.StateError {
		r.lg.Warn("MCP 健康检查连续失败",
			loggateway.StepID("mcp.health_consecutive_fail"),
			loggateway.Str("server_id", srv.ID),
			loggateway.Str("server_key", srv.Key),
			loggateway.Str("error_since", mcpmetadata.ErrorSince(mcpmetadata.Parse(res.Server.MetadataJSON))),
			loggateway.Str("probe_message", result.Message))
	}
	if !result.OK && isProbeTimeout(result.Message) {
		r.lg.Warn("MCP 健康探测超时",
			loggateway.StepID("mcp.health_probe_timeout"),
			loggateway.Str("server_id", srv.ID),
			loggateway.Str("server_key", srv.Key),
			loggateway.Int64("elapsed_ms", elapsed.Milliseconds()))
	}

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

	// TestMCPServer already persisted health metadata via persistHealth.
	// Use the returned server (with updated metadata) for alert debounce logic.
	isHardFailure := !result.OK
	isAuthWarning := result.OK && result.Status == "auth_required"
	if isHardFailure || isAuthWarning {
		updated := res.Server
		if r.deps.Alerts != nil {
			r.deps.Alerts.MaybeEmitAfterHealth(ctx, updated, result)
		}
	}

	// P2 低频兜底发现：对 probe_mode 非 full_handshake 的服务器，健康巡检只
	// 验证连通性、不写 tool 元数据。这里按 discoveryInterval（默认 30min）
	// 对 metadata 过期且本次可达的服务器追加一次真实握手发现。full_handshake
	// 服务器刚在 persistHealth 写过 tools_discovered_at，stale 判定天然跳过，
	// 不会重复握手。发现失败只记日志，绝不影响健康主流程。
	if result.OK && mcpmetadata.ToolsDiscoveryStale(mcpmetadata.Parse(res.Server.MetadataJSON), time.Now().UTC(), discoveryInterval()) {
		discRes, derr := r.deps.UC.DiscoverMCPServerTools(ctx, srv.ID)
		switch {
		case derr != nil:
			r.lg.Warn("MCP 工具发现持久化失败",
				loggateway.StepID("mcp.tools_discovery_persist_fail"),
				loggateway.Str("server_id", srv.ID),
				loggateway.Str("server_key", srv.Key),
				loggateway.Err(derr))
		case !discRes.OK:
			r.lg.Warn("MCP 工具发现失败",
				loggateway.StepID("mcp.tools_discovery_fail"),
				loggateway.Str("server_id", srv.ID),
				loggateway.Str("server_key", srv.Key),
				loggateway.Str("message", discRes.Message))
		default:
			r.lg.Info("MCP 工具发现完成",
				loggateway.StepID("mcp.tools_discovery_done"),
				loggateway.Str("server_id", srv.ID),
				loggateway.Str("server_key", srv.Key),
				loggateway.Int("tool_count", discRes.ToolCount))
		}
	}
}
