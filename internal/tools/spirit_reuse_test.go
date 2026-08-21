package tools

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestWantsForceNewOrchestration(t *testing.T) {
	t.Parallel()
	if !wantsForceNewOrchestration(true, "分析金鹏科技") {
		t.Fatal("flag=true must force new")
	}
	if wantsForceNewOrchestration(false, "组建几个团队，分析金鹏科技行情") {
		t.Fatal("repeat of the same ask must not force new")
	}
	if !wantsForceNewOrchestration(false, "请重新组建团队分析茅台") {
		t.Fatal("重新组建 must force new")
	}
}

func TestReusableOverlappingTeams_SameEntity(t *testing.T) {
	t.Parallel()
	teams := []biz.Team{
		{
			ID:              "t1",
			DisplayName:     "核实金鹏科技（金鹏信息）对应A股上市公司主体",
			TaskDescription: "确认证券代码",
			Status:          biz.TeamStatusCompleted,
		},
		{
			ID:          "t2",
			DisplayName: "茅台基本面",
			Status:      biz.TeamStatusCompleted,
		},
	}
	got := reusableOverlappingTeams("组建几个团队，分析金鹏科技行情", teams)
	if len(got) != 1 || got[0].ID != "t1" {
		t.Fatalf("got %+v, want only t1", got)
	}
}

func TestReusableOverlappingTeams_IgnoresGenericListedCompanyToken(t *testing.T) {
	t.Parallel()
	teams := []biz.Team{
		{ID: "t1", DisplayName: "核实对应A股上市公司主体与证券代码", Status: biz.TeamStatusCompleted},
	}
	got := reusableOverlappingTeams("帮我看看另一家上市公司的分红政策", teams)
	if len(got) != 0 {
		t.Fatalf("generic 上市公司/证券代码 must not reuse, got %+v", got)
	}
}

func TestExpandToOrchestrationCohort_IncludesGraphSiblings(t *testing.T) {
	t.Parallel()
	all := []biz.Team{
		{ID: "t1", DisplayName: "核实金鹏科技主体", Status: biz.TeamStatusCompleted, LinkedGraphID: "g1"},
		{ID: "t2", DisplayName: "基本面", Status: biz.TeamStatusCompleted, LinkedGraphID: "g1"},
		{ID: "t3", DisplayName: "茅台", Status: biz.TeamStatusCompleted, LinkedGraphID: "g2"},
	}
	overlap := reusableOverlappingTeams("分析金鹏科技行情", all)
	got := expandToOrchestrationCohort(overlap, all)
	if len(got) != 2 {
		t.Fatalf("cohort = %d want 2 (金鹏 + 基本面 sibling), got %+v", len(got), got)
	}
}

func TestReuseNextAction_RunningWaits(t *testing.T) {
	t.Parallel()
	msg := reuseNextAction([]biz.Team{
		{Status: biz.TeamStatusRunning},
		{Status: biz.TeamStatusCompleted},
	})
	if !strings.Contains(msg, "still running") {
		t.Fatalf("running cohort must tell the model to wait, got %q", msg)
	}
}

func TestReusableOverlappingTeams_IgnoresFailedAndUnrelated(t *testing.T) {
	t.Parallel()
	teams := []biz.Team{
		{ID: "fail", DisplayName: "金鹏科技评审", Status: biz.TeamStatusFailed},
		{ID: "other", DisplayName: "研究新能源车", Status: biz.TeamStatusCompleted},
	}
	got := reusableOverlappingTeams("组建几个团队，分析金鹏科技行情", teams)
	if len(got) != 0 {
		t.Fatalf("got %+v, want none (failed + unrelated)", got)
	}
}

func TestReusableOverlappingTeams_ActiveCounts(t *testing.T) {
	t.Parallel()
	teams := []biz.Team{
		{ID: "run", DisplayName: "金鹏科技技术面", Status: biz.TeamStatusRunning},
	}
	got := reusableOverlappingTeams("继续看金鹏科技怎么走", teams)
	if len(got) != 1 {
		t.Fatalf("active overlapping team should be reusable, got %d", len(got))
	}
}

type stubReuseTeamQuery struct {
	teams []biz.Team
}

func (s *stubReuseTeamQuery) ListActiveTeams(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (s *stubReuseTeamQuery) ListAllTeams(_ context.Context, _ string) ([]biz.Team, error) {
	return s.teams, nil
}
func (s *stubReuseTeamQuery) GetMaxParallelTeams(_ context.Context, _ string) int { return 0 }

func TestTryReuseExistingOrchestration(t *testing.T) {
	t.Parallel()
	deps := planAndExecuteDeps{
		teamQuery: &stubReuseTeamQuery{teams: []biz.Team{{
			ID:          "t1",
			DisplayName: "核实金鹏科技主体",
			Status:      biz.TeamStatusCompleted,
		}}},
		lg: loggateway.NewNoop(),
	}
	out, ok := tryReuseExistingOrchestration(context.Background(), "sess-1", "组建几个团队，分析金鹏科技行情", false, deps)
	if !ok || !out.ReuseExisting || len(out.ExistingTeams) != 1 {
		t.Fatalf("reuse = %v ok=%v teams=%d", out, ok, len(out.ExistingTeams))
	}
	if out.Strategy != reuseStrategy {
		t.Fatalf("strategy = %s, want reuse (not direct)", out.Strategy)
	}

	_, ok = tryReuseExistingOrchestration(context.Background(), "sess-1", "组建几个团队，分析金鹏科技行情", true, deps)
	if ok {
		t.Fatal("force_new must skip reuse")
	}
	_, ok = tryReuseExistingOrchestration(context.Background(), "sess-1", "请重新组建团队分析金鹏科技", false, deps)
	if ok {
		t.Fatal("重新组建 must skip reuse")
	}
}
