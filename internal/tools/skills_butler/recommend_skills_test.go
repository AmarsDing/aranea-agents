package skills_butler

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- fakes ---

type fakeSkillUsecase struct {
	suggestions    []biz.SkillEvolutionSuggestion
	suggestionsErr error
	gotStatus      string
}

func (f *fakeSkillUsecase) ListAgentSuggestions(_ context.Context, _ string, status string) ([]biz.SkillEvolutionSuggestion, error) {
	f.gotStatus = status
	if f.suggestionsErr != nil {
		return nil, f.suggestionsErr
	}
	return f.suggestions, nil
}
func (f *fakeSkillUsecase) CreateAgentSkillSuggestion(_ context.Context, agentID, skillName, patternDesc, patternHash string) (*biz.SkillEvolutionSuggestion, error) {
	return &biz.SkillEvolutionSuggestion{
		ID:         "s1",
		TargetType: "agent",
		TargetID:   agentID,
		DraftName:  skillName,
		Status:     biz.EvoSuggestionPending,
	}, nil
}

type fakeSkillQueries struct {
	stats    []SkillInvocationStat
	statsErr error
	gotSince time.Time
}

func (f *fakeSkillQueries) GetSkillInvocationStats(_ context.Context, _ string, since time.Time) ([]SkillInvocationStat, error) {
	f.gotSince = since
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	return f.stats, nil
}

type fakeAnalytics struct{}

func (fakeAnalytics) AnalyzeToolWeights(_ context.Context) ([]biz.ToolWeightReport, error) {
	return nil, nil
}
func (fakeAnalytics) AnalyzeSkillHealth(_ context.Context) ([]biz.SkillHealth, error) {
	return nil, nil
}
func (fakeAnalytics) AnalyzeOrchestration(_ context.Context, _, _ string) ([]biz.OrchestrationModeReport, error) {
	return nil, nil
}

func recommendTestDeps() Deps {
	return Deps{
		Skills:    &fakeSkillUsecase{},
		Queries:   &fakeSkillQueries{},
		Evolution: nil,
		Analytics: fakeAnalytics{},
	}
}

func callRecommend(t *testing.T, deps Deps, args string) recommendSkillsOutput {
	t.Helper()
	tl, ok := newRecommendSkillsTool(deps).(trpctool.CallableTool)
	if !ok {
		t.Fatal("recommend tool must be callable")
	}
	out, err := tl.Call(context.Background(), []byte(args))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	o, ok := out.(recommendSkillsOutput)
	if !ok {
		t.Fatalf("unexpected output type %T", out)
	}
	return o
}

// --- tests ---

func TestRecommendSkills_EmptyAgentID(t *testing.T) {
	tl, ok := newRecommendSkillsTool(recommendTestDeps()).(trpctool.CallableTool)
	if !ok {
		t.Fatal("recommend tool must be callable")
	}
	if _, err := tl.Call(context.Background(), []byte(`{"agent_id":""}`)); !errors.Is(err, ErrAgentIDRequired) {
		t.Fatalf("expected ErrAgentIDRequired, got %v", err)
	}
}

func TestRecommendSkills_PendingProposalsBecomeRecommendations(t *testing.T) {
	deps := recommendTestDeps()
	skills := &fakeSkillUsecase{suggestions: []biz.SkillEvolutionSuggestion{
		{ID: "p1", DraftName: "excel-helper", TriggerReason: "read_spreadsheet→write_document 重复 5 次"},
		{ID: "p2", DraftName: "sql-runner", TriggerReason: "query_db 重复 3 次"},
	}}
	deps.Skills = skills
	out := callRecommend(t, deps, `{"agent_id":"agent-1"}`)
	if out.AgentID != "agent-1" {
		t.Fatalf("unexpected agent id: %+v", out)
	}
	if skills.gotStatus != "pending" {
		t.Fatalf("ListAgentSuggestions must query status=pending, got %q", skills.gotStatus)
	}
	var pending []skillRecommendation
	for _, r := range out.Recommendations {
		if r.Source == "pending_proposal" {
			pending = append(pending, r)
		}
	}
	if len(pending) != 2 || pending[0].SkillName != "excel-helper" || pending[1].SkillName != "sql-runner" {
		t.Fatalf("unexpected pending recommendations: %+v", out.Recommendations)
	}
	if pending[0].Reason == "" {
		t.Fatal("recommendation must carry a reason")
	}
}

