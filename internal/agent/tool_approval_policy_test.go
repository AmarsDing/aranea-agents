package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/biz/policyrule"
	biztool "aranea-agents/internal/biz/tool"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// fakeConfirmEmitter 捕获 SystemNoticeEvent（never/shadow 命中）并固定确认卡
// id（approval.asked/decided correlation 配对断言）。
type fakeConfirmEmitter struct {
	notices   []string
	confirmID string
}

func (f *fakeConfirmEmitter) EmitNotice(_ context.Context, _ string, noticeType string) error {
	f.notices = append(f.notices, noticeType)
	return nil
}
func (f *fakeConfirmEmitter) EmitConfirmRequest(_ context.Context, _ biz.ActivityConfirmParams) (string, error) {
	return f.confirmID, nil
}
func (f *fakeConfirmEmitter) EmitConfirmResult(_ context.Context, _ string, _ bool) error { return nil }
func (f *fakeConfirmEmitter) EmitConfirmTimeout(_ context.Context, _ string) error        { return nil }

func approvalPolicyTestRule(id, mode string, shadow bool) biztool.ApprovalPolicy {
	return biztool.ApprovalPolicy{
		ID:            id,
		Scope:         string(biztool.ApprovalScopeSession),
		ScopeKey:      "sess-1",
		ToolPattern:   "*",
		CommandPrefix: "rm -rf*",
		Mode:          mode,
		Priority:      100,
		Enabled:       true,
		ShadowOnly:    shadow,
	}
}

// newApprovalPolicyTestHook 构造带写入捕获与决策捕获的审批策略闸；cc 可为 nil。
func newApprovalPolicyTestHook(t *testing.T, cc *captureDecisionCollector, rules []biztool.ApprovalPolicy, lookupErr error) (callbacks.BeforeToolHook, *gateToolLookup) {
	t.Helper()
	lookup := &gateToolLookup{writes: make(chan biztool.ToolInvocationWrite, 1)}
	deps := TRPCBuilderDeps{}
	deps.ToolUC = lookup
	if cc != nil {
		deps.DecisionCollector = cc
	}
	hook := newApprovalPolicyGateBeforeHook(biz.Agent{ID: "agent-1"}, deps, func(context.Context) ([]biztool.ApprovalPolicy, error) {
		return rules, lookupErr
	})
	if hook == nil {
		t.Fatal("hook should be registered when lookup present")
	}
	return hook, lookup
}

func approvalPolicyCtxWithEmitter(ctx context.Context, em biz.ActivityEmitter) context.Context {
	return biz.WithActivityEmitter(ctx, em)
}

func TestApprovalPolicyGate_NilLookupNotRegistered(t *testing.T) {
	t.Parallel()
	if hook := newApprovalPolicyGateBeforeHook(biz.Agent{ID: "a"}, TRPCBuilderDeps{}, nil); hook != nil {
		t.Fatal("nil lookup must skip registration (OFF byte-compat)")
	}
}

