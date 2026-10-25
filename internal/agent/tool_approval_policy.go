package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// P0-5 审批工程化（docs/reports/2026-10-21-review-harness-p0-gap-solutions.md §6，
// 批 A12）：会话/Agent 级确定性审批姿态 BeforeTool 闸。
// priority 3（与 paramRuleGate 同档，链为稳定排序——批 B 接线时必须把本 hook
// 追加在 paramRuleGate 之后），求值点早于循环守卫（4）与确认门禁（10）：
//   - never   → 确定性 Reject（镜像 param_rule_deny 五件事：Info 日志 +
//     tool_invocations blocked 写 + EmitGate approval_policy_denied + Reject
//     文案只给规则 ID 不给 pattern + 不写 ctx verdict）。先于循环守卫→不消耗
//     inflight 槽位。事件定级 AS-EVT-01=Informational（SystemNoticeEvent +
//     审计双写，避开 BlockUpTo 行列放大等待副作用）。
//   - ask     → 裁定写入 ctx，确认门禁 decide() 强制确认（grant 可满足）。
//   - auto    → 裁定写入 ctx（mode=auto），消费语义等同 ask（同 grants 可满足、
//     同 reason=policy_ask，零新 trigger/reason）；verdict 投影携带
//     source_mode=auto 供 auto_review_verdict 审计区分（D5/phase 2）。
//   - trusted → 裁定写入 ctx，作为新 grant 档在 decide() danger floor 之后
//     消费——danger floor 恒优先于 trusted（computer-use danger /
//     injectionSuspected 不受 trusted 影响）。
//
// shadow_only=1 的规则参与竞选但零强制：命中必落 shadow 审计（EmitGate
// outcome=tripped + Extra.shadow_only=true）+ SystemNoticeEvent。
//
// 规则读取失败 fail-closed 折叠为 ask verdict（与 paramRuleGate 同一先例：
// 无法区分有/无 never 规则时向人工兜底，绝不 fail-open 放行）。
// store 未装配（lookup 为 nil）时构造返回 nil 不注册——OFF 状态运行时零开销，
// 行为与现状字节级一致。

// approvalPolicyGatePriority 与 paramRuleGate 同档（3）；稳定排序保序依赖
// 装配链追加次序——批 C（2026-10-22）已在 callback_chain.go 注册点落位
// （paramRuleGate 之后、args 守卫之前）。
const approvalPolicyGatePriority = 3

// approvalPolicyVerdictCtxKey 是审批策略 gate → 确认门禁的 ctx 传递键。
type approvalPolicyVerdictCtxKey struct{}

// approvalPolicyVerdict 是审批策略命中裁定（ask/auto/trusted 入 ctx；never 在
// gate 内直接拒绝、不下传——与 paramRuleVerdict 同一设计）。
type approvalPolicyVerdict struct {
	mode   biztool.ApprovalMode
	ruleID string
}

// approvalPolicyVerdictFromCtx 读取 gate 裁定；未命中/未装配返回 nil。
func approvalPolicyVerdictFromCtx(ctx context.Context) *approvalPolicyVerdict {
	v, _ := ctx.Value(approvalPolicyVerdictCtxKey{}).(*approvalPolicyVerdict)
	return v
}

// triggerApprovalPolicyDenied 是 never 命中的闸触发词。
// AS-EVT-01 定级：Informational——SystemNoticeEvent + flowlog/审计双写，不上
// Critical/Important 的 BlockUpTo 行列（§9.3）。
// 批 C（2026-10-22）归位：常量登记进 internal/biz/decision/gate.go 并同步
// flow_log_steps.go 标签；本地别名存续仅保批 A 测试零改动，取值同源。
const triggerApprovalPolicyDenied = decision.TriggerApprovalPolicyDenied

// triggerApprovalPolicyShadow 是 shadow_only 预演命中（tripped，零强制）。
const triggerApprovalPolicyShadow = decision.TriggerApprovalPolicyShadow

