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
