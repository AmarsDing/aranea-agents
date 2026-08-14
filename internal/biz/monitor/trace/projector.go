package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

type TraceProjector struct {
	repo      Writer
	usageRepo UsageRepo
	buses     []contract.MonitorBus
	lg        loggateway.Logger

	mu     sync.Mutex
	traces map[string]*activeTrace

	// started records whether Start() has been invoked. Combined with
	// lastEventUnixNano it lets the self-check distinguish idle (never
	// started or never received events) from stalled (used to receive
	// events but hasn't recently).
	started           atomic.Bool
	lastEventUnixNano atomic.Int64

	// upsertWarnInterval throttles the per-span upsert-failure Warn. When
	// the repo is down (e.g. PG connection lost), every span event fails and
	// would otherwise emit one Warn per event, flooding the log pipeline.
	upsertWarnInterval time.Duration
	lastUpsertWarnNano atomic.Int64

	// insertWarnThrottle / completionWarnThrottle / usageAggWarnThrottle
	// throttle the per-failure Warn on the trace-insert, trace-completion
	// and usage-aggregation paths. When the repo is down, each new trace or
	// runner completion would otherwise emit one Warn per call.
	insertWarnThrottle     *loggateway.Throttle
	completionWarnThrottle *loggateway.Throttle
	usageAggWarnThrottle   *loggateway.Throttle
}

type activeTrace struct {
	traceID   string
	sessionID string
	runID     string
	agentID   string
	teamID    string
	name      string
	createdAt time.Time
	spanCount int
	errCount  int
	tokens    int64
	costUsd   float64
}

func NewTraceProjector(repo Writer, lg loggateway.Logger, usageRepo UsageRepo, buses ...contract.MonitorBus) *TraceProjector {
	if repo == nil {
		return nil
	}
	seen := make([]contract.MonitorBus, 0, len(buses))
	for _, bus := range buses {
		if bus == nil {
			continue
		}
		dup := false
		for _, existing := range seen {
			if existing == bus {
				dup = true
				break
			}
		}
		if !dup {
			seen = append(seen, bus)
		}
	}
	if len(seen) == 0 {
		return nil
	}
	return &TraceProjector{
		repo:      repo,
		usageRepo: usageRepo,
		buses:     seen,
		lg:        lg,
		traces:    make(map[string]*activeTrace),

		upsertWarnInterval: defaultUpsertWarnInterval,

		insertWarnThrottle:     loggateway.NewThrottle(defaultUpsertWarnInterval),
		completionWarnThrottle: loggateway.NewThrottle(defaultUpsertWarnInterval),
		usageAggWarnThrottle:   loggateway.NewThrottle(defaultUpsertWarnInterval),
	}
}

const traceActiveTTL = 10 * time.Minute

// defaultUpsertWarnInterval is the default throttle window for the per-span
// upsert-failure Warn (see TraceProjector.upsertWarnInterval).
const defaultUpsertWarnInterval = 10 * time.Second

func (p *TraceProjector) Start(ctx context.Context) {
	if p == nil {
		return
	}
	if err := p.repo.EnsureTraceSchema(ctx); err != nil {
		p.lg.Warn("EnsureTraceSchema failed", loggateway.StepID("monitor.trace_schema_fail"), loggateway.Err(err))
	}
	p.started.Store(true)
	opts := contract.MonitorSubscribeOptions{
		BufferSize: 256,
		GlobalMode: true,
		Filter: func(ev contract.MonitorEvent) bool {
			return ev.Type == contract.MonitorEventTypeFlowLog
		},
	}
	for i, bus := range p.buses {
		name := "monitor-trace-projector"
		if len(p.buses) > 1 {
			name = fmt.Sprintf("monitor-trace-projector-%d", i)
		}
		p.subscribeBus(ctx, name, bus, opts)
	}
	cleanupTicker := time.NewTicker(time.Minute)
	safego.Go(ctx, "monitor-trace-projector-cleanup", func() {
		defer cleanupTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-cleanupTicker.C:
				p.evictStaleTraces()
			}
		}
	})
}

