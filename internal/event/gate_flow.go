package event

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz/decision"
)

// EmitGate writes a system_guard decision record and a matching flow_log
// event (three-way evidence: decision_records + flow_log + gate-stats).
// Collector nil still emits the flow event when a TraceEmitter is on ctx,
// and logs an error so wiring gaps cannot stay silent (D1 / 0827 skip).
func EmitGate(ctx context.Context, c decision.Collector, gd decision.GateDecision) {
	if c == nil {
		LogNilCollector(ctx, gd.TriggerRule)
	}
	decision.EmitGate(ctx, c, gd)
	LogGateFlow(ctx, gd.TriggerRule, gd.Outcome, gd.Scenario, gd.Reasoning)
}

// LogNilCollector records that a gate/HITL path ran without a DecisionCollector.
func LogNilCollector(ctx context.Context, trigger string) {
	em := TraceEmitterFromContext(ctx)
	if em == nil {
		return
	}
	if trigger == "" {
		trigger = "unknown"
	}
	em.LogError("system.gate.collector_nil", "decision collector is nil, gate record skipped",
		P("trigger", trigger))
}

// LogGateFlow records a gate/HITL decision on the turn flow log. Missing
// TraceEmitter is a no-op (CLI / unit tests without a turn ctx).
func LogGateFlow(ctx context.Context, trigger, outcome, scenario, reasoning string) {
	em := TraceEmitterFromContext(ctx)
	if em == nil {
		return
	}
	step := "system.gate." + trigger
	if trigger == "" {
		step = "system.gate.unknown"
	}
	msg := scenario
	if msg == "" {
		msg = trigger
	}
	pairs := []Pair{P("trigger", trigger), P("outcome", outcome)}
	if reasoning != "" {
		pairs = append(pairs, P("reasoning", reasoning))
	}
	em.LogDone(step, msg, pairs...)
}

// LogHITLFlow records a hitl_approval decision on the turn flow log.
func LogHITLFlow(ctx context.Context, outcome, scenario, toolKey string) {
	em := TraceEmitterFromContext(ctx)
	if em == nil {
		return
	}
	msg := scenario
	if msg == "" {
		msg = fmt.Sprintf("hitl_approval %s", outcome)
	}
	pairs := []Pair{P("trigger", "hitl_approval"), P("outcome", outcome)}
	if toolKey != "" {
		pairs = append(pairs, P("tool_key", toolKey))
	}
	em.LogDone("system.gate.hitl_approval", msg, pairs...)
}
