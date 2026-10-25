package agent

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	biztool "aranea-agents/internal/biz/tool"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// toolUCWithApprovalReader 在 gateToolLookup 上附加审批策略读口，模拟
// 批 C 装配后 *biztool.ToolUsecase 满足 biztool.ApprovalPolicyReader
// 的形态（deps.ToolUC 可选接口断言命中分支）。
type toolUCWithApprovalReader struct {
	gateToolLookup
	reader biztool.ApprovalPolicyReader
}

func (f *toolUCWithApprovalReader) ListEnabledApprovalPolicies(ctx context.Context, scope, scopeKey string, nowUnix int64) ([]biztool.ApprovalPolicy, error) {
	return f.reader.ListEnabledApprovalPolicies(ctx, scope, scopeKey, nowUnix)
}

// TestApprovalPolicyLookupForDeps_NilToolUC 未装配 ToolUC → lookup 为 nil，
// gate 不注册（OFF 字节级兜底钉死）。
func TestApprovalPolicyLookupForDeps_NilToolUC(t *testing.T) {
	t.Parallel()
	if lookup := approvalPolicyLookupForDeps(biz.Agent{ID: "a"}, TRPCBuilderDeps{}); lookup != nil {
		t.Fatal("nil ToolUC must produce nil lookup")
	}
}

// TestApprovalPolicyLookupForDeps_ToolUCWithoutReader ToolUC 未实现读口
// （未装配 store 的旧 usecase 形态）→ lookup 为 nil。
func TestApprovalPolicyLookupForDeps_ToolUCWithoutReader(t *testing.T) {
	t.Parallel()
	deps := TRPCBuilderDeps{TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{ToolUC: &gateToolLookup{}}}
	if lookup := approvalPolicyLookupForDeps(biz.Agent{ID: "a"}, deps); lookup != nil {
		t.Fatal("ToolUC without ApprovalPolicyReader must produce nil lookup")
	}
}

// TestApprovalPolicyLookupForDeps_SessionScopedMerge 批 C activation 端到端：
// WithGateSessionID 注入会话后，session 与 agent 双作用域规则合并可见；
// 未注入时仅 agent 作用域生效（gateSessionID 回落空串）。
func TestApprovalPolicyLookupForDeps_SessionScopedMerge(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	store := biztool.NewInMemApprovalPolicyStore()
	sessRule := approvalPolicyTestRule("ap-sess-never", string(biztool.ApprovalModeNever), false)
	sessRule.ScopeKey = "sess-9"
	agentRule := approvalPolicyTestRule("ap-agent-ask", string(biztool.ApprovalModeAsk), false)
	agentRule.Scope = string(biztool.ApprovalScopeAgent)
	agentRule.ScopeKey = "agent-1"
	if err := store.UpsertApprovalPolicy(context.Background(), sessRule); err != nil {
		t.Fatalf("seed session rule: %v", err)
	}
	if err := store.UpsertApprovalPolicy(context.Background(), agentRule); err != nil {
		t.Fatalf("seed agent rule: %v", err)
	}
	deps := TRPCBuilderDeps{TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{
		ToolUC: &toolUCWithApprovalReader{reader: store},
	}}
	ag := biz.Agent{ID: "agent-1"}
	lookup := approvalPolicyLookupForDeps(ag, deps)
	if lookup == nil {
		t.Fatal("reader-equipped ToolUC must produce lookup")
	}

	// 无会话注入：仅 agent 作用域。
	rows, err := lookup(context.Background())
	if err != nil || len(rows) != 1 || rows[0].ID != agentRule.ID {
		t.Fatalf("agent-scope-only merge = (%v,%v)", len(rows), err)
	}
	// WithGateSessionID 注入：双作用域合并。
	ctx := decision.WithGateSessionID(context.Background(), "sess-9")
	rows, err = lookup(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("dual-scope merge = (%v,%v)", len(rows), err)
	}
	if now < 0 {
		t.Fatal("unreachable")
	}
}