// 本批不动 internal/event/contract 常量表（允许清单外）。tool_invocations 的
// error_code 为自由字符串列，取值冻结；contract 常量表吸收单列排期（批 C
// 评审裁定：error_code 无跨表消费方，吸收收益不抵改动面）。
const errorCodeApprovalPolicyDenied = "approval_policy_denied"

// ApprovalPolicyLookup 是审批策略闸的运行时读口签名：(scope, scopeKey) 双作用
// 域合并由调用方在闭包内完成（批 C 接到 ToolUsecase 读口；测试经
// biztool.InMemApprovalPolicyStore 驱动）。
type ApprovalPolicyLookup func(ctx context.Context) ([]biztool.ApprovalPolicy, error)

// listMergedApprovalPolicies 合并 session 与 agent 两个作用域的启用规则
// （错误以第一失败返回，fail-closed 职责在 gate 调用方）。批 C 接线与
// CompositeApprovalPolicyLookup 共用的单源合并实现。
func listMergedApprovalPolicies(ctx context.Context, reader biztool.ApprovalPolicyReader, sessionID, agentID string, nowUnix int64) ([]biztool.ApprovalPolicy, error) {
	var all []biztool.ApprovalPolicy
	if sessionID != "" {
		rows, err := reader.ListEnabledApprovalPolicies(ctx, string(biztool.ApprovalScopeSession), sessionID, nowUnix)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	if agentID != "" {
		rows, err := reader.ListEnabledApprovalPolicies(ctx, string(biztool.ApprovalScopeAgent), agentID, nowUnix)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	return all, nil
}

// CompositeApprovalPolicyLookup 合并 session 与 agent 两个作用域的读取
// （批 C：签名由具体 Store 泛化为 ApprovalPolicyReader——*biztool.ToolUsecase
// 装配 WithApprovalPolicyStore 后自动满足）。装配 ToolUsecase 读口时可直接
// 复用本组合函数；sessionID 需每次调用解析的场景用 approvalPolicyLookupForDeps。
func CompositeApprovalPolicyLookup(reader biztool.ApprovalPolicyReader, sessionID, agentID string, nowUnix int64) ApprovalPolicyLookup {
	return func(ctx context.Context) ([]biztool.ApprovalPolicy, error) {
		return listMergedApprovalPolicies(ctx, reader, sessionID, agentID, nowUnix)
	}
}

// approvalPolicyLookupForDeps 从 deps.ToolUC 可选接口断言装配审批策略读口
// （P0-5 批 C activation）：ToolUsecase 实现 biztool.ApprovalPolicyReader 后
// 闸才注册；未装配（CLI/旧 usecase/nil store）返回 nil——运行时零开销、
// 行为与现状字节级一致。sessionID 每次调用经 gateSessionID(ctx) 解析
// （team 成员路径用 WithGateSessionID 注入值，chat 路径回落 invocation 会话）。
func approvalPolicyLookupForDeps(ag biz.Agent, deps TRPCBuilderDeps) ApprovalPolicyLookup {
	if deps.ToolUC == nil {
		return nil
	}
	reader, ok := deps.ToolUC.(biztool.ApprovalPolicyReader)
	if !ok || reader == nil {
		return nil
	}
	// typed-nil 防护（同 trpc_build.go runtimeIface 先例）：接口值包装 nil
	// 指针时 ListEnabled 方法内的 u==nil 守卫返回 nil,nil，无需额外处理。
	agentID := ag.ID
	return func(ctx context.Context) ([]biztool.ApprovalPolicy, error) {
		return listMergedApprovalPolicies(ctx, reader, gateSessionID(ctx), agentID, time.Now().Unix())
	}
}

// approvalPolicyCommandText 从工具参数 JSON 提取命令匹配文本：仅认
// command/cmd 键的首个字符串值（缺失/非 JSON→空串，command_prefix 维度不参与）。
func approvalPolicyCommandText(arguments []byte) string {
	var v map[string]any
	if err := json.Unmarshal(arguments, &v); err != nil {
		return ""
	}
	for _, k := range []string{"command", "cmd"} {
		if s, ok := v[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// newApprovalPolicyGateBeforeHook 装配审批策略闸。lookup 为 nil（策略存储未
// 装配）时返回 nil 不注册——默认 OFF 状态与现状字节级一致。
// 批 C（2026-10-22 activation）：callback_chain.go 在 paramRuleGate 注册之后、
// args 守卫之前追加本 hook，lookup 经 approvalPolicyLookupForDeps 从
// deps.ToolUC 可选断言（ApprovalPolicyReader）+ 双作用域合并构建。
func newApprovalPolicyGateBeforeHook(ag biz.Agent, deps TRPCBuilderDeps, lookup ApprovalPolicyLookup) callbacks.BeforeToolHook {
	if lookup == nil {
		return nil
	}
	lg := deps.Logger()
	return callbacks.NewBeforeToolHook(approvalPolicyGatePriority, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		toolName := strings.TrimSpace(args.ToolName)
		if toolName == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		policies, err := lookup(ctx)
		if err != nil {
			lg.Warn("approval policy gate: list rules failed, degrade to ask",
				loggateway.StepID("agent.approval_policy_gate"),
				loggateway.Str("tool", toolName),
				loggateway.Err(err))
			// 读取失败无法区分有/无 never 规则：fail-closed 折叠为 ask，
			// 确认门禁人工兜底（与 paramRuleGate 同一先例）。
			v := &approvalPolicyVerdict{mode: biztool.ApprovalModeAsk, ruleID: "load-error"}
			return &trpctool.BeforeToolResult{Context: context.WithValue(ctx, approvalPolicyVerdictCtxKey{}, v)}, nil
		}
		if len(policies) == 0 {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		// 与 paramRuleGate 同一键空间与匹配文本构建器（复用其校验/解析，不改其语义）。
		canonical := loopGuardCanonicalToolName(toolName)
		in := biztool.ApprovalMatchInput{
			ToolName: canonical,
			Command:  approvalPolicyCommandText(args.Arguments),
			ArgsText: paramRuleMatchText(args.Arguments),
		}
		enforce, shadow := biztool.EvalApprovalPolicies(policies, in, time.Now().Unix())
		if shadow != nil {
			// shadow_only 预演：命中必落 shadow 审计 + notice，零强制。
			// AS-EVT-01=Informational（观测语义，不阻断、不上 BlockUpTo 行列）。
			lg.Warn("approval policy shadow match (would enforce)",
				loggateway.StepID("agent.approval_policy_gate"),
				loggateway.Str("tool", toolName),
				loggateway.Str("rule_id", shadow.ID),
				loggateway.Str("mode", shadow.Mode),
				loggateway.Str("scope", shadow.Scope))
			event.EmitGate(ctx, deps.DecisionCollector, decision.GateDecision{
				TriggerRule: triggerApprovalPolicyShadow,
				Outcome:     "tripped",
				Scenario:    "审批策略 shadow 预演命中: " + toolName,
				Reasoning:   "rule " + shadow.ID + " mode " + shadow.Mode,
				GuardName:   "approval_policy_gate",
				RunID:       gateRunID(ctx),
				SessionID:   gateSessionID(ctx),
				Entities:    []decision.EntityRef{{Type: "tool", Key: toolName}},
				Threshold:   shadow.Mode,
				Action:      "shadow",
				Extra:       map[string]any{"shadow_only": true, "rule_id": shadow.ID, "scope": shadow.Scope, "scope_key": shadow.ScopeKey},
			})
			if emitter := biz.ActivityEmitterFromContext(ctx); emitter != nil {
				if noticeErr := emitter.EmitNotice(ctx, "审批策略（预演）规则 "+shadow.ID+" 命中工具 "+toolName+"：正式态下将按 "+shadow.Mode+" 处置", "approval_policy_shadow"); noticeErr != nil {
					lg.Warn("approval policy shadow notice failed",
						loggateway.StepID("agent.approval_policy_gate"),
						loggateway.Err(noticeErr))
				}
			}
		}
		if enforce == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		switch biztool.ApprovalMode(enforce.Mode) {
		case biztool.ApprovalModeNever:
			lg.Info("approval policy gate denied tool call",
				loggateway.StepID("agent.approval_policy_gate"),
				loggateway.Str("tool", toolName),
				loggateway.Str("rule_id", enforce.ID),
				loggateway.Str("scope", enforce.Scope))
			recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
				ToolKey:      toolName,
				AgentID:      ag.ID,
				Status:       "blocked",
				ErrorCode:    errorCodeApprovalPolicyDenied,
				ErrorMessage: "denied by approval policy " + enforce.ID + " (scope: " + enforce.Scope + ":" + enforce.ScopeKey + ")",
				InputPreview: previewFromToolArgs(args.Arguments),
				StartedAt:    time.Now().UTC().Format(time.RFC3339),
				EndedAt:      time.Now().UTC().Format(time.RFC3339),
				Source:       biz.ToolInvocationSourceRuntime,
				ToolCallID:   args.ToolCallID,
				ParamsJSON:   paramsJSONFromToolArgs(args.Arguments),
			}, nil, ag, deps)
			// approval_policy_denied 决策双写（Informational，§9.3）；notice 复用
			// SystemNoticeEvent——显式反 BlockUpTo，防 never 静默拒绝 Ops 无感知（反作用#2）。
			event.EmitGate(ctx, deps.DecisionCollector, decision.GateDecision{
				TriggerRule:   triggerApprovalPolicyDenied,
				Outcome:       "blocked",
				Scenario:      "审批策略拒绝工具调用: " + toolName,
				Reasoning:     "rule " + enforce.ID + " scope " + enforce.Scope + ":" + enforce.ScopeKey,
				GuardName:     "approval_policy_gate",
				RunID:         gateRunID(ctx),
				SessionID:     gateSessionID(ctx),
				Entities:      []decision.EntityRef{{Type: "tool", Key: toolName}},
				ObservedValue: previewFromToolArgs(args.Arguments),
				Threshold:     string(biztool.ApprovalModeNever),
				Action:        "deny",
				Extra:         map[string]any{"rule_id": enforce.ID, "scope": enforce.Scope, "scope_key": enforce.ScopeKey},
			})
			if emitter := biz.ActivityEmitterFromContext(ctx); emitter != nil {
				if noticeErr := emitter.EmitNotice(ctx, "工具 "+toolName+" 的本次调用被审批策略规则 "+enforce.ID+" 确定性拒绝（scope "+enforce.Scope+"）", triggerApprovalPolicyDenied); noticeErr != nil {
					lg.Warn("approval policy denied notice failed",
						loggateway.StepID("agent.approval_policy_gate"),
						loggateway.Err(noticeErr))
				}
			}
			// LLM 可见文案只给规则 ID——pattern/匹配维度不回给模型，防针对性变形
			// 绕过（镜像 param_rule_deny 信息论保护；审计侧 ErrorMessage/决策记录可溯）。
			return callbacks.Reject("工具 \"" + toolName + "\" 的本次调用被审批策略拒绝（规则 " + enforce.ID + "）。这是管理员配置的确定性安全姿态，不是临时故障。禁止重试相同或等价的调用；请向用户说明该操作被策略禁止，并询问替代方案。").BeforeToolResult(ctx), nil
		case biztool.ApprovalModeAsk, biztool.ApprovalModeTrusted, biztool.ApprovalModeAuto:
			v := &approvalPolicyVerdict{mode: biztool.ApprovalMode(enforce.Mode), ruleID: enforce.ID}
			return &trpctool.BeforeToolResult{Context: context.WithValue(ctx, approvalPolicyVerdictCtxKey{}, v)}, nil
		default:
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
	})
}