// TestApprovalPolicyGate_NeverDeterministicReject 钉死 never 五件事镜像
// param_rule_deny：CustomResult 短路 + 文案只给规则 ID 不给 pattern + 不写
// ctx verdict + tool_invocations blocked 写 + EmitGate approval_policy_denied
// 触发（+ SystemNoticeEvent，Informational 反静默）。先于循环守卫（priority
// 3 < 4）→ 短路不消耗 inflight 槽位。
func TestApprovalPolicyGate_NeverDeterministicReject(t *testing.T) {
	t.Parallel()
	cc := &captureDecisionCollector{}
	hook, lookup := newApprovalPolicyTestHook(t, cc, []biztool.ApprovalPolicy{
		approvalPolicyTestRule("builtin-never-rmrf", string(biztool.ApprovalModeNever), false),
	}, nil)
	if hook.Priority() != approvalPolicyGatePriority || approvalPolicyGatePriority >= 4 {
		t.Fatalf("审批闸必须先于循环守卫(4)求值——never 短路不消耗 inflight，priority=%d", hook.Priority())
	}
	em := &fakeConfirmEmitter{}
	ctx := approvalPolicyCtxWithEmitter(context.Background(), em)
	res, err := hook.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"rm -rf /tmp/x"}`), ToolCallID: "call-9",
	})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res.CustomResult == nil {
		t.Fatal("never should short-circuit with CustomResult")
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "builtin-never-rmrf") {
		t.Fatalf("reject message should name the rule id, got %q", msg)
	}
	if strings.Contains(msg, "rm -rf") {
		t.Fatalf("reject message must not leak pattern text (信息论保护), got %q", msg)
	}
	if v := approvalPolicyVerdictFromCtx(res.Context); v != nil {
		t.Fatalf("never must not write ctx verdict, got %+v", v)
	}
	select {
	case w := <-lookup.writes:
		if w.Status != "blocked" {
			t.Errorf("blocked write Status = %q, want blocked", w.Status)
		}
		if w.ErrorCode != errorCodeApprovalPolicyDenied {
			t.Errorf("blocked write ErrorCode = %q, want %q", w.ErrorCode, errorCodeApprovalPolicyDenied)
		}
		if w.ToolCallID != "call-9" {
			t.Errorf("blocked write ToolCallID = %q, want call-9", w.ToolCallID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("never 后未捕获 tool_invocations blocked 写入")
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 gate record, got %d", len(cc.recs))
	}
	r := cc.recs[0]
	if got := r.Metadata["trigger_rule"]; got != triggerApprovalPolicyDenied {
		t.Errorf("trigger_rule = %v, want %q", got, triggerApprovalPolicyDenied)
	}
	if r.Outcome != "blocked" {
		t.Errorf("gate outcome = %q, want blocked", r.Outcome)
	}
	if got := r.Metadata["rule_id"]; got != "builtin-never-rmrf" {
		t.Errorf("metadata.rule_id = %v", got)
	}
	if len(em.notices) != 1 || em.notices[0] != triggerApprovalPolicyDenied {
		t.Fatalf("SystemNoticeEvent approval_policy_denied expected once, got %v", em.notices)
	}
}

// TestApprovalPolicyGate_ShadowZeroEnforce 钉死 shadow_only=1：零拒绝、必落
// shadow 审计（EmitGate tripped + shadow_only 标记）+ notice。
func TestApprovalPolicyGate_ShadowZeroEnforce(t *testing.T) {
	t.Parallel()
	cc := &captureDecisionCollector{}
	store := biztool.NewInMemApprovalPolicyStore()
	if err := store.UpsertApprovalPolicy(context.Background(), approvalPolicyTestRule("shadow-never-1", string(biztool.ApprovalModeNever), true)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	hook := newApprovalPolicyGateBeforeHook(biz.Agent{ID: "agent-1"}, deps,
		CompositeApprovalPolicyLookup(store, "sess-1", "agent-1", time.Now().Unix()))
	if hook == nil {
		t.Fatal("hook should be registered")
	}
	em := &fakeConfirmEmitter{}
	ctx := approvalPolicyCtxWithEmitter(context.Background(), em)
	res, err := hook.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"rm -rf /tmp/x"}`), ToolCallID: "call-10",
	})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res.CustomResult != nil {
		t.Fatalf("shadow must never reject, got %v", res.CustomResult)
	}
	// D5-f F15：shadow 分支零 verdict 写——shadow=纯观测语义，绝不产生
	// 消费侧裁定（下游 decide() 不得见到本规则的 verdict）。
	if v := approvalPolicyVerdictFromCtx(res.Context); v != nil {
		t.Fatalf("shadow must not write ctx verdict (零强制语义), got %+v", v)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("shadow audit record expected, got %d", len(cc.recs))
	}
	if got := cc.recs[0].Metadata["trigger_rule"]; got != triggerApprovalPolicyShadow {
		t.Errorf("trigger_rule = %v, want %q", got, triggerApprovalPolicyShadow)
	}
	if cc.recs[0].Outcome != "tripped" {
		t.Errorf("shadow outcome = %q, want tripped", cc.recs[0].Outcome)
	}
	if got := cc.recs[0].Metadata["shadow_only"]; got != true {
		t.Errorf("metadata.shadow_only = %v, want true", got)
	}
	if len(em.notices) != 1 || em.notices[0] != triggerApprovalPolicyShadow {
		t.Fatalf("shadow notice expected once, got %v", em.notices)
	}
}

