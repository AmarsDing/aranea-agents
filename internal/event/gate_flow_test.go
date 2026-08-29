package event

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/decision"
)

func TestEmitGate_NilCollectorLogsErrorAndStillFlows(t *testing.T) {
	monBus := &captureMonitorBus{}
	em := NewTraceEmitter(&Infra{MonitorEventBus: monBus}, TraceContext{
		TraceID:   "tr_gate",
		SessionID: "sess_gate",
		RunID:     "run_gate",
		Domain:    TraceDomainChat,
	}, nil)
	ctx := WithTraceEmitter(context.Background(), em)
	EmitGate(ctx, nil, decision.GateDecision{
		TriggerRule: decision.TriggerInputRiskFlagged,
		Outcome:     "tripped",
		Scenario:    "用户输入命中确定性风险扫描",
		Reasoning:   "flags=destructive",
		GuardName:   "input_safety_scan",
		SessionID:   "sess_gate",
	})
	time.Sleep(20 * time.Millisecond)

	monBus.mu.Lock()
	defer monBus.mu.Unlock()
	var sawNil, sawFlow bool
	for _, ev := range monBus.evs {
		step, _ := ev.Metadata["step_id"].(string)
		if step == "" {
			if md, ok := ev.Metadata["step"].(string); ok {
				step = md
			}
		}
		msg := ev.Message
		if strings.Contains(step, "collector_nil") || strings.Contains(msg, "collector is nil") {
			sawNil = true
		}
		if strings.Contains(step, "input_risk_flagged") || strings.Contains(msg, "风险扫描") {
			sawFlow = true
		}
	}
	if !sawNil {
		t.Fatalf("nil collector must log error, events=%d", len(monBus.evs))
	}
	if !sawFlow {
		t.Fatalf("nil collector must still write gate flow, events=%d", len(monBus.evs))
	}
}

func TestEmitGate_WithCollectorDoesNotLogNil(t *testing.T) {
	monBus := &captureMonitorBus{}
	em := NewTraceEmitter(&Infra{MonitorEventBus: monBus}, TraceContext{
		TraceID:   "tr_ok",
		SessionID: "sess_ok",
		RunID:     "run_ok",
		Domain:    TraceDomainChat,
	}, nil)
	ctx := WithTraceEmitter(context.Background(), em)
	cc := &captureDecisionCollector{}
	EmitGate(ctx, cc, decision.GateDecision{
		TriggerRule: decision.TriggerLoopGuardBlocked,
		Outcome:     "blocked",
		Scenario:    "同参拦截",
		GuardName:   "tool_loop_guard",
	})
	time.Sleep(20 * time.Millisecond)
	if len(cc.recs) != 1 {
		t.Fatalf("collector recs=%d want 1", len(cc.recs))
	}
	monBus.mu.Lock()
	defer monBus.mu.Unlock()
	for _, ev := range monBus.evs {
		step, _ := ev.Metadata["step_id"].(string)
		if strings.Contains(step, "collector_nil") {
			t.Fatal("wired collector must not log collector_nil")
		}
	}
}

type captureDecisionCollector struct {
	recs []decision.Record
}

func (c *captureDecisionCollector) Emit(_ context.Context, r decision.Record) {
	c.recs = append(c.recs, r)
}
