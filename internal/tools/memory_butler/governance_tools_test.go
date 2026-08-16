package memory_butler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ── M4 补丁：治理提案人工二审工具 ─────────────────────────────────────────
// 契约：Knowledge 未接线显式报错；proposals 透传 biz 列表并投影 resolved 标志；
// resolve 校验 proposal_id/decision 后透传 biz 闭环。

type stubGovernanceCurateRepo struct {
	bizknowledge.KnowledgeCurateRepo // 嵌入 nil 接口，仅覆盖用到的两个方法
	views                            []bizknowledge.GovernanceProposalView
	resolvedID                       int64
	resolvedStatus                   string
}

func (s *stubGovernanceCurateRepo) ListGovernanceProposals(context.Context, string, string, int) ([]bizknowledge.GovernanceProposalView, error) {
	return s.views, nil
}

func (s *stubGovernanceCurateRepo) ResolveGovernanceProposal(_ context.Context, id int64, status string) error {
	s.resolvedID, s.resolvedStatus = id, status
	return nil
}

func wiredGovernanceDeps(repo bizknowledge.KnowledgeCurateRepo) Deps {
	u := bizknowledge.NewUsecase(nil, nil, nil)
	u.SetCurateRepo(repo)
	return Deps{Knowledge: u}
}

func TestGovernanceProposals_NotWired(t *testing.T) {
	tl := newGovernanceProposalsTool(Deps{}).(trpctool.CallableTool)
	if _, err := tl.Call(context.Background(), []byte(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "not wired") {
		t.Fatalf("unwired must error, got %v", err)
	}
}

func TestGovernanceProposals_List(t *testing.T) {
	stub := &stubGovernanceCurateRepo{views: []bizknowledge.GovernanceProposalView{
		{ID: 7, CollectionID: "c1", Kind: bizknowledge.ProposalKindConflict, Risk: bizknowledge.ProposalRiskHigh,
			Status: bizknowledge.ProposalStatusPending, Payload: map[string]any{"dedup_key": "conflict:a→b"},
			CreatedAt: time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)},
		{ID: 3, CollectionID: "c1", Kind: bizknowledge.ProposalKindStale, Risk: bizknowledge.ProposalRiskLow,
			Status: bizknowledge.ProposalStatusApplied, Payload: map[string]any{"dedup_key": "stale:d2"},
			CreatedAt: time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC),
			// ResolvedAt 非零 → resolved=true
			ResolvedAt: time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC)},
	}}
	tl := newGovernanceProposalsTool(wiredGovernanceDeps(stub)).(trpctool.CallableTool)
	raw, err := tl.Call(context.Background(), []byte(`{"status":"pending"}`))
	if err != nil {
		t.Fatal(err)
	}
	out, ok := raw.(governanceProposalsOutput)
	if !ok {
		b, _ := json.Marshal(raw)
		t.Fatalf("unexpected output type %T: %s", raw, b)
	}
	if out.Total != 2 || len(out.Proposals) != 2 {
		t.Fatalf("out = %+v", out)
	}
	if out.Proposals[0].ID != 7 || out.Proposals[0].Resolved {
		t.Fatalf("pending proposal投影错误: %+v", out.Proposals[0])
	}
	if !out.Proposals[1].Resolved {
		t.Fatalf("resolved proposal投影错误: %+v", out.Proposals[1])
	}
}

func TestGovernanceResolve_Validation(t *testing.T) {
	tl := newGovernanceResolveTool(wiredGovernanceDeps(&stubGovernanceCurateRepo{})).(trpctool.CallableTool)
	for _, args := range []string{
		`{"proposal_id":0,"decision":"applied"}`,
		`{"proposal_id":-1,"decision":"applied"}`,
		`{"proposal_id":1,"decision":"bogus"}`,
		`{"proposal_id":1,"decision":""}`,
	} {
		if _, err := tl.Call(context.Background(), []byte(args)); err == nil {
			t.Fatalf("%s must error", args)
		}
	}
	// 未接线也报错。
	if _, err := newGovernanceResolveTool(Deps{}).(trpctool.CallableTool).
		Call(context.Background(), []byte(`{"proposal_id":1,"decision":"applied"}`)); err == nil {
		t.Fatal("unwired must error")
	}
}

func TestGovernanceResolve_Resolve(t *testing.T) {
	stub := &stubGovernanceCurateRepo{}
	tl := newGovernanceResolveTool(wiredGovernanceDeps(stub)).(trpctool.CallableTool)
	raw, err := tl.Call(context.Background(), []byte(`{"proposal_id":42,"decision":"REJECTED"}`))
	if err != nil {
		t.Fatal(err)
	}
	out := raw.(governanceResolveOutput)
	if !out.Resolved || out.ProposalID != 42 || out.Decision != bizknowledge.ProposalStatusRejected {
		t.Fatalf("out = %+v", out)
	}
	if stub.resolvedID != 42 || stub.resolvedStatus != bizknowledge.ProposalStatusRejected {
		t.Fatalf("stub = %d/%s", stub.resolvedID, stub.resolvedStatus)
	}
}

func TestRegisterAll_GovernanceToolsMounted(t *testing.T) {
	// RegisterAll 对 Analytics/MemoryAdmin/Agents 仅做非 nil 检查（不调方法），
	// 零值指针与最小 stub 即可满足。
	baseDeps := Deps{
		Analytics:   &biz.ExperienceAnalyticsUsecase{},
		MemoryAdmin: &biz.MemoryAdminUsecase{},
		Agents:      stubAgentsRuntimeRepo{},
	}
	// Knowledge 未接线：7 个基础工具，无治理工具。
	base := RegisterAll(baseDeps)
	if got := len(base); got != 7 {
		names := make([]string, 0, got)
		for _, tl := range base {
			names = append(names, tl.Declaration().Name)
		}
		t.Fatalf("unwired tools = %v", names)
	}
	// Knowledge 接线：追加 curate + proposals + resolve 三个工具。
	baseDeps.Knowledge = wiredGovernanceDeps(&stubGovernanceCurateRepo{}).Knowledge
	full := RegisterAll(baseDeps)
	if got := len(full); got != 10 {
		t.Fatalf("wired tools count = %d, want 10", got)
	}
	seen := map[string]bool{}
	for _, tl := range full {
		seen[tl.Declaration().Name] = true
	}
	for _, want := range []string{
		"memory_butler_knowledge_curate",
		"memory_butler_governance_proposals",
		"memory_butler_governance_resolve",
	} {
		if !seen[want] {
			t.Fatalf("missing tool %s in %v", want, seen)
		}
	}
}

// stubAgentsRuntimeRepo 最小 AgentRuntimeSettingsRepo 实现（仅满足非 nil 检查）。
type stubAgentsRuntimeRepo struct{}

func (stubAgentsRuntimeRepo) GetAgentRuntimeSettings(context.Context, string) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, nil
}

func (stubAgentsRuntimeRepo) ListAgentRuntimeSettings(context.Context) (map[string]biz.AgentRuntimeSettings, error) {
	return nil, nil
}

func (stubAgentsRuntimeRepo) UpsertAgentRuntimeSettings(_ context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	return v, nil
}
