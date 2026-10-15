package biz

import (
	bizsession "aranea-agents/internal/biz/session"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/workspace"
	"context"
	"testing"
)

func TestSessionSearch_ReadSessionHistory_WorkspaceGuard(t *testing.T) {
	msgs := []bizsession.ChatMessage{{ID: "m1", Role: "user", ContentMarkdown: "x"}}
	uc := newM71SessionSearch(SpiritAgentKey, &m71Auditor{}, msgs)
	reader := &m71SessionReader{sess: bizsession.Session{ID: "s1", WorkspaceID: "ws-a"}}
	uc.sessions = reader

	ctx := workspace.WithContext(context.Background(), "ws-a")
	if _, _, err := uc.ReadSessionHistory(ctx, "spirit-1", "s1", "", 0, 0); err != nil {
		t.Fatalf("expected allow in same workspace: %v", err)
	}

	ctxOther := workspace.WithContext(context.Background(), "ws-b")
	if _, _, err := uc.ReadSessionHistory(ctxOther, "spirit-1", "s1", "", 0, 0); err == nil {
		t.Fatal("expected denial for cross-workspace read")
	}

	reader.sessErr = shared.ErrNotFound
	if _, _, err := uc.ReadSessionHistory(ctx, "spirit-1", "s1", "", 0, 0); err == nil {
		t.Fatal("expected not found for missing session")
	}
}

func TestIsDeptLeadAgent(t *testing.T) {
	cases := []struct {
		name  string
		agent Agent
		want  bool
	}{
		{"variant match", Agent{AgentVariant: "dept_lead"}, true},
		{"key prefix+suffix", Agent{AgentKey: "__dept_lead_sales__"}, true},
		{"prefix only no suffix", Agent{AgentKey: "__dept_lead_sales"}, false},
		{"regular agent", Agent{AgentKey: "worker_a"}, false},
		{"company lead is not dept", Agent{AgentVariant: AgentVariantCompanyLead}, false},
		{"empty", Agent{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsDeptLeadAgent(c.agent); got != c.want {
				t.Fatalf("IsDeptLeadAgent = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNormalizeRefsJSON(t *testing.T) {
	if got, err := normalizeRefsJSON(""); err != nil || got != "[]" {
		t.Fatalf("empty refs: got=%q err=%v", got, err)
	}
	if _, err := normalizeRefsJSON(`{"a":1}`); err == nil {
		t.Fatal("expected error for non-array refs")
	}
	if got, err := normalizeRefsJSON(`[{"team_id":"t1"}]`); err != nil || got != `[{"team_id":"t1"}]` {
		t.Fatalf("valid refs: got=%q err=%v", got, err)
	}
}

func TestResourceAccess_PositionShortNameAlias(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	member := Agent{
		ID:          "ppc-1",
		AgentKey:    "ppc_strategist__general",
		PositionID:  m71PosA1,
		PositionKey: "digital_content_media/paid_promotion/ppc_strategist",
	}
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, member.ID: member},
		byKey: map[string]Agent{member.AgentKey: member},
	}
	uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{}, &m71Auditor{})
	_, err := uc.ListMemberFiles(context.Background(), leadA.ID, "ppc_strategist", "", 0)
	if err != nil {
		t.Fatalf("short name ppc_strategist should resolve in-department: %v", err)
	}
}

// 同部门内多个 agent 命中同一短名时必须 fail-closed（denied），不允许
// 「随便挑一个」放行——resolveMemberTarget 唯一命中才解析。
func TestResourceAccess_AliasAmbiguousFailClosed(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	m1 := Agent{ID: "ppc-1", AgentKey: "ppc_strategist__general", PositionID: m71PosA1}
	m2 := Agent{ID: "ppc-2", AgentKey: "ppc_strategist__senior", PositionID: m71PosA1}
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, m1.ID: m1, m2.ID: m2},
		byKey: map[string]Agent{m1.AgentKey: m1, m2.AgentKey: m2},
	}
	auditor := &m71Auditor{}
	uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{}, auditor)
	_, err := uc.ListMemberFiles(context.Background(), leadA.ID, "ppc_strategist", "", 0)
	if err == nil {
		t.Fatal("ambiguous alias must be denied (fail-closed)")
	}
	if auditor.countByResult(ResultDenied) == 0 {
		t.Fatalf("expected denied audit entry, got %+v", auditor.entries)
	}
}

// 唯一 alias 命中在别的部门时不得放行（alias 解析只在 caller 部门内生效）。
func TestResourceAccess_AliasCrossDeptDenied(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	memberB := Agent{ID: "ppc-b", AgentKey: "ppc_strategist__general", PositionID: m71PosB1}
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, memberB.ID: memberB},
		byKey: map[string]Agent{memberB.AgentKey: memberB},
	}
	auditor := &m71Auditor{}
	uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{}, auditor)
	_, err := uc.ListMemberFiles(context.Background(), leadA.ID, "ppc_strategist", "", 0)
	if err == nil {
		t.Fatal("cross-department alias hit must be denied")
	}
	if auditor.countByResult(ResultDenied) == 0 {
		t.Fatalf("expected denied audit entry, got %+v", auditor.entries)
	}
}

// 钉住 mock 对齐后的生产语义（F13）：仅 PositionKey 短名命中而 AgentKey
// 不含关键词时，生产 SearchAgents 的 keyword 字段集（AgentKey/DisplayName/
// Provider/Model/AgentDescription）不会返回该 agent → 解析为 not found。
// 若未来生产把 PositionKey 纳入搜索字段，本测试应同步反转为正向用例。
func TestResourceAccess_PositionKeyOnlyAliasNotSearchable(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	member := Agent{
		ID:          "ppc-1",
		AgentKey:    "paid_ads_specialist__general",
		PositionID:  m71PosA1,
		PositionKey: "digital_content_media/paid_promotion/ppc_strategist",
	}
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, member.ID: member},
		byKey: map[string]Agent{member.AgentKey: member},
	}
	auditor := &m71Auditor{}
	uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{}, auditor)
	_, err := uc.ListMemberFiles(context.Background(), leadA.ID, "ppc_strategist", "", 0)
	if err == nil {
		t.Fatal("position-key-only alias is not searchable in production; must deny")
	}
	if auditor.countByResult(ResultDenied) == 0 {
		t.Fatalf("expected denied audit entry, got %+v", auditor.entries)
	}
}
