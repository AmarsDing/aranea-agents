package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"

	"aranea-agents/pkg/safego"
)

type TraceProjector struct {
	repo  Repo
	buses []contract.Bus

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

func NewTraceProjector(repo Repo, buses ...contract.Bus) *TraceProjector {
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
		traces: make(map[string]*activeTrace),
	}
}

const traceActiveTTL = 10 * time.Minute

func (p *TraceProjector) Start(ctx context.Context) {
	if p == nil {
		return
	}
	if err := p.repo.EnsureTraceSchema(ctx); err != nil {
		event.SysLogWarn("system.monitor.trace_schema_fail", "EnsureTraceSchema failed", event.P("error", err.Error()))
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
	worker := newTraceProjectorWorker(name)
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
		event.SysLogWarn("system.monitor.trace_span_upsert_fail", "UpsertMonitorTraceSpan failed",
			event.P("trace_id", traceID), event.P("span_id", spanID), event.P("error", err.Error()))
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
		event.SysLogWarn("system.monitor.trace_completion_fail", "UpdateMonitorTraceCompletion failed",
			event.P("trace_id", traceID), event.P("error", err.Error()))
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
		event.SysLogWarn("system.monitor.trace_insert_fail", "InsertMonitorTrace failed",
			event.P("trace_id", traceID), event.P("error", err.Error()))
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
	case hasPrefix(s, "llm.") || hasPrefix(s, "model.") || hasPrefix(s, "completion"):
		return "llm"
	case hasPrefix(s, "tool.") || hasPrefix(s, "function.") || hasSuffix(s, ".tool") || hasSuffix(s, ".function"):
		return "tool"
	case hasPrefix(s, "retrieve.") || hasPrefix(s, "memory.") || hasPrefix(s, "recall."):
		return "retrieve"
	case hasPrefix(s, "graph.") || hasPrefix(s, "node.") || hasSuffix(s, ".node"):
		return "graph_node"
	case hasPrefix(s, "hitl.") || hasPrefix(s, "human.") || hasPrefix(s, "confirm."):
		return "hitl"
	case hasPrefix(s, "team.") || hasPrefix(s, "subteam."):
		return "subteam"
	default:
		return "step"
	}
}

func hasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

func hasSuffix(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
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
	name     string
	queue    chan contract.Envelope
	dropCount int64
}

func newTraceProjectorWorker(name string) *traceProjectorWorker {
	return &traceProjectorWorker{
		name:  name,
		queue: make(chan contract.Envelope, 256),
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
		w.dropCount++
		if w.dropCount%100 == 1 {
			event.SysLogWarn("system.monitor.trace_projector_queue_full", "TraceProjector queue full, dropping envelope",
				event.P("worker", w.name), event.P("total_drops", fmt.Sprint(w.dropCount)))
		}
	}
}
