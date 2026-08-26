package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/policyrule"
	biztool "aranea-agents/internal/biz/tool"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// gateToolLookup 实现 biz.TeamToolLookup：内嵌 tool_build_catalog_test.go 的
// fakeToolLookup 复用全部存量方法，仅覆写参数规则读口并捕获查询键。
type gateToolLookup struct {
	fakeToolLookup
	rules   []biztool.ToolParamRule
	err     error
	gotKeys []string
}

func (f *gateToolLookup) ListEnabledParamRulesForGate(_ context.Context, key string) ([]biztool.ToolParamRule, error) {
	f.gotKeys = append(f.gotKeys, key)
	return f.rules, f.err
}

func newParamRuleGateTestHook(t *testing.T, lookup biz.TeamToolLookup) (interface {
	HandleBeforeTool(context.Context, *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error)
}, *gateToolLookup) {
	t.Helper()
	gl, _ := lookup.(*gateToolLookup)
	deps := TRPCBuilderDeps{}
	deps.ToolUC = lookup
	hook := newParamRuleGateBeforeHook(biz.Agent{ID: "agent-1"}, deps)
	if hook == nil {
		t.Fatal("hook should be registered when ToolUC present")
	}
	return hook, gl
}

func callParamRuleGate(hook interface {
	HandleBeforeTool(context.Context, *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error)
}, tool, args string) *trpctool.BeforeToolResult {
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName: tool, Arguments: []byte(args), ToolCallID: "call-1",
	})
	if err != nil {
		panic(err)
	}
	return res
}

func TestParamRuleGate_NilToolUCNotRegistered(t *testing.T) {
	t.Parallel()
	if hook := newParamRuleGateBeforeHook(biz.Agent{ID: "a"}, TRPCBuilderDeps{}); hook != nil {
		t.Fatal("nil ToolUC should skip registration")
	}
}

func TestParamRuleGate_DenyRejects(t *testing.T) {
	t.Parallel()
	hook, _ := newParamRuleGateTestHook(t, &gateToolLookup{rules: []biztool.ToolParamRule{
		{ID: "builtin-exec-deny-rmrf", Pattern: "rm -rf*", Effect: "deny", Priority: 10, Enabled: true},
	}})
	res := callParamRuleGate(hook, "exec_command", `{"command":"rm -rf /tmp/x"}`)
	if res.CustomResult == nil {
		t.Fatal("deny should short-circuit with CustomResult")
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "builtin-exec-deny-rmrf") {
		t.Fatalf("deny message should name the rule, got %q", msg)
	}
	if v := paramRuleVerdictFromCtx(res.Context); v != nil {
		t.Fatalf("deny must not write ctx verdict, got %+v", v)
	}
}

func TestParamRuleGate_AskWritesVerdict(t *testing.T) {
	t.Parallel()
	hook, _ := newParamRuleGateTestHook(t, &gateToolLookup{rules: []biztool.ToolParamRule{
		{ID: "builtin-gns3-fallback-ask", Pattern: "*", Effect: "ask", Priority: 900, Enabled: true},
	}})
	res := callParamRuleGate(hook, "gns3_exec", `{"command":"reload"}`)
	if res.CustomResult != nil {
		t.Fatalf("ask must not short-circuit, got %v", res.CustomResult)
	}
	v := paramRuleVerdictFromCtx(res.Context)
	if v == nil || v.effect != policyrule.EffectAsk || v.ruleID != "builtin-gns3-fallback-ask" {
		t.Fatalf("verdict = %+v, want ask/builtin-gns3-fallback-ask", v)
	}
}

func TestParamRuleGate_AllowWritesVerdict(t *testing.T) {
	t.Parallel()
	hook, _ := newParamRuleGateTestHook(t, &gateToolLookup{rules: []biztool.ToolParamRule{
		{ID: "builtin-gns3-allow-show", Pattern: "show *", Effect: "allow", Priority: 10, Enabled: true},
	}})
	res := callParamRuleGate(hook, "gns3_exec", `{"command":"show ip interface brief"}`)
	v := paramRuleVerdictFromCtx(res.Context)
	if v == nil || v.effect != policyrule.EffectAllow {
		t.Fatalf("verdict = %+v, want allow", v)
	}
}

func TestParamRuleGate_DenyBeatsAllow(t *testing.T) {
	t.Parallel()
	// 同文本同时命中 allow 与 deny：deny 生效（effectRank 优先于 priority 数值）。
	hook, _ := newParamRuleGateTestHook(t, &gateToolLookup{rules: []biztool.ToolParamRule{
		{ID: "z-allow-all", Pattern: "*", Effect: "allow", Priority: 1, Enabled: true},
		{ID: "a-deny-rmrf", Pattern: "rm -rf*", Effect: "deny", Priority: 900, Enabled: true},
	}})
	res := callParamRuleGate(hook, "exec_command", `{"command":"rm -rf /"}`)
	if res.CustomResult == nil {
		t.Fatal("deny must win over allow regardless of priority numbers")
	}
}

func TestParamRuleGate_NoMatchPasses(t *testing.T) {
	t.Parallel()
	hook, _ := newParamRuleGateTestHook(t, &gateToolLookup{rules: []biztool.ToolParamRule{
		{ID: "r1", Pattern: "show *", Effect: "allow", Enabled: true},
	}})
	res := callParamRuleGate(hook, "gns3_exec", `{"command":"ping 8.8.8.8"}`)
	if res.CustomResult != nil || paramRuleVerdictFromCtx(res.Context) != nil {
		t.Fatal("no match should pass clean (fallback)")
	}
}