// TestApprovalPolicyGate_LoadErrorFailsClosed 钉死规则读取失败 fail-closed
// 折叠为 ask verdict（同 paramRuleGate 先例），绝不 fail-open。
func TestApprovalPolicyGate_LoadErrorFailsClosed(t *testing.T) {
	t.Parallel()
	hook, _ := newApprovalPolicyTestHook(t, nil, nil, errors.New("db down"))
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"ls"}`), ToolCallID: "call-11",
	})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res.CustomResult != nil {
		t.Fatalf("load error must not reject directly, got %v", res.CustomResult)
	}
	v := approvalPolicyVerdictFromCtx(res.Context)
	if v == nil || v.mode != biztool.ApprovalModeAsk || v.ruleID != "load-error" {
		t.Fatalf("load error must degrade to ask verdict, got %+v", v)
	}
}

// TestApprovalPolicyGate_AskTrustedVerdict 钉死 ask/trusted 命中只写 ctx
// 裁定（消费在确认门禁 decide()，本闸不短路与不放行）。
func TestApprovalPolicyGate_AskTrustedVerdict(t *testing.T) {
	t.Parallel()
	mk := func(mode string) []biztool.ApprovalPolicy {
		return []biztool.ApprovalPolicy{approvalPolicyTestRule("r-"+mode, mode, false)}
	}
	hook, _ := newApprovalPolicyTestHook(t, nil, mk(string(biztool.ApprovalModeAsk)), nil)
	res, _ := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"rm -rf /a"}`), ToolCallID: "c1",
	})
	if res.CustomResult != nil {
		t.Fatal("ask must not reject")
	}
	if v := approvalPolicyVerdictFromCtx(res.Context); v == nil || v.mode != biztool.ApprovalModeAsk {
		t.Fatalf("ask verdict missing, got %+v", v)
	}
	hook, _ = newApprovalPolicyTestHook(t, nil, mk(string(biztool.ApprovalModeTrusted)), nil)
	res, _ = hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"rm -rf /a"}`), ToolCallID: "c2",
	})
	if v := approvalPolicyVerdictFromCtx(res.Context); v == nil || v.mode != biztool.ApprovalModeTrusted {
		t.Fatalf("trusted verdict missing, got %+v", v)
	}
}

// decide() 层档位测试（设计 §6.7 ② hook 测试族）。

func approvalVerdictTestCtx(mode biztool.ApprovalMode) context.Context {
	return context.WithValue(context.Background(), approvalPolicyVerdictCtxKey{}, &approvalPolicyVerdict{mode: mode, ruleID: "r1"})
}

// TestDecide_DangerFloorBeatsTrusted 钉死 danger floor（computer-use 危险词 /
// injectionSuspected）恒优先于 trusted——trusted 不可绕过危险地板。
func TestDecide_DangerFloorBeatsTrusted(t *testing.T) {
	t.Parallel()
	gate := &toolConfirmGate{
		injectionSuspected: func(string) bool { return true },
		agentKey:           "ak-1",
	}
	d := gate.decide(approvalVerdictTestCtx(biztool.ApprovalModeTrusted), "sess-1", "agent-1", "computer_use_act", []byte(`{"target":"ok"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyDanger {
		t.Fatalf("danger floor must win over trusted, got %+v", d)
	}
}

// TestDecide_TrustedSkipsCatalog 钉死 trusted 档位越过 catalog/plugin 确认。
func TestDecide_TrustedSkipsCatalog(t *testing.T) {
	t.Parallel()
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	d := gate.decide(approvalVerdictTestCtx(biztool.ApprovalModeTrusted), "sess-1", "agent-1", "bash", []byte(`{"command":"ls"}`))
	if d.needsConfirm || d.reason != confirmReasonPolicyTrusted {
		t.Fatalf("trusted must skip catalog/plugin, got %+v", d)
	}
}

// TestDecide_TrustedSatisfiesParamRuleAsk 钉死 param rule ask 判决可被
// trusted 满足（grant 同语义——approve_always 对 ask 规则等效放开）。
func TestDecide_TrustedSatisfiesParamRuleAsk(t *testing.T) {
	t.Parallel()
	gate := &toolConfirmGate{sessionGrants: newToolGrantStore(time.Now)}
	ctx := context.WithValue(approvalVerdictTestCtx(biztool.ApprovalModeTrusted), paramRuleVerdictCtxKey{}, &paramRuleVerdict{
		effect: policyrule.EffectAsk, ruleID: "builtin-ask",
	})
	d := gate.decide(ctx, "sess-1", "agent-1", "gns3_exec", []byte(`{"command":"reload"}`))
	if d.needsConfirm || d.reason != confirmReasonPolicyTrusted {
		t.Fatalf("trusted must satisfy param rule ask (grant 同语义), got %+v", d)
	}
}