func (p *TraceProjector) evictStaleTraces() {
	p.mu.Lock()
	now := time.Now()
	for id, at := range p.traces {
		if now.Sub(at.createdAt) > traceActiveTTL {
			delete(p.traces, id)
		}
	}
	p.mu.Unlock()
}

func (p *TraceProjector) subscribeBus(ctx context.Context, name string, bus contract.MonitorBus, opts contract.MonitorSubscribeOptions) {
	worker := newTraceProjectorWorker(name, p.lg)
	worker.Start(ctx, p.handle)
	ch, unsub := bus.Subscribe(opts)
	safego.Go(ctx, name, func() {
		defer unsub()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				worker.Offer(ctx, ev)
			}
		}
	})
}

func (p *TraceProjector) handle(ctx context.Context, ev contract.MonitorEvent) {
	if p == nil || ev.Metadata == nil {
		return
	}
	// Record last-event time on every invocation, even for events that
	// don't carry a trace_id. This lets the self-check verify the projector
	// is still receiving flow_log traffic from the bus.
	p.lastEventUnixNano.Store(time.Now().UnixNano())
	m := ev.Metadata
	traceID := metaStr(m, "trace_id")
	if traceID == "" {
		return
	}

	sessionID := coalesceStr(metaStr(m, "session_id"), ev.SessionID)
	runID := metaStr(m, "run_id")
	agentID := metaStr(m, "agent_id")
	agentKey := metaStr(m, "agent_key")
	provider := metaStr(m, "provider")
	model := metaStr(m, "model")
	teamID := metaStr(m, "team_id")
	stepID := metaStr(m, "step_id")
	flowPhase := metaStr(m, "flow_phase")
	domain := metaStr(m, "domain")

	if agentID == "" && agentKey != "" {
		agentID = agentKey
	}

	p.ensureTrace(ctx, traceID, sessionID, runID, agentID, provider, model, teamID, domain)

	kind := spanKindFromStep(stepID, domain)
	if kind == "" {
		return
	}

	spanID := stepID
	if spanID == "" {
		return
	}

	status := "ok"
	if flowPhase == "error" {
		status = "error"
		p.mu.Lock()
		if at, ok := p.traces[traceID]; ok {
			at.errCount++
		}
		p.mu.Unlock()
	}

	var durationMs int64
	if d, ok := m["duration_ms"]; ok {
		switch v := d.(type) {
		case int:
			durationMs = int64(v)
		case int64:
			durationMs = v
		case float64:
			durationMs = int64(v)
		}
	}

	nowMs := time.Now().UnixMilli()
	startedAt := nowMs - durationMs
	if durationMs == 0 && flowPhase == "start" {
		startedAt = nowMs
	}

	attrs, _ := json.Marshal(map[string]any{
		"step_id":    stepID,
		"flow_phase": flowPhase,
		"domain":     domain,
		"session_id": sessionID,
	})

	sw := TraceSpanWrite{
		TraceID:        traceID,
		SpanID:         spanID,
		Kind:           kind,
		Name:           stepID,
		StartedAt:      startedAt,
		EndedAt:        nowMs,
		Status:         status,
		AttributesJSON: string(attrs),
	}
	if flowPhase == "error" {
		errMsg := metaStr(m, "error_message")
		if errMsg == "" {
			errMsg = metaStr(m, "message")
		}
		errJSON, _ := json.Marshal(map[string]any{"message": errMsg})
		sw.ErrorJSON = string(errJSON)
	}

	if err := p.repo.UpsertMonitorTraceSpan(ctx, sw); err != nil {
		p.warnUpsertSpanThrottled(traceID, spanID, err)
	}

	p.mu.Lock()
	if at, ok := p.traces[traceID]; ok {
		at.spanCount++
		if flowPhase == "done" || flowPhase == "error" {
			if pt, ok := m["prompt_tokens"]; ok {
				switch v := pt.(type) {
				case int:
					at.tokens += int64(v)
				case int64:
					at.tokens += v
				case float64:
					at.tokens += int64(v)
				}
			}
			if ct, ok := m["completion_tokens"]; ok {
				switch v := ct.(type) {
				case int:
					at.tokens += int64(v)
				case int64:
					at.tokens += v
				case float64:
					at.tokens += int64(v)
				}
			}
		}
	}
	p.mu.Unlock()
}

