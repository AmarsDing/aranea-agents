package agent

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/policyrule"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// newPluginAllowGate 构造 pluginAllowWithoutChannel 返回 true 的门禁：
// 插件 default_action=allow 且 ConfirmTools 列名该工具（catalog 为空）。
func newPluginAllowGate(toolKey string) *toolConfirmGate {
	return &toolConfirmGate{
		plugin: plugintrpc.ConfirmationGuardConfig{
			DefaultAction: "allow",
			ConfirmTools:  []string{toolKey},
		},
		hasPlugin:     true,
		sessionGrants: newToolGrantStore(time.Now),
	}
}

// TestToolConfirm_AskVerdictNotShortCircuitedByPluginAllow 钉死 2026-08-27
// 二轮审查根修：param 规则 ask 裁定（含规则读取失败的 load-error 降级）优先于
// 插件 allow 早退——pluginAllowWithoutChannel 不消费 ctx verdict，命中 ask 的
// 调用必须落入 decide() 走确认流程，而非被静默放行。
func TestToolConfirm_AskVerdictNotShortCircuitedByPluginAllow(t *testing.T) {
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	h := newToolConfirmationBeforeHook(newPluginAllowGate("gns3_exec"), biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)

	replyCalled := false
	ctx := decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
		replyCalled = true
		return serviceawaitreply.ReplyApprove, nil
	})
	// 模拟 paramRuleGate（priority 3，先行）写入的 ask 裁定。
	ctx = context.WithValue(ctx, paramRuleVerdictCtxKey{}, &paramRuleVerdict{
		effect: policyrule.EffectAsk, ruleID: "builtin-gns3-fallback-ask", pattern: "*",
	})

	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName:   "gns3_exec",
		Arguments:  []byte(`{"command":"reload"}`),
		ToolCallID: "tc-1",
	}); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if !replyCalled {
		t.Fatal("ask verdict must force the confirmation flow (reply channel), not be silently allowed by plugin early-return")
	}
	// approve 后产出 hitl_approval 记录，decision_reason 必须是 param_rule_ask——
	// 证明走的是 decide() 的 ask 分支而非插件静默放行。
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 decision record, got %d", len(cc.recs))
	}
	if got := cc.recs[0].Metadata["decision_reason"]; got != confirmReasonParamRuleAsk {
		t.Fatalf("decision_reason = %v, want %q", got, confirmReasonParamRuleAsk)
	}
}

// TestToolConfirm_PluginAllowEarlyReturnWithoutVerdict 对照组：无 ask 裁定时
// 插件 allow 早退路径保持原语义——不调回复通道、不产决策记录、ctx 标记
// handled（P1-10，防 ConfirmationGuardPlugin 重复硬拦）。
func TestToolConfirm_PluginAllowEarlyReturnWithoutVerdict(t *testing.T) {
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	h := newToolConfirmationBeforeHook(newPluginAllowGate("gns3_exec"), biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)

	replyCalled := false
	ctx := decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
		replyCalled = true
		return serviceawaitreply.ReplyApprove, nil
	})

	res, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName:   "gns3_exec",
		Arguments:  []byte(`{"command":"reload"}`),
		ToolCallID: "tc-1",
	})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if replyCalled {
		t.Fatal("plugin allow early-return must not invoke the confirmation reply channel")
	}
	if !plugintrpc.ToolConfirmHandled(res.Context) {
		t.Fatal("plugin allow early-return must mark the context handled (P1-10)")
	}
	if len(cc.recs) != 0 {
		t.Fatalf("plugin allow early-return must not emit decision records, got %d", len(cc.recs))
	}
}