func TestRecommendSkills_WarningAndCriticalStats(t *testing.T) {
	deps := recommendTestDeps()
	deps.Queries = &fakeSkillQueries{stats: []SkillInvocationStat{
		{SkillName: "healthy-skill", Count: 30, SuccessRate: 0.9},   // ~7/wk, 90% → healthy
		{SkillName: "warning-skill", Count: 26, SuccessRate: 0.7},   // ~6/wk, 70% → warning
		{SkillName: "critical-low-use", Count: 4, SuccessRate: 0.9}, // <2/wk → critical
		{SkillName: "critical-low-rate", Count: 30, SuccessRate: 0.4},
	}}
	out := callRecommend(t, deps, `{"agent_id":"agent-1"}`)
	bySource := map[string]string{}
	for _, r := range out.Recommendations {
		bySource[r.SkillName] = r.Source
	}
	if _, ok := bySource["healthy-skill"]; ok {
		t.Fatalf("healthy skill must not be recommended: %+v", out.Recommendations)
	}
	if bySource["warning-skill"] != "usage_warning" {
		t.Fatalf("warning-skill source = %q", bySource["warning-skill"])
	}
	if bySource["critical-low-use"] != "usage_critical" {
		t.Fatalf("critical-low-use source = %q", bySource["critical-low-use"])
	}
	if bySource["critical-low-rate"] != "usage_critical" {
		t.Fatalf("critical-low-rate source = %q", bySource["critical-low-rate"])
	}
}

func TestRecommendSkills_ProposalFailureDegradesToUsageOnly(t *testing.T) {
	deps := recommendTestDeps()
	deps.Skills = &fakeSkillUsecase{suggestionsErr: errors.New("db down")}
	deps.Queries = &fakeSkillQueries{stats: []SkillInvocationStat{
		{SkillName: "warning-skill", Count: 26, SuccessRate: 0.7},
	}}
	out := callRecommend(t, deps, `{"agent_id":"agent-1"}`)
	if len(out.Recommendations) != 1 || out.Recommendations[0].Source != "usage_warning" {
		t.Fatalf("expected usage-only recommendations, got %+v", out.Recommendations)
	}
}

func TestRecommendSkills_StatsFailureDegradesToProposalOnly(t *testing.T) {
	deps := recommendTestDeps()
	deps.Skills = &fakeSkillUsecase{suggestions: []biz.SkillEvolutionSuggestion{
		{ID: "p1", DraftName: "excel-helper", TriggerReason: "pattern"},
	}}
	deps.Queries = &fakeSkillQueries{statsErr: errors.New("db down")}
	out := callRecommend(t, deps, `{"agent_id":"agent-1"}`)
	if len(out.Recommendations) != 1 || out.Recommendations[0].Source != "pending_proposal" {
		t.Fatalf("expected proposal-only recommendations, got %+v", out.Recommendations)
	}
}

func TestRecommendSkills_StatsWindowIs30Days(t *testing.T) {
	deps := recommendTestDeps()
	q := &fakeSkillQueries{}
	deps.Queries = q
	_ = callRecommend(t, deps, `{"agent_id":"agent-1"}`)
	since := q.gotSince
	want := time.Now().AddDate(0, 0, -30)
	if d := want.Sub(since); d < -time.Minute || d > time.Minute {
		t.Fatalf("stats window = %v, want ~30d ago (%v)", since, want)
	}
}