// TestDecide_PolicyAskForcesConfirmAndGrantSatisfies 钉死 policy ask 强制确认
// + session grant 可满足（与 catalog/param ask 的 grant 语义对齐）。
func TestDecide_PolicyAskForcesConfirmAndGrantSatisfies(t *testing.T) {
	t.Parallel()
	gate := &toolConfirmGate{sessionGrants: newToolGrantStore(time.Now)}
	d := gate.decide(approvalVerdictTestCtx(biztool.ApprovalModeAsk), "sess-1", "agent-1", "uncatalogued_tool", []byte(`{"x":"y"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyAsk {
		t.Fatalf("policy ask must force confirmation, got %+v", d)
	}
	gate.sessionGrants.GrantSession("sess-1", "agent-1", "uncatalogued_tool")
	d = gate.decide(approvalVerdictTestCtx(biztool.ApprovalModeAsk), "sess-1", "agent-1", "uncatalogued_tool", []byte(`{"x":"y"}`))
	if d.needsConfirm || d.reason != confirmReasonGrantSession {
		t.Fatalf("session grant must satisfy policy ask, got %+v", d)
	}
}

// TestDecide_OffStateByteIdentical 对照组：无成文策略时决策链与现状逐字节
// 一致（未装配方 store → hook 为 nil → ctx 无 verdict → decide 走原链）。
func TestDecide_OffStateByteIdentical(t *testing.T) {
	t.Parallel()
	gate := &toolConfirmGate{sessionGrants: newToolGrantStore(time.Now)}
	d := gate.decide(context.Background(), "sess-1", "agent-1", "uncatalogued_tool", []byte(`{"x":"y"}`))
	if d.needsConfirm || d.reason != confirmReasonDefaultAllow {
		t.Fatalf("OFF state must be byte-identical to legacy chain, got %+v", d)
	}
}

// TestToolConfirm_PolicyAskNotShortCircuitedByPluginAllow 钉死审批策略 ask 不被
// 插件 allow 早退静默放行（与 param rule ask 同一不变量）。
func TestToolConfirm_PolicyAskNotShortCircuitedByPluginAllow(t *testing.T) {
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	h := newToolConfirmationBeforeHook(newPluginAllowGate("gns3_exec"), biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)

	replyCalled := false
	ctx := decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
		replyCalled = true
		return serviceawaitreply.ReplyApprove, nil
	})
	ctx = context.WithValue(ctx, approvalPolicyVerdictCtxKey{}, &approvalPolicyVerdict{mode: biztool.ApprovalModeAsk, ruleID: "r-ask"})

	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "gns3_exec", Arguments: []byte(`{"command":"reload"}`), ToolCallID: "tc-1",
	}); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if !replyCalled {
		t.Fatal("policy ask must force the confirmation flow, not be silently allowed by plugin early-return")
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	if got := cc.recs[0].Metadata["decision_reason"]; got != confirmReasonPolicyAsk {
		t.Fatalf("decision_reason = %v, want %q", got, confirmReasonPolicyAsk)
	}
}

// TestToolConfirm_AskedDecidedPairing 钉死 approval.asked/decided 对偶：首张
// 确认卡发出即落 asked（Actor=system:hitl_approval），decided 半部携同一
// correlation_id（ability 可回收）。
func TestToolConfirm_AskedDecidedPairing(t *testing.T) {
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)

	em := &fakeConfirmEmitter{confirmID: "act-111"}
	ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyApprove, nil
	}), em)

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 2 {
		t.Fatalf("expected asked+decided pair, got %d records", len(cc.recs))
	}
	asked, decided := cc.recs[0], cc.recs[1]
	if asked.Outcome != "asked" || asked.ActorType != decision.ActorSystem || asked.ActorKey != "system:hitl_approval" {
		t.Fatalf("asked half wrong: %+v", asked)
	}
	if got := asked.Metadata["correlation_id"]; got != "act-111" {
		t.Fatalf("asked correlation_id = %v, want act-111", got)
	}
	if decided.Outcome != "approved" {
		t.Fatalf("decided outcome = %q, want approved", decided.Outcome)
	}
	if got := decided.Metadata["correlation_id"]; got != "act-111" {
		t.Fatalf("decided correlation must recycle the first card id, got %v", got)
	}
}

// TestToolConfirm_TimeoutAskedDecidedPairing 超时出口同样成对（重试补发卡沿用
// 首个卡片 id——全场确认 1:1 配对，不重发 asked）。
func TestToolConfirm_TimeoutAskedDecidedPairing(t *testing.T) {
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)
	h.confirmTimeout = 20 * time.Millisecond
	h.confirmRetries = -1

	em := &fakeConfirmEmitter{confirmID: "act-222"}
	ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}), em)

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 2 {
		t.Fatalf("expected asked+timeout pair, got %d records", len(cc.recs))
	}
	askedCount := 0
	timeoutCount := 0
	for i := range cc.recs {
		if cc.recs[i].Outcome == "asked" {
			askedCount++
		}
		if cc.recs[i].Outcome == "timeout" {
			timeoutCount++
		}
		if got := cc.recs[i].Metadata["correlation_id"]; got != "act-222" {
			t.Fatalf("both halves must carry first-card correlation, got %v", got)
		}
	}
	if askedCount != 1 {
		t.Fatalf("asked must be emitted exactly once across retries, got %d", askedCount)
	}
	if timeoutCount != 1 {
		t.Fatalf("timeout decided record missing, got %d timeout records", timeoutCount)
	}
}

// TestToolConfirm_NoReplyChannelNoPair 无回复通道出口：确认卡从未发出（无人被
// 真正询问）→ 无 asked 半部，decided 不携 correlation_id（语义诚实，五出口
// 配对约束仅覆盖有真实询问的流向）。
func TestToolConfirm_NoReplyChannelNoPair(t *testing.T) {
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)

	em := &fakeConfirmEmitter{confirmID: "act-333"}
	ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", nil), em)

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("no-channel exit must emit exactly one decided record, got %d", len(cc.recs))
	}
	if got := cc.recs[0].Metadata["correlation_id"]; got != nil {
		t.Fatalf("no-channel decided must not carry correlation_id, got %v", got)
	}
	if cc.recs[0].Outcome != "rejected" {
		t.Fatalf("outcome = %q, want rejected", cc.recs[0].Outcome)
	}
}

// TestToolConfirm_GrantSkipNoAskedPair grant 秒过出口同样无询问发生：只有
// grant_skip decided，无 asked 半部、无 correlation_id。
func TestToolConfirm_GrantSkipNoAskedPair(t *testing.T) {
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	gate.sessionGrants.GrantSession("sess-1", "agent-1", "bash")
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)

	em := &fakeConfirmEmitter{confirmID: "act-444"}
	ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", nil), em)

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("grant_skip must emit exactly one record, got %d", len(cc.recs))
	}
	if cc.recs[0].Outcome != "grant_skip" {
		t.Fatalf("outcome = %q, want grant_skip", cc.recs[0].Outcome)
	}
	if got := cc.recs[0].Metadata["correlation_id"]; got != nil {
		t.Fatalf("grant_skip must not carry correlation_id, got %v", got)
	}
}

// ─── D5（P0-5 phase 2）：auto 第四档行为钉板 ─────────────────────────────
// 语义铁律：auto = 等同 ask 消费（同 reason=policy_ask、同 grants 可满足、
// 零新 trigger/reason），区分仅在 verdict 投影 source_mode=auto。

// TestApprovalPolicyGate_AutoVerdict 钉死 gate switch 的 auto 分支：命中 auto
// 规则不短路、不放行，写 ctx verdict（mode=auto）——消费由确认门禁 decide()
// 完成（与 ask/trusted 同一下传机制）。
func TestApprovalPolicyGate_AutoVerdict(t *testing.T) {
	t.Parallel()
	hook, _ := newApprovalPolicyTestHook(t, nil, []biztool.ApprovalPolicy{
		approvalPolicyTestRule("r-auto", string(biztool.ApprovalModeAuto), false),
	}, nil)
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"rm -rf /a"}`), ToolCallID: "c-auto",
	})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res.CustomResult != nil {
		t.Fatalf("auto must not reject directly, got %v", res.CustomResult)
	}
	if v := approvalPolicyVerdictFromCtx(res.Context); v == nil || v.mode != biztool.ApprovalModeAuto || v.ruleID != "r-auto" {
		t.Fatalf("auto verdict missing, got %+v", v)
	}
}

