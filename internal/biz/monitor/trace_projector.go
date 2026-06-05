package monitor

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
	repo  TraceRepo
	buses []contract.Bus
	lg    loggateway.Logger

	mu     sync.Mutex
	traces map[string]*activeTrace
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

func NewTraceProjector(repo TraceRepo, lg loggateway.Logger, buses ...contract.Bus) *TraceProjector {
	if repo == nil {
		return nil
	}
	seen := make([]contract.Bus, 0, len(buses))
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
		repo:   repo,
		buses:  seen,
		lg:     lg,
		traces: make(map[string]*activeTrace),
	}
}

const traceActiveTTL = 10 * time.Minute

func (p *TraceProjector) Start(ctx context.Context) {
	if p == nil {
		return
	}
	if err := p.repo.EnsureTraceSchema(ctx); err != nil {
		p.lg.Warn("EnsureTraceSchema failed", loggateway.StepID("monitor.trace_schema_fail"), loggateway.Err(err))
	}
	opts := contract.SubscribeOptions{
		EventTypes: []contract.EnvelopeType{contract.EnvelopeTypeFlowLog},
		BufferSize: 256,
		Reliable:   true,
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

func (p *TraceProjector) subscribeBus(ctx context.Context, name string, bus contract.Bus, opts contract.SubscribeOptions) {
	worker := newTraceProjectorWorker(name, p.lg)
	worker.Start(ctx, p.handle)
	ch, unsub := bus.Subscribe(opts)
	safego.Go(ctx, name, func() {
		defer unsub()
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-ch:
				if !ok {
					return
				}
				worker.Offer(ctx, env)
			}
		}
	})
}

func (p *TraceProjector) handle(ctx context.Context, env contract.Envelope) {
	if p == nil || env.Metadata == nil {
		return
	}
	m := env.Metadata
	traceID := metaStr(m, "trace_id")
	if traceID == "" {
		return
	}

	sessionID := coalesceStr(metaStr(m, "session_id"), env.SessionID)
	runID := metaStr(m, "run_id")
	agentID := metaStr(m, "agent_id")
	agentKey := metaStr(m, "agent_key")
	teamID := strings.TrimSpace(env.TeamID)
	stepID := metaStr(m, "step_id")
	flowPhase := metaStr(m, "flow_phase")
	domain := metaStr(m, "domain")

	if agentID == "" && agentKey != "" {
		agentID = agentKey
	}

	p.ensureTrace(ctx, traceID, sessionID, runID, agentID, teamID, domain)

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
		p.lg.Warn("UpsertMonitorTraceSpan failed",
			loggateway.StepID("monitor.trace_span_upsert_fail"), loggateway.Str("trace_id", traceID), loggateway.Str("span_id", spanID), loggateway.Err(err))
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

	if err := p.repo.UpdateMonitorTraceCompletion(ctx, traceID, status, durationMs, spanCount, errCount, tokens, costUsd); err != nil {
		p.lg.Warn("UpdateMonitorTraceCompletion failed",
			loggateway.StepID("monitor.trace_completion_fail"), loggateway.Str("trace_id", traceID), loggateway.Err(err))
	}
}

func (p *TraceProjector) ensureTrace(ctx context.Context, traceID, sessionID, runID, agentID, teamID, domain string) {
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
		TeamID:    teamID,
		Name:      domain,
		Status:    "running",
	}
	if err := p.repo.InsertMonitorTrace(ctx, tw); err != nil {
		p.lg.Warn("InsertMonitorTrace failed",
			loggateway.StepID("monitor.trace_insert_fail"), loggateway.Str("trace_id", traceID), loggateway.Err(err))
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
	queue     chan contract.Envelope
	dropCount atomic.Int64
	lg        loggateway.Logger
}

func newTraceProjectorWorker(name string, lg loggateway.Logger) *traceProjectorWorker {
	return &traceProjectorWorker{
		name:  name,
		queue: make(chan contract.Envelope, 256),
		lg:    lg,
	}
}

func (w *traceProjectorWorker) Start(ctx context.Context, fn func(context.Context, contract.Envelope)) {
	safego.Go(ctx, w.name, func() {
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-w.queue:
				if !ok {
					return
				}
				fn(ctx, env)
			}
		}
	})
}

func (w *traceProjectorWorker) Offer(ctx context.Context, env contract.Envelope) {
	select {
	case w.queue <- env:
	default:
		w.dropCount.Add(1)
		if w.dropCount.Load()%100 == 1 {
			w.lg.Warn("TraceProjector queue full, dropping envelope",
				loggateway.StepID("monitor.trace_projector_queue_full"), loggateway.Str("worker", w.name), loggateway.Str("total_drops", fmt.Sprint(w.dropCount.Load())))
		}
	}
}
