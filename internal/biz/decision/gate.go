package decision

import (
	"context"

	"github.com/google/uuid"
)

// 系统闸 trigger_rule 枚举（C6，2026-08-26 一次列全）。param_rule_deny
// 属 79-runtime-governance R9 tool_param_rules，随其 Wave 上线后接通挂点；
// 其余四类已在生产路径产出。
const (
	TriggerTokenBudgetTripped = "token_budget_tripped"
	TriggerNoProgressTripped  = "no_progress_tripped"
	TriggerLoopGuardBlocked   = "loop_guard_blocked"
	TriggerParamRuleDeny      = "param_rule_deny"
	TriggerTeamCountMismatch  = "team_count_mismatch"
)

// GateDecision 是 S2 统一闸决策事件结构（设计 §3.2 row 3）：各闸点只填
// 语义字段，EmitGate 负责映射为 system_guard Record。Outcome 约定取值
// tripped/blocked/truncated（proceeded 仅 team_count_mismatch 放行分支）。
type GateDecision struct {
	TriggerRule string // 必填，取 TriggerXxx 常量
	Outcome     string // 必填：tripped / blocked / truncated / proceeded
	Scenario    string // 短标题（如 "run 累计 input token 超预算"）
	Reasoning   string // 规则描述（如 "run 累计 input 超 150 万"）
	GuardName   string // actor 后缀：system:{GuardName}
	// RunID 与 FlowTraceID 二选一：team run 闸填 RunID（父链查询期按 run
	// 内最近 planner 决策补全，见设计 §3.2 父链建立规则）；planner 侧闸
	// 填 FlowTraceID。
	RunID       string
	FlowTraceID string
	Entities    []EntityRef
	// ObservedValue / Threshold / Action 落入 metadata（observed_value /
	// threshold / action）；零值省略。
	ObservedValue any
	Threshold     any
	Action        string
	// Extra 并入 metadata（不与保留键冲突时）。
	Extra map[string]any
}

// EmitGate 把一条闸事件双写到决策记录层。collector 为 nil 时静默跳过；
// TriggerRule/Outcome/GuardName 为空时不产出（防半成品记录污染审计）。
func EmitGate(ctx context.Context, c Collector, gd GateDecision) {
	if c == nil || gd.TriggerRule == "" || gd.Outcome == "" || gd.GuardName == "" {
		return
	}
	metadata := map[string]any{"trigger_rule": gd.TriggerRule}
	if gd.ObservedValue != nil {
		metadata["observed_value"] = gd.ObservedValue
	}
	if gd.Threshold != nil {
		metadata["threshold"] = gd.Threshold
	}
	if gd.Action != "" {
		metadata["action"] = gd.Action
	}
	for k, v := range gd.Extra {
		if _, reserved := metadata[k]; !reserved {
			metadata[k] = v
		}
	}
	c.Emit(ctx, Record{
		DecisionKey:     uuid.NewString(),
		Category:        CategorySystemGuard,
		Scenario:        gd.Scenario,
		Reasoning:       gd.Reasoning,
		Outcome:         gd.Outcome,
		ActorType:       ActorSystem,
		ActorKey:        "system:" + gd.GuardName,
		RelatedEntities: gd.Entities,
		SourceRef:       SourceRef{RunID: gd.RunID, FlowTraceID: gd.FlowTraceID},
		Metadata:        metadata,
	})
}