// TestDecide_AutoConsumesLikeAsk 钉死 auto 与 ask 同消费路径：强制确认且
// reason=policy_ask（零新 reason），session/persisted grant 同语义可满足。
func TestDecide_AutoConsumesLikeAsk(t *testing.T) {
	t.Parallel()
	gate := &toolConfirmGate{sessionGrants: newToolGrantStore(time.Now)}
	d := gate.decide(approvalVerdictTestCtx(biztool.ApprovalModeAuto), "sess-1", "agent-1", "uncatalogued_tool", []byte(`{"x":"y"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyAsk {
		t.Fatalf("auto must force confirmation with reason=policy_ask (零新 reason), got %+v", d)
	}
	// session grant 可满足。
	gate.sessionGrants.GrantSession("sess-1", "agent-1", "uncatalogued_tool")
	d = gate.decide(approvalVerdictTestCtx(biztool.ApprovalModeAuto), "sess-1", "agent-1", "uncatalogued_tool", []byte(`{"x":"y"}`))
	if d.needsConfirm || d.reason != confirmReasonGrantSession {
		t.Fatalf("session grant must satisfy auto verdict, got %+v", d)
	}
	// persisted grant 可满足。
	gate2 := &toolConfirmGate{
		sessionGrants:  newToolGrantStore(time.Now),
		persistedGrant: func(context.Context, string, string) bool { return true },
	}
	d = gate2.decide(approvalVerdictTestCtx(biztool.ApprovalModeAuto), "sess-1", "agent-1", "uncatalogued_tool", []byte(`{"x":"y"}`))
	if d.needsConfirm || d.reason != confirmReasonGrantPersisted {
		t.Fatalf("persisted grant must satisfy auto verdict, got %+v", d)
	}
}

// TestDecide_AutoDoesNotSatisfyParamRuleAsk 钉死档位隔离：auto ≠ trusted——
// param rule ask 只能被 trusted 满足，auto 命中时仍强制确认（防 auto 被当
// 隐性提权档）。
func TestDecide_AutoDoesNotSatisfyParamRuleAsk(t *testing.T) {
	t.Parallel()
	gate := &toolConfirmGate{sessionGrants: newToolGrantStore(time.Now)}
	ctx := context.WithValue(approvalVerdictTestCtx(biztool.ApprovalModeAuto), paramRuleVerdictCtxKey{}, &paramRuleVerdict{
		effect: policyrule.EffectAsk, ruleID: "builtin-ask",
	})
	d := gate.decide(ctx, "sess-1", "agent-1", "gns3_exec", []byte(`{"command":"reload"}`))
	if !d.needsConfirm || d.reason != confirmReasonParamRuleAsk {
		t.Fatalf("auto must NOT satisfy param rule ask (only trusted may), got %+v", d)
	}
}

// TestToolConfirm_AutoSourceModeProjection 钉死 source_mode=auto 投影：auto
// verdict 驱动的确认流程，asked/decided 两半部 metadata 均携带
// source_mode=auto（auto_review_verdict 审计区分点），decision_reason 仍是
// policy_ask；ask verdict 对照组零投影（缺省即普通 HITL）。
func TestToolConfirm_AutoSourceModeProjection(t *testing.T) {
	t.Parallel()
	mk := func(mode biztool.ApprovalMode, confirmID string) (*captureDecisionCollector, *toolConfirmationBeforeHook, context.Context) {
		cc := &captureDecisionCollector{}
		deps := TRPCBuilderDeps{}
		deps.DecisionCollector = cc
		gate := &toolConfirmGate{sessionGrants: newToolGrantStore(time.Now)}
		h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)
		em := &fakeConfirmEmitter{confirmID: confirmID}
		ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
			return serviceawaitreply.ReplyApprove, nil
		}), em)
		ctx = context.WithValue(ctx, approvalPolicyVerdictCtxKey{}, &approvalPolicyVerdict{mode: mode, ruleID: "r-x"})
		return cc, h, ctx
	}
	// auto verdict → 两半部 source_mode=auto。
	cc, h, ctx := mk(biztool.ApprovalModeAuto, "act-auto-1")
	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "uncatalogued_tool", Arguments: []byte(`{"x":"y"}`), ToolCallID: "tc-auto",
	}); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 2 {
		t.Fatalf("expected asked+decided pair, got %d records", len(cc.recs))
	}
	for i := range cc.recs {
		if got := cc.recs[i].Metadata["source_mode"]; got != "auto" {
			t.Fatalf("rec[%d] source_mode = %v, want auto (outcome=%s)", i, got, cc.recs[i].Outcome)
		}
		if got := cc.recs[i].Metadata["decision_reason"]; got != confirmReasonPolicyAsk {
			t.Fatalf("rec[%d] decision_reason = %v, want %q", i, got, confirmReasonPolicyAsk)
		}
	}
	// ask verdict 对照 → 零投影。
	cc, h, ctx = mk(biztool.ApprovalModeAsk, "act-ask-1")
	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "uncatalogued_tool", Arguments: []byte(`{"x":"y"}`), ToolCallID: "tc-ask",
	}); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	for i := range cc.recs {
		if got := cc.recs[i].Metadata["source_mode"]; got != nil {
			t.Fatalf("rec[%d] ask path must not project source_mode, got %v", i, got)
		}
	}
}

// ─── D5-f（P0-5 phase 2 评审修复批）：17 确认项行为钉板 ───────────────────

// TestApprovalPolicyGate_ShadowAutoZeroEnforce D5-f F15：shadow+auto 同守
// 「零拒绝、必落 shadow 审计、零 verdict 写」——shadow 语义对四档一律纯观测
// （enforce switch 的 auto 分支只覆盖非 shadow 路径）。
func TestApprovalPolicyGate_ShadowAutoZeroEnforce(t *testing.T) {
	t.Parallel()
	cc := &captureDecisionCollector{}
	store := biztool.NewInMemApprovalPolicyStore()
	if err := store.UpsertApprovalPolicy(context.Background(), approvalPolicyTestRule("shadow-auto-1", string(biztool.ApprovalModeAuto), true)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	hook := newApprovalPolicyGateBeforeHook(biz.Agent{ID: "agent-1"}, deps,
		CompositeApprovalPolicyLookup(store, "sess-1", "agent-1", time.Now().Unix()))
	if hook == nil {
		t.Fatal("hook should be registered")
	}
	em := &fakeConfirmEmitter{}
	ctx := approvalPolicyCtxWithEmitter(context.Background(), em)
	res, err := hook.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"rm -rf /tmp/x"}`), ToolCallID: "call-sa",
	})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res.CustomResult != nil {
		t.Fatalf("shadow auto must not reject/enforce, got %v", res.CustomResult)
	}
	if v := approvalPolicyVerdictFromCtx(res.Context); v != nil {
		t.Fatalf("shadow must not write ctx verdict (D5-f F15), got %+v", v)
	}
	if len(cc.recs) != 1 || cc.recs[0].Outcome != "tripped" ||
		cc.recs[0].Metadata["trigger_rule"] != triggerApprovalPolicyShadow ||
		cc.recs[0].Metadata["shadow_only"] != true {
		t.Fatalf("shadow auto audit record wrong: %+v", cc.recs)
	}
}

