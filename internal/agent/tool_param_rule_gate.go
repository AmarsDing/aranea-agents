package agent

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/biz/policyrule"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// 79-runtime-governance R9（Phase 5.4）：工具参数模式权限门禁。
// priority 3，在循环守卫（priority 4）与确认门禁（priority 10）之前求值：
//   - deny  → 立即 Reject（CustomResult 短路），记 tool_invocations blocked +
//     M80 system_guard 决策（param_rule_deny）；先于循环守卫，deny 重发不消耗
//     循环守卫计数、不被循环守卫的纠偏消息覆盖语义。
//   - ask   → 裁定写入 ctx，确认门禁按 confirmReasonParamRuleAsk 强制确认
//    （session/persisted grant 仍可满足，与 catalog 路径同语义）。
//   - allow → 裁定写入 ctx，确认门禁跳过 catalog/plugin 确认
//    （computer-use danger floor 永不被跳过）。
//   - 无命中 → fallback：工具自身 requires_confirmation（目录策略）决定。
//
// 规则读取失败 fail-open（记 warn 放行）：目录确认策略仍是基础层，DB 抖动
// 不扩大故障面。

// paramRuleGatePriority 必须小于循环守卫的 priority 4。
const paramRuleGatePriority = 3

// paramRuleVerdictCtxKey 是 paramRuleGate → 确认门禁的 ctx 传递键。
type paramRuleVerdictCtxKey struct{}

// paramRuleVerdict 是 paramRuleGate 的命中裁定（仅 ask/allow 入 ctx；
// deny 在 gate 内直接拒绝，不下传）。
type paramRuleVerdict struct {
	effect  policyrule.Effect
	ruleID  string
	pattern string
}

// paramRuleVerdictFromCtx 读取 gate 裁定；未命中/未装配返回 nil。
func paramRuleVerdictFromCtx(ctx context.Context) *paramRuleVerdict {
	v, _ := ctx.Value(paramRuleVerdictCtxKey{}).(*paramRuleVerdict)
	return v
}

// newParamRuleGateBeforeHook 装配参数规则门禁。deps.ToolUC 未装配（或规则
// 存储缺失）时返回 nil 不注册——运行时每次调用零开销。
func newParamRuleGateBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.BeforeToolHook {
	if deps.ToolUC == nil {
		return nil
	}
	lg := deps.Logger()
	return callbacks.NewBeforeToolHook(paramRuleGatePriority, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		toolName := strings.TrimSpace(args.ToolName)
		if toolName == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		// 与循环守卫同一键空间：ToolSet 前缀形态剥离 + 别名链定点，
		// 管理面任一别名写入的规则对运行时变体名同效。
		canonical := loopGuardCanonicalToolName(toolName)
		rules, err := deps.ToolUC.ListEnabledParamRulesForGate(ctx, canonical)
		if err != nil {
			lg.Warn("param rule gate: list rules failed, fail-open",
				loggateway.StepID("agent.param_rule_gate"),
				loggateway.Str("tool", toolName),
				loggateway.Err(err))
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if len(rules) == 0 {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		evalRules := make([]policyrule.Rule, 0, len(rules))
		for _, r := range rules {
			evalRules = append(evalRules, policyrule.Rule{
				ID:       r.ID,
				Pattern:  r.Pattern,
				Effect:   policyrule.Effect(r.Effect),
				Priority: r.Priority,
				Enabled:  r.Enabled,
			})
		}
		winner, matchErr := policyrule.Evaluate(evalRules, paramRuleMatchText(args.Arguments))
		if matchErr != nil {
			lg.Warn("param rule gate: bad pattern skipped",
				loggateway.StepID("agent.param_rule_gate"),
				loggateway.Str("tool", toolName),
				loggateway.Err(matchErr))
		}
		if winner == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		switch winner.Effect {
		case policyrule.EffectDeny:
			lg.Info("param rule gate denied tool call",
				loggateway.StepID("agent.param_rule_gate"),
				loggateway.Str("tool", toolName),
				loggateway.Str("rule_id", winner.ID),
				loggateway.Str("pattern", winner.Pattern))
			recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
				ToolKey:      toolName,
				AgentID:      ag.ID,
				Status:       "blocked",
				ErrorCode:    event.ErrorCodeParamRuleDenied,
				ErrorMessage: "denied by tool param rule " + winner.ID + " (pattern: " + winner.Pattern + ")",
				InputPreview: previewFromToolArgs(args.Arguments),
				StartedAt:    time.Now().UTC().Format(time.RFC3339),
				EndedAt:      time.Now().UTC().Format(time.RFC3339),
				Source:       biz.ToolInvocationSourceRuntime,
				ToolCallID:   args.ToolCallID,
				ParamsJSON:   paramsJSONFromToolArgs(args.Arguments),
			}, nil, ag, deps)
			// M80 决策双写（trigger 枚举 C6 已预留 param_rule_deny 挂点）。
			var runID string
			if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
				runID = inv.InvocationID
			}
			decision.EmitGate(ctx, deps.DecisionCollector, decision.GateDecision{
				TriggerRule:   decision.TriggerParamRuleDeny,
				Outcome:       "blocked",
				Scenario:      "工具参数命中 deny 规则: " + toolName,
				Reasoning:     "rule " + winner.ID + " pattern " + winner.Pattern,
				GuardName:     "param_rule_gate",
				RunID:         runID,
				Entities:      []decision.EntityRef{{Type: "tool", Key: toolName}},
				ObservedValue: previewFromToolArgs(args.Arguments),
				Threshold:     winner.Pattern,
				Action:        "deny",
			})
			return callbacks.Reject("工具 \"" + toolName + "\" 的本次调用被参数权限规则拒绝（规则 " + winner.ID + "，模式 " + winner.Pattern + "）。这是管理员配置的安全策略，不是临时故障。禁止重试相同或等价的调用；请向用户说明该操作被策略禁止，并询问替代方案。").BeforeToolResult(ctx), nil
		case policyrule.EffectAsk, policyrule.EffectAllow:
			v := &paramRuleVerdict{effect: winner.Effect, ruleID: winner.ID, pattern: winner.Pattern}
			return &trpctool.BeforeToolResult{Context: context.WithValue(ctx, paramRuleVerdictCtxKey{}, v)}, nil
		default:
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
	})
}

// paramRuleMatchText 从工具参数 JSON 构建匹配文本：递归收集全部字符串值，
// map 键按字典序遍历保证确定性（与循环守卫签名归一互补：那边归一码点
// 微差异，这边把结构参数压平成可 glob 的命令文本）。非 JSON 参数原样返回。
func paramRuleMatchText(arguments []byte) string {
	var v any
	if err := json.Unmarshal(arguments, &v); err != nil {
		return strings.TrimSpace(string(arguments))
	}
	var sb strings.Builder
	collectParamRuleStrings(v, &sb)
	return strings.TrimSpace(sb.String())
}

func collectParamRuleStrings(v any, sb *strings.Builder) {
	switch t := v.(type) {
	case string:
		if sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			collectParamRuleStrings(t[k], sb)
		}
	case []any:
		for _, item := range t {
			collectParamRuleStrings(item, sb)
		}
	}
}
