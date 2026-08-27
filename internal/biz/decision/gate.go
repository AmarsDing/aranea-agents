package decision

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// gateRunIDCtxKey 是 WithGateRunID 的 ctx 键（未导出，强制经函数读写）。
type gateRunIDCtxKey struct{}

// WithGateRunID 把 team run id 注入执行 ctx（2026-08-27 二轮审查 H5 根修）。
// 背景：框架 invocation.Clone 生成全新 uuid 且清空 RunOptions.InvocationID
// （vendored FW 例外补丁，id 唯一性加固），图谱成员子 invocation 不再继承根
// invocation 的 run.ID——成员侧闸钩子（param_rule_gate / loop_guard /
// tool_result_prune / context_compression）无法从 invocation 回溯 run 归属，
// RunGateStats 按 team run id 过滤恒不命中。team runner 在图执行起点注入本
// 值，ctx 值随图执行派生链传到全部成员回调。chat/非团队路径不注入，钩子回
// 落 invocation id（不 join 任何 team run，stats 聚合自然忽略）。与
// sandbox.WithRunID 同型先例（runner_team_trpc.go）。
func WithGateRunID(ctx context.Context, runID string) context.Context {
	if ctx == nil || strings.TrimSpace(runID) == "" {
		return ctx
	}
	return context.WithValue(ctx, gateRunIDCtxKey{}, strings.TrimSpace(runID))
}

// GateRunIDFromContext 取 WithGateRunID 注入的 team run id；未注入返回 ""。
func GateRunIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(gateRunIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// gateSessionIDCtxKey 是 WithGateSessionID 的 ctx 键（未导出，强制经函数读写）。
type gateSessionIDCtxKey struct{}

// WithGateSessionID 把 chat/team 会话 id 注入执行 ctx（T5，2026-08-27）。
// 与 WithGateRunID 同型同理：框架 Clone 后成员子 invocation 的 Session 指针
// 不一定反映 chat 会话归属，team runner 在注入 run id 的同一坐标注入会话 id，
// 成员侧闸钩子（param_rule_gate / loop_guard / tool_result_prune /
// context_compression）经 GateSessionIDFromContext 取回，SessionGateStats
// 聚合才可命中。chat 路径不注入——钩子回落 invocation.Session.ID。
func WithGateSessionID(ctx context.Context, sessionID string) context.Context {
	if ctx == nil || strings.TrimSpace(sessionID) == "" {
		return ctx
	}
	return context.WithValue(ctx, gateSessionIDCtxKey{}, strings.TrimSpace(sessionID))
}

// GateSessionIDFromContext 取 WithGateSessionID 注入的会话 id；未注入返回 ""。
func GateSessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(gateSessionIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// 系统闸 trigger_rule 枚举（C6，2026-08-26 一次列全）。param_rule_deny
// 属 79-runtime-governance R9 tool_param_rules，随其 Wave 上线后接通挂点；
// 其余六类已在生产路径产出。
const (
	TriggerTokenBudgetTripped = "token_budget_tripped"
	TriggerNoProgressTripped  = "no_progress_tripped"
	TriggerLoopGuardBlocked   = "loop_guard_blocked"
	TriggerParamRuleDeny      = "param_rule_deny"
	TriggerTeamCountMismatch  = "team_count_mismatch"
	// 79-runtime-governance R7（G-1）：压缩/剪枝事件结构化持久化——此前仅
	// 运行时日志与 invocation state（进程即逝），run 结束后无法回溯。两类
	// 事件 outcome 均为 "truncated"，prune 负载在 metadata.prune_bytes。
	TriggerToolResultPruned = "tool_result_pruned"
	TriggerContextCompacted = "context_compacted"
	// TriggerInputRiskFlagged 是输入级确定性安全扫描命中事件（2026-08-28
	// 方案② S3）：chat/team turn 入口 ScanInputRisk 命中即发，outcome 恒
	// tripped（观测/审计语义，不阻断——硬拦截保持在 L3 ParamRuleGate）。
	TriggerInputRiskFlagged = "input_risk_flagged"
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
	// SessionID 是 chat/team 会话归属（T5）：写入 SourceRef.SessionID，
	// 并同步 metadata.session_id（与 Extra 注入时期的旧记录口径一致，
	// 读侧 COALESCE 两路兼容）。
	SessionID string
	Entities  []EntityRef
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
	// session_id 双写：SourceRef 一等公民（索引服务聚合/过滤）+ metadata
	// 旧口径（Extra 注入时期的存量记录只在 metadata，读侧 COALESCE 两路）。
	if sid := strings.TrimSpace(gd.SessionID); sid != "" {
		metadata["session_id"] = sid
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
		SourceRef:       SourceRef{RunID: gd.RunID, FlowTraceID: gd.FlowTraceID, SessionID: strings.TrimSpace(gd.SessionID)},
		Metadata:        metadata,
	})
}