// TestToolConfirm_AutoVerdict_BlocksPluginAllowSilentBypass 钉死 D5-f F1
// （major）：approval policy ask/auto 裁定与 param rule ask 同认——插件 allow
// 早退守卫不得绕过。若漏此守卫，auto 规则在插件 allow 路径被静默放行，
// decide() 根本不运行（auto 档空转）：零决策记录、零拒绝。无回复通道场景
// 下正确行为=进入确认流程被拒（no_reply_channel），且记录投射
// source_mode=auto。
func TestToolConfirm_AutoVerdict_BlocksPluginAllowSilentBypass(t *testing.T) {
	t.Parallel()
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	gate := &toolConfirmGate{
		sessionGrants: newToolGrantStore(time.Now),
		hasPlugin:     true,
		plugin: plugintrpc.ConfirmationGuardConfig{
			ConfirmTools:  []string{"db_migrate"},
			DefaultAction: "allow",
		},
	}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)
	em := &fakeConfirmEmitter{confirmID: "act-f1"}
	ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", nil), em)
	ctx = context.WithValue(ctx, approvalPolicyVerdictCtxKey{}, &approvalPolicyVerdict{mode: biztool.ApprovalModeAuto, ruleID: "r-auto"})

	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "db_migrate", Arguments: []byte(`{"q":"drop table t"}`), ToolCallID: "tc-f1",
	}); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("auto verdict must NOT silently pass the plugin allow path (F1): got %d records, want 1", len(cc.recs))
	}
	r := cc.recs[0]
	if r.Outcome != "rejected" || r.Metadata["block_cause"] != "no_reply_channel" {
		t.Fatalf("no-channel exit expected rejected/no_reply_channel, got outcome=%q block_cause=%v", r.Outcome, r.Metadata["block_cause"])
	}
	if got := r.Metadata["source_mode"]; got != "auto" {
		t.Fatalf("policy auto driven record must project source_mode=auto, got %v", got)
	}
	if got := r.Metadata["decision_reason"]; got != confirmReasonPolicyAsk {
		t.Fatalf("decision_reason = %v, want %q", got, confirmReasonPolicyAsk)
	}
}