func TestParamRuleGate_AliasAndToolSetPrefixCanonicalized(t *testing.T) {
	t.Parallel()
	hook, gl := newParamRuleGateTestHook(t, &gateToolLookup{})
	callParamRuleGate(hook, "shell", `{"command":"ls"}`)
	callParamRuleGate(hook, "hostexec_exec_command", `{"command":"ls"}`)
	callParamRuleGate(hook, "exec_command", `{"command":"ls"}`)
	want := []string{"exec_command", "exec_command", "exec_command"}
	if len(gl.gotKeys) != 3 {
		t.Fatalf("gotKeys = %v, want %v", gl.gotKeys, want)
	}
	for i, k := range gl.gotKeys {
		if k != want[i] {
			t.Fatalf("gotKeys[%d] = %q, want %q (all %v)", i, k, want[i], gl.gotKeys)
		}
	}
}

func TestParamRuleGate_ListErrorFailsOpen(t *testing.T) {
	t.Parallel()
	hook, _ := newParamRuleGateTestHook(t, &gateToolLookup{err: errors.New("db down")})
	res := callParamRuleGate(hook, "gns3_exec", `{"command":"reload"}`)
	if res.CustomResult != nil || paramRuleVerdictFromCtx(res.Context) != nil {
		t.Fatal("list error should fail-open")
	}
}

func TestParamRuleGate_DisabledRuleIgnored(t *testing.T) {
	t.Parallel()
	// store 读口只回 enabled 行；此处模拟 store 误回 disabled 行时 gate 仍跳过
	//（policyrule.Evaluate 的 Enabled 检查兜底）。
	hook, _ := newParamRuleGateTestHook(t, &gateToolLookup{rules: []biztool.ToolParamRule{
		{ID: "r1", Pattern: "*", Effect: "deny", Enabled: false},
	}})
	res := callParamRuleGate(hook, "exec_command", `{"command":"rm -rf /"}`)
	if res.CustomResult != nil {
		t.Fatal("disabled rule must not deny")
	}
}

func TestParamRuleMatchText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args string
		want string
	}{
		{"单命令", `{"command":"show ip br"}`, "show ip br"},
		{"键序确定", `{"b":"2","a":"1"}`, "1 2"},
		{"嵌套数组", `{"cmd":["sh","-c","ls -la"]}`, "sh -c ls -la"},
		{"非字符串忽略", `{"command":"ls","timeout":30,"flag":true}`, "ls"},
		{"非 JSON 原样", `not-json`, "not-json"},
		{"空", ``, ""},
	}
	for _, tt := range cases {
		if got := paramRuleMatchText([]byte(tt.args)); got != tt.want {
			t.Errorf("%s: paramRuleMatchText = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// --- 确认门禁集成（ctx 裁定消费，tool_confirm_gate.go decide） ---

func paramRuleCtx(effect policyrule.Effect) context.Context {
	return context.WithValue(context.Background(), paramRuleVerdictCtxKey{},
		&paramRuleVerdict{effect: effect, ruleID: "r-test", pattern: "*"})
}

func TestToolConfirmGate_ParamRuleAskForcesConfirm(t *testing.T) {
	t.Parallel()
	// 目录未标确认的只读工具，ask 裁定仍强制确认。
	g := newTestGate(nil, nil)
	d := g.decide(paramRuleCtx(policyrule.EffectAsk), "sess-1", "agent-1", "file_read", nil)
	if !d.needsConfirm || d.reason != confirmReasonParamRuleAsk {
		t.Fatalf("decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonParamRuleAsk)
	}
}

func TestToolConfirmGate_ParamRuleAllowSkipsCatalog(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{"exec_command": {requiresConfirm: true}}, nil)
	d := g.decide(paramRuleCtx(policyrule.EffectAllow), "sess-1", "agent-1", "exec_command", []byte(`{"command":"ls"}`))
	if d.needsConfirm || d.reason != confirmReasonParamRuleAllow {
		t.Fatalf("decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonParamRuleAllow)
	}
}

func TestToolConfirmGate_ParamRuleAskSatisfiedByGrants(t *testing.T) {
	t.Parallel()
	// persisted grant 满足 ask。
	g := newTestGate(nil, func(context.Context, string, string) bool { return true })
	d := g.decide(paramRuleCtx(policyrule.EffectAsk), "sess-1", "agent-1", "gns3_exec", nil)
	if d.needsConfirm || d.reason != confirmReasonGrantPersisted {
		t.Fatalf("persisted decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantPersisted)
	}
	// session grant 满足 ask。
	g2 := newTestGate(nil, nil)
	g2.sessionGrants.GrantSession("sess-1", "agent-1", "gns3_exec")
	d = g2.decide(paramRuleCtx(policyrule.EffectAsk), "sess-1", "agent-1", "gns3_exec", nil)
	if d.needsConfirm || d.reason != confirmReasonGrantSession {
		t.Fatalf("session decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantSession)
	}
}

func TestToolConfirmGate_ParamRuleAllowNeverBypassesDangerFloor(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{"computer_use_act": {requiresConfirm: true}}, nil)
	d := g.decide(paramRuleCtx(policyrule.EffectAllow), "sess-1", "agent-1", "computer_use_act",
		[]byte(`{"target":"永久删除按钮","action":"click"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyDanger {
		t.Fatalf("decide = (%v,%q), want (true,%q) — allow 不得跳过 danger floor", d.needsConfirm, d.reason, confirmReasonPolicyDanger)
	}
}