// TestApprovalPolicyGate_ActivatedViaDeps 装配路径触发 never 确定性拒绝：
// gate 读口从 deps.ToolUC 派生（非测试直灌），确认 callback_chain 接线成立。
func TestApprovalPolicyGate_ActivatedViaDeps(t *testing.T) {
	t.Parallel()
	store := biztool.NewInMemApprovalPolicyStore()
	rule := approvalPolicyTestRule("ap-deps-never", string(biztool.ApprovalModeNever), false)
	rule.Scope = string(biztool.ApprovalScopeAgent)
	rule.ScopeKey = "agent-1"
	rule.ToolPattern = "*"
	rule.CommandPrefix = "rm -rf*"
	if err := store.UpsertApprovalPolicy(context.Background(), rule); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
	deps := TRPCBuilderDeps{TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{
		ToolUC: &toolUCWithApprovalReader{gateToolLookup: gateToolLookup{writes: make(chan biztool.ToolInvocationWrite, 1)}, reader: store},
	}}
	hook := newApprovalPolicyGateBeforeHook(biz.Agent{ID: "agent-1"}, deps, approvalPolicyLookupForDeps(biz.Agent{ID: "agent-1"}, deps))
	if hook == nil {
		t.Fatal("hook must register when reader equipped")
	}
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"rm -rf /tmp/y"}`), ToolCallID: "call-d1",
	})
	if err != nil {
		t.Fatalf("gate handle: %v", err)
	}
	if res == nil || res.CustomResult == nil {
		t.Fatal("never rule via deps path must deterministically reject")
	}
}

// TestApprovalPolicyGate_ChainActivation 对抗评审 #10：不直灌 hook，而是经
// productCallbackChain 全链装配（deps.ToolUC 可选断言 → lookup → 注册闸
// 在 paramRuleGate 之后、args 守卫之前），never 命中须沿链判定为
// CustomResult 拒绝——callback_chain.go 注册块的行为钉死。
func TestApprovalPolicyGate_ChainActivation(t *testing.T) {
	t.Parallel()
	store := biztool.NewInMemApprovalPolicyStore()
	rule := approvalPolicyTestRule("ap-chain-never", string(biztool.ApprovalModeNever), false)
	rule.Scope = string(biztool.ApprovalScopeAgent)
	rule.ScopeKey = "agent-1"
	rule.ToolPattern = "*"
	rule.CommandPrefix = "rm -rf*"
	if err := store.UpsertApprovalPolicy(context.Background(), rule); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
	deps := TRPCBuilderDeps{TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{
		ToolUC: &toolUCWithApprovalReader{gateToolLookup: gateToolLookup{writes: make(chan biztool.ToolInvocationWrite, 1)}, reader: store},
	}}
	ag := biz.Agent{ID: "agent-1", AgentKey: "test", Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true}}
	chain := productCallbackChain(context.Background(), ag, deps, nil)
	if chain == nil || !chain.HasToolHooks() {
		t.Fatal("chain must register tool hooks")
	}
	res, err := chain.AdaptToolCallbacks().RunBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"rm -rf /tmp/y"}`), ToolCallID: "call-chain-1",
	})
	if err != nil {
		t.Fatalf("chain run before tool: %v", err)
	}
	if res == nil || res.CustomResult == nil {
		t.Fatal("never rule must deterministically reject via chain-level gate")
	}
	// 同链同时验证 paramRuleGate 未误命中（无规则时不得拒绝合法工具）。
	allow, err := chain.AdaptToolCallbacks().RunBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName: "exec_command", Arguments: []byte(`{"command":"ls -la /tmp"}`), ToolCallID: "call-chain-2",
	})
	if err != nil {
		t.Fatalf("chain allow path: %v", err)
	}
	if allow != nil && allow.CustomResult != nil {
		t.Fatal("non-matching command must not be rejected")
	}
}