// TestToolConfirm_ParamRuleAskDriven_NoAutoProjection 钉死 D5-f F2：param
// rule ask 驱动的记录绝不投影 source_mode=auto——裁决链 param 分支先于
// policy 分支，ctx 同时携带 auto verdict 也不表示 auto 驱动了本次流程。
func TestToolConfirm_ParamRuleAskDriven_NoAutoProjection(t *testing.T) {
	t.Parallel()
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	gate := &toolConfirmGate{sessionGrants: newToolGrantStore(time.Now)}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)
	em := &fakeConfirmEmitter{confirmID: "act-f2"}
	ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", nil), em)
	ctx = context.WithValue(ctx, paramRuleVerdictCtxKey{}, &paramRuleVerdict{effect: policyrule.EffectAsk, ruleID: "builtin-ask"})
	ctx = context.WithValue(ctx, approvalPolicyVerdictCtxKey{}, &approvalPolicyVerdict{mode: biztool.ApprovalModeAuto, ruleID: "r-auto"})

	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "gns3_exec", Arguments: []byte(`{"command":"reload"}`), ToolCallID: "tc-f2",
	}); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("got %d records, want 1 (no_reply_channel rejected)", len(cc.recs))
	}
	if got := cc.recs[0].Metadata["decision_reason"]; got != confirmReasonParamRuleAsk {
		t.Fatalf("decision_reason = %v, want %q", got, confirmReasonParamRuleAsk)
	}
	if got := cc.recs[0].Metadata["source_mode"]; got != nil {
		t.Fatalf("param_rule_ask driven record must not project source_mode (D5-f F2), got %v", got)
	}
}

