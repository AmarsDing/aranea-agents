package agent

import (
	"context"
	"testing"
)

// 75 M1.4 A5：敏感词目标强制逐次确认——即使持持久化/会话授权，
// computer_use_act/launch 命中高危词时也必须弹出确认卡。
func TestToolConfirmGate_Decide_ComputerUseDangerBypassesGrants(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"computer_use_act":    {requiresConfirm: true},
		"computer_use_launch": {requiresConfirm: true},
	}, func(context.Context, string, string) bool { return true }) // 持持久化授权
	g.sessionGrants.GrantSession("sess-1", "agent-1", "computer_use_act")

	// 非敏感目标：持久化授权短路免确认（A5 后半句）。
	d := g.decide(context.Background(), "sess-1", "agent-1", "computer_use_act",
		[]byte(`{"target":"保存菜单项","action":"invoke"}`))
	if d.needsConfirm || d.reason != confirmReasonGrantPersisted {
		t.Fatalf("non-danger decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantPersisted)
	}

	// 敏感词目标（删除）：强制逐次确认，授权不生效。
	d = g.decide(context.Background(), "sess-1", "agent-1", "computer_use_act",
		[]byte(`{"target":"永久删除按钮","action":"click"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyDanger {
		t.Fatalf("danger decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonPolicyDanger)
	}

	// 敏感词藏在 text 参数（转账内容）：同样强制确认。
	d = g.decide(context.Background(), "sess-1", "agent-1", "computer_use_act",
		[]byte(`{"target":"输入框","action":"type","text":"确认转账 100 元"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyDanger {
		t.Fatalf("danger text decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonPolicyDanger)
	}

	// launch 敏感目标（shutdown）：强制确认。
	d = g.decide(context.Background(), "sess-1", "agent-1", "computer_use_launch",
		[]byte(`{"target":"shutdown.exe"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyDanger {
		t.Fatalf("danger launch decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonPolicyDanger)
	}
}

// 非 computer_use 工具不受内容强制确认影响（grant 行为不变）。
func TestToolConfirmGate_Decide_ComputerUseDangerDoesNotAffectOtherTools(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"shell_exec": {requiresConfirm: true},
	}, func(context.Context, string, string) bool { return true })
	d := g.decide(context.Background(), "sess-1", "agent-1", "shell_exec",
		[]byte(`{"command":"删除临时文件"}`))
	if d.needsConfirm || d.reason != confirmReasonGrantPersisted {
		t.Fatalf("decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantPersisted)
	}
}

// 批量 actions[] 子步敏感词同样强制确认（持久授权不可豁免）。
func TestToolConfirmGate_Decide_ComputerUseBatchDangerBypassesGrants(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"computer_use_act": {requiresConfirm: true},
	}, func(context.Context, string, string) bool { return true })

	d := g.decide(context.Background(), "sess-1", "agent-1", "computer_use_act",
		[]byte(`{"actions":[{"target":"删除按钮","action":"invoke"}]}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyDanger {
		t.Fatalf("batch danger decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonPolicyDanger)
	}

	d = g.decide(context.Background(), "sess-1", "agent-1", "computer_use_act",
		[]byte(`{"actions":[{"target":"保存","action":"invoke"}]}`))
	if d.needsConfirm || d.reason != confirmReasonGrantPersisted {
		t.Fatalf("batch non-danger decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantPersisted)
	}
}

// M77 B3：Observe 注入打标后，无敏感词的 act 仍强制逐次确认。
func TestToolConfirmGate_Decide_InjectionSuspectedBypassesGrants(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"computer_use_act": {requiresConfirm: true},
	}, func(context.Context, string, string) bool { return true })
	g.agentKey = "notepad-agent"
	g.injectionSuspected = func(key string) bool { return key == "notepad-agent" }

	d := g.decide(context.Background(), "sess-1", "agent-uuid", "computer_use_act",
		[]byte(`{"target":"保存菜单项","action":"invoke"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyDanger {
		t.Fatalf("injection decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonPolicyDanger)
	}

	g.injectionSuspected = func(string) bool { return false }
	d = g.decide(context.Background(), "sess-1", "agent-uuid", "computer_use_act",
		[]byte(`{"target":"保存菜单项","action":"invoke"}`))
	if d.needsConfirm || d.reason != confirmReasonGrantPersisted {
		t.Fatalf("clean injection decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantPersisted)
	}
}
