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

// EmitDecision 是所有非闸类决策观测的统一入口（SP-1b，2026-08-29 Q2 根修）：
// decision_records（经 collector outbox）与 flow_log_events（经 ctx
// TraceEmitter）的双写在同一调用内完成——此前 planner/HITL/orch 各站手工
// 拼两次调用，任一侧被后续编辑删掉都无编译错误，两表证据链静默断裂。
// collector 为 nil 时记 collector_nil 告警事件后仍写 flowlog；ctx 无
// TraceEmitter 时 flowlog 静默跳过（CLI/单测无 turn ctx）。flowStep 是
// flowlog 的 step_id；flowMsg 为空时回落 rec.Scenario。
func EmitDecision(ctx context.Context, c decision.Collector, rec decision.Record, flowStep string, flowMsg string, pairs ...Pair) {
	if c == nil {
		LogNilCollector(ctx, string(rec.Category))
	} else {
		c.Emit(ctx, rec)
	}
	em := TraceEmitterFromContext(ctx)
	if em == nil {
		return
	}
	if flowMsg == "" {
		flowMsg = rec.Scenario
	}
	em.LogDone(flowStep, flowMsg, pairs...)
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