// TestToolConfirm_ParamRuleAskDriven_AskedHalfNoAutoProjection 钉死 D5-f3
// （自复审 wf_d5613cd2-059 收口）：param rule ask 驱动 + 回复通道的正常
// 确认卡路径，asked/decided 双半部同样绝不投影 source_mode=auto——F2 钉
// 只走 no_reply_channel 半流程（无确认卡、无 asked 半部），本钉补齐带卡
// 路径的双半部级签一致性（单点半部误标 auto 时同流程两半部级签互相矛盾）。
func TestToolConfirm_ParamRuleAskDriven_AskedHalfNoAutoProjection(t *testing.T) {
	t.Parallel()
	cc := &captureDecisionCollector{}
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	gate := &toolConfirmGate{sessionGrants: newToolGrantStore(time.Now)}
	h := newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)
	em := &fakeConfirmEmitter{confirmID: "act-f2-card"}
	ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyApprove, nil
	}), em)
	ctx = context.WithValue(ctx, paramRuleVerdictCtxKey{}, &paramRuleVerdict{effect: policyrule.EffectAsk, ruleID: "builtin-ask"})
	// ctx 同时携带 auto verdict：param 分支先于 policy 分支，auto 未驱动。
	ctx = context.WithValue(ctx, approvalPolicyVerdictCtxKey{}, &approvalPolicyVerdict{mode: biztool.ApprovalModeAuto, ruleID: "r-auto"})

	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "gns3_exec", Arguments: []byte(`{"command":"reload"}`), ToolCallID: "tc-f2-card",
	}); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 2 {
		t.Fatalf("expected asked+decided pair via reply channel, got %d records", len(cc.recs))
	}
	for i := range cc.recs {
		if got := cc.recs[i].Metadata["decision_reason"]; got != confirmReasonParamRuleAsk {
			t.Fatalf("rec[%d] decision_reason = %v, want %q", i, got, confirmReasonParamRuleAsk)
		}
		if got := cc.recs[i].Metadata["source_mode"]; got != nil {
			t.Fatalf("rec[%d] param-rule-ask driven must not project source_mode (outcome=%s), got %v", i, cc.recs[i].Outcome, got)
		}
	}
}

// TestToolConfirm_GrantSkipSourceModeProjection 钉死 D5-f F13/F14 裁决：
// grant 秒过记录的 source_mode 投影取决于驱动分支——审批策略 ask/auto 分支
// 内 grant 满足（policyAskDriven=true）照常投影 source_mode=auto（auto 审计
// 区分点不因秒过丢失）；param rule ask 分支内 grant 满足不投影（param 分支
// 先于 policy 分支求值，auto 未驱动该流程）。
func TestToolConfirm_GrantSkipSourceModeProjection(t *testing.T) {
	t.Parallel()
	mk := func(withParamAsk bool) (*captureDecisionCollector, *toolConfirmationBeforeHook, context.Context) {
		cc := &captureDecisionCollector{}
		deps := TRPCBuilderDeps{}
		deps.DecisionCollector = cc
		gate := &toolConfirmGate{
			sessionGrants:  newToolGrantStore(time.Now),
			persistedGrant: func(context.Context, string, string) bool { return true },
		}
		em := &fakeConfirmEmitter{confirmID: "act-f14"}
		ctx := approvalPolicyCtxWithEmitter(decisionTestCtx("sess-1", "u-1", nil), em)
		if withParamAsk {
			ctx = context.WithValue(ctx, paramRuleVerdictCtxKey{}, &paramRuleVerdict{effect: policyrule.EffectAsk, ruleID: "builtin-ask"})
		}
		ctx = context.WithValue(ctx, approvalPolicyVerdictCtxKey{}, &approvalPolicyVerdict{mode: biztool.ApprovalModeAuto, ruleID: "r-x"})
		return cc, newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps), ctx
	}
	// A：auto verdict、无 param 规则——policy 分支 grant 秒过，投影 auto。
	cc, h, ctx := mk(false)
	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "uncatalogued_tool", Arguments: []byte(`{"x":"y"}`), ToolCallID: "tc-f14a",
	}); err != nil {
		t.Fatalf("HandleBeforeTool A: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("A: want 1 grant_skip record, got %d", len(cc.recs))
	}
	if cc.recs[0].Outcome != "grant_skip" || cc.recs[0].Metadata["decision_reason"] != confirmReasonGrantPersisted {
		t.Fatalf("A: grant_skip persisted expected, got %+v", cc.recs[0])
	}
	if got := cc.recs[0].Metadata["source_mode"]; got != "auto" {
		t.Fatalf("A: policy auto + grant_skip must project source_mode=auto (F13/F14), got %v", got)
	}
	// B：param ask + auto verdict——param 分支 grant 秒过，零投影（F2 grant 变体）。
	cc, h, ctx = mk(true)
	if _, err := h.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "gns3_exec", Arguments: []byte(`{"command":"reload"}`), ToolCallID: "tc-f14b",
	}); err != nil {
		t.Fatalf("HandleBeforeTool B: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("B: want 1 grant_skip record, got %d", len(cc.recs))
	}
	if cc.recs[0].Outcome != "grant_skip" || cc.recs[0].Metadata["decision_reason"] != confirmReasonGrantPersisted {
		t.Fatalf("B: grant_skip persisted expected, got %+v", cc.recs[0])
	}
	if got := cc.recs[0].Metadata["source_mode"]; got != nil {
		t.Fatalf("B: param branch grant_skip must not project source_mode (F2 grant variant), got %v", got)
	}
}