// BackfillTraces ensures trace schema and evicts stale traces to refresh the projector state.
func (p *TraceProjector) BackfillTraces(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if err := p.repo.EnsureTraceSchema(ctx); err != nil {
		return err
	}
	p.evictStaleTraces()
	return nil
}

// TraceCount returns the number of currently active traces in the projector.
func (p *TraceProjector) TraceCount() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.traces)
}

// Started reports whether Start() has been invoked on this projector.
func (p *TraceProjector) Started() bool {
	if p == nil {
		return false
	}
	return p.started.Load()
}

// LastEventAt returns the wall-clock time at which the projector last
// received an envelope from the bus. The zero value indicates that no
// envelope has been received since process start.
func (p *TraceProjector) LastEventAt() time.Time {
	if p == nil {
		return time.Time{}
	}
	ns := p.lastEventUnixNano.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}

// HasEverProcessed reports whether the projector has received at least
// one envelope since it was started. It is the boolean counterpart of
// LastEventAt and lets callers cheaply distinguish "fresh / never seen
// traffic" from "stale / once saw traffic but now silent".
func (p *TraceProjector) HasEverProcessed() bool {
	if p == nil {
		return false
	}
	return p.lastEventUnixNano.Load() > 0
}

func (p *TraceProjector) OnRunnerCompletion(ctx context.Context, traceID, status string, durationMs int64) {
	if p == nil || traceID == "" {
		return
	}
	p.mu.Lock()
	at, ok := p.traces[traceID]
	spanCount := 0
	errCount := 0
	tokens := int64(0)
	costUsd := 0.0
	if ok {
		spanCount = at.spanCount
		errCount = at.errCount
		tokens = at.tokens
		costUsd = at.costUsd
		delete(p.traces, traceID)
	}
	p.mu.Unlock()

	if status == "error" && errCount == 0 {
		errCount = 1
	}

	c := TraceCompletion{
		Status:       status,
		DurationMs:   durationMs,
		SpanCount:    spanCount,
		ErrorCount:   errCount,
		TotalTokens:  tokens,
		TotalCostUsd: costUsd,
	}
	// Usage events are the authoritative source for cost; aggregate them at
	// completion so traces written before flow-log token capture still get
	// accurate tokens/cost/provider/model.
	if p.usageRepo != nil {
		if agg, err := p.usageRepo.AggregateUsageByTrace(ctx, traceID); err != nil {
			p.warnRepoOpThrottled(p.usageAggWarnThrottle, "AggregateUsageByTrace failed", err,
				loggateway.StepID("monitor.trace_usage_agg_fail"), loggateway.Str("trace_id", traceID))
		} else if agg.CallCount > 0 {
			if agg.TotalTokens > c.TotalTokens {
				c.TotalTokens = agg.TotalTokens
			}
			c.TotalCostUsd = agg.TotalCostUsd
			c.Provider = agg.Provider
			c.Model = agg.Model
		}
	}

	if err := p.repo.UpdateMonitorTraceCompletion(ctx, traceID, c); err != nil {
		p.warnRepoOpThrottled(p.completionWarnThrottle, "UpdateMonitorTraceCompletion failed", err,
			loggateway.StepID("monitor.trace_completion_fail"), loggateway.Str("trace_id", traceID))
	}
}

// warnRepoOpThrottled emits a repo-failure Warn at most once per throttle
// window. When the window resets, the Warn carries the number of failures
// suppressed since the previous emission so no signal is silently lost.
func (p *TraceProjector) warnRepoOpThrottled(th *loggateway.Throttle, msg string, err error, fields ...loggateway.Field) {
	ok, suppressed := th.Allow()
	if !ok {
		return
	}
	fields = append(fields, loggateway.Err(err))
	if suppressed > 0 {
		fields = append(fields, loggateway.Int("suppressed", suppressed))
	}
	p.lg.Warn(msg, fields...)
}

// warnUpsertSpanThrottled emits the per-span upsert-failure Warn at most
// once per upsertWarnInterval. Span events are the highest-frequency failure
// path in the projector; without throttling, a dead repo turns every event
// into a Warn and floods the log pipeline.
func (p *TraceProjector) warnUpsertSpanThrottled(traceID, spanID string, err error) {
	interval := p.upsertWarnInterval
	if interval <= 0 {
		interval = defaultUpsertWarnInterval
	}
	now := time.Now().UnixNano()
	for {
		last := p.lastUpsertWarnNano.Load()
		if last != 0 && now-last < int64(interval) {
			return
		}
		if p.lastUpsertWarnNano.CompareAndSwap(last, now) {
			p.lg.Warn("UpsertMonitorTraceSpan failed (throttled)",
				loggateway.StepID("monitor.trace_span_upsert_fail"),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("span_id", spanID),
				loggateway.Err(err))
			return
		}
	}
}

func (p *TraceProjector) ensureTrace(ctx context.Context, traceID, sessionID, runID, agentID, provider, model, teamID, domain string) {
	p.mu.Lock()
	if _, exists := p.traces[traceID]; exists {
		p.mu.Unlock()
		return
	}
	at := &activeTrace{
		traceID:   traceID,
		sessionID: sessionID,
		runID:     runID,
		agentID:   agentID,
		teamID:    teamID,
		name:      domain,
		createdAt: time.Now(),
	}
	p.traces[traceID] = at
	p.mu.Unlock()

	tw := TraceWrite{
		TraceID:   traceID,
		SessionID: sessionID,
		RunID:     runID,
		AgentID:   agentID,
		Provider:  provider,
		Model:     model,
		TeamID:    teamID,
		Name:      domain,
		Status:    "running",
	}
	if err := p.repo.InsertMonitorTrace(ctx, tw); err != nil {
		p.warnRepoOpThrottled(p.insertWarnThrottle, "InsertMonitorTrace failed", err,
			loggateway.StepID("monitor.trace_insert_fail"), loggateway.Str("trace_id", traceID))
	}
}

func spanKindFromStep(stepID, domain string) string {
	if stepID == "" {
		return ""
	}
	s := strings.ToLower(stepID)
	switch {
	case s == "chat.turn" || s == "turn":
		return "root"
	case strings.HasPrefix(s, "llm.") || strings.HasPrefix(s, "model.") || strings.HasPrefix(s, "completion"):
		return "llm"
	case strings.HasPrefix(s, "tool.") || strings.HasPrefix(s, "function.") || strings.HasSuffix(s, ".tool") || strings.HasSuffix(s, ".function"):
		return "tool"
	case strings.HasPrefix(s, "retrieve.") || strings.HasPrefix(s, "memory.") || strings.HasPrefix(s, "recall."):
		return "memory"
	case strings.HasPrefix(s, "graph.") || strings.HasPrefix(s, "node.") || strings.HasSuffix(s, ".node"):
		return "graph"
	case strings.HasPrefix(s, "hitl.") || strings.HasPrefix(s, "human.") || strings.HasPrefix(s, "confirm."):
		return "hitl"
	case strings.HasPrefix(s, "team.") || strings.HasPrefix(s, "subteam."):
		return "team"
	default:
		return "step"
	}
}

func metaStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func coalesceStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

type traceProjectorWorker struct {
	name      string
	queue     chan contract.MonitorEvent
	dropCount atomic.Int64
	lg        loggateway.Logger
}

func newTraceProjectorWorker(name string, lg loggateway.Logger) *traceProjectorWorker {
	return &traceProjectorWorker{
		name:  name,
		queue: make(chan contract.MonitorEvent, 256),
		lg:    lg,
	}
}

func (w *traceProjectorWorker) Start(ctx context.Context, fn func(context.Context, contract.MonitorEvent)) {
	safego.Go(ctx, w.name, func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.queue:
				if !ok {
					return
				}
				fn(ctx, ev)
			}
		}
	})
}

func (w *traceProjectorWorker) Offer(ctx context.Context, ev contract.MonitorEvent) {
	select {
	case w.queue <- ev:
	default:
		w.dropCount.Add(1)
		if w.dropCount.Load()%100 == 1 {
			w.lg.Warn("TraceProjector queue full, dropping event",
				loggateway.StepID("monitor.trace_projector_queue_full"), loggateway.Str("worker", w.name), loggateway.Str("total_drops", fmt.Sprint(w.dropCount.Load())))
		}
	}
}
