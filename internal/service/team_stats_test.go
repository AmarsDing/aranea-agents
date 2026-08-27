package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	bizusage "aranea-agents/internal/biz/usage"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 79-runtime-governance R7：run stats API + JSONL 导出的 service 层测试。
// usage/decision 两个窄源用接口嵌入 fake（只实现 stats 路径触达的方法，
// 其余方法永不调用——嵌入 nil 接口满足宽接口签名）。

type statsUsageRepo struct {
	bizusage.Repo // 嵌入 nil 接口满足宽签名；stats 路径只走下面三个窄方法
	hit           bizusage.RunCacheHitRatio
	peak          bizusage.RunTurnPeak
	members       []bizusage.RunMemberUsage
	membersErr    error
}

func (r *statsUsageRepo) RunCacheHitRatio(_ context.Context, _ string) (bizusage.RunCacheHitRatio, error) {
	return r.hit, nil
}
func (r *statsUsageRepo) RunTurnPeak(_ context.Context, _ string) (bizusage.RunTurnPeak, error) {
	return r.peak, nil
}
func (r *statsUsageRepo) RunMemberUsageStats(_ context.Context, _ string) ([]bizusage.RunMemberUsage, error) {
	return r.members, r.membersErr
}

type statsDecisionRepo struct {
	gates decision.RunGateStats
}

func (r *statsDecisionRepo) ListRecords(_ context.Context, _ decision.ListFilter) ([]decision.Record, int64, error) {
	return nil, 0, nil
}
func (r *statsDecisionRepo) GetByKey(_ context.Context, _ string) (*decision.Record, error) {
	return nil, nil
}
func (r *statsDecisionRepo) RunGateStats(_ context.Context, _ string) (decision.RunGateStats, error) {
	return r.gates, nil
}

// statsTeamRepo 在 summaryTeamRepo 基础上覆盖 team 查找（带 workspace）并
// 实现 TeamRunStatsExportReader 窄口。gotTeamIDs/gotTeamIDsN 记录导出的
// 下推过滤参数（nil=system 全量）；teamIDs 非 nil 时按之模拟 SQL 过滤。
type statsTeamRepo struct {
	summaryTeamRepo
	teams       map[string]biz.Team
	wsTeams     map[string][]biz.Team
	exportRuns  []biz.TeamRunRecord
	gotTeamIDs  []string
	gotTeamIDsN int
	exportErr   error
}

func (r *statsTeamRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	if t, ok := r.teams[id]; ok {
		return t, nil
	}
	return biz.Team{}, apierror.NotFound("TEAM", "team not found")
}

func (r *statsTeamRepo) ListTeamsByWorkspace(_ context.Context, ws string) ([]biz.Team, error) {
	return r.wsTeams[ws], nil
}

func (r *statsTeamRepo) ListTeamRunsForStatsExport(_ context.Context, _, _ time.Time, _ string, teamIDs []string, _ int) ([]biz.TeamRunRecord, error) {
	r.gotTeamIDs = teamIDs
	r.gotTeamIDsN++
	if r.exportErr != nil {
		return nil, r.exportErr
	}
	if teamIDs == nil {
		return r.exportRuns, nil
	}
	// 模拟 SQL 下推过滤（P5.1 M1：service 不再做 Go 侧过滤）。
	allowed := make(map[string]bool, len(teamIDs))
	for _, id := range teamIDs {
		allowed[id] = true
	}
	out := make([]biz.TeamRunRecord, 0, len(r.exportRuns))
	for _, run := range r.exportRuns {
		if allowed[run.TeamID] {
			out = append(out, run)
		}
	}
	return out, nil
}

func newStatsTeamService(repo *statsTeamRepo, usageRepo *statsUsageRepo, decisionRepo *statsDecisionRepo) *TeamService {
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})
	var usageUC *bizusage.Usecase
	if usageRepo != nil {
		usageUC = bizusage.NewUsecase(usageRepo, nil)
	}
	var decisionQ *decision.QueryUsecase
	if decisionRepo != nil {
		decisionQ = decision.NewQueryUsecase(decisionRepo)
	}
	return NewTeamService(uc, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil, nil, nil, nil, nil, nil, usageUC, decisionQ)
}

func TestGetTeamRunStats_AssemblesAllSources(t *testing.T) {
	repo := &statsTeamRepo{
		summaryTeamRepo: summaryTeamRepo{
			runs: map[string]biz.TeamRunRecord{
				"run-1": {ID: "run-1", TeamID: "t1", SessionID: "s1", Status: biz.TeamRunStatusSuccess, CreatedAt: "2026-08-27T01:00:00Z"},
			},
			steps: map[string][]biz.TeamRunStep{
				"run-1": {
					{AgentKey: "planner", ToolCallCount: 3},
					{AgentKey: "planner", ToolCallCount: 1},
					{AgentKey: "executor", ToolCallCount: 2},
				},
			},
		},
		teams: map[string]biz.Team{"t1": {ID: "t1"}},
	}
	svc := newStatsTeamService(repo,
		&statsUsageRepo{
			hit:  bizusage.RunCacheHitRatio{Found: true, PromptTok: 4000, CompletionTok: 800, CachedTok: 3000, Ratio: 0.75},
			peak: bizusage.RunTurnPeak{Found: true, MaxInputTokens: 2500},
			members: []bizusage.RunMemberUsage{
				{AgentKey: "planner", PromptTok: 3000, CompletionTok: 600, CachedTok: 2000, Calls: 2},
				{AgentKey: "", PromptTok: 1000, CompletionTok: 200, CachedTok: 1000, Calls: 1},
			},
		},
		&statsDecisionRepo{gates: decision.RunGateStats{
			LoopGuardBlocks: 2, BudgetTripped: true, NoProgressTripped: true,
			PruneCount: 5, PruneBytes: 20000, CompactCount: 1, ParamRuleDenies: 3,
		}})

	resp, err := svc.GetTeamRunStats(context.Background(), &v1.GetTeamRunStatsRequest{Id: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	st := resp.GetStats()
	if st.GetTurns() != 3 || st.GetToolCalls() != 6 {
		t.Errorf("turns/tool_calls = %d/%d, want 3/6", st.GetTurns(), st.GetToolCalls())
	}
	if st.GetPromptTokens() != 4000 || st.GetCompletionTokens() != 800 || st.GetCachedTokens() != 3000 {
		t.Errorf("tokens = %d/%d/%d, want 4000/800/3000", st.GetPromptTokens(), st.GetCompletionTokens(), st.GetCachedTokens())
	}
	if st.GetCacheHitRatio() != 0.75 {
		t.Errorf("cache_hit_ratio = %v, want 0.75", st.GetCacheHitRatio())
	}
	if st.GetMaxTurnInputTokens() != 2500 {
		t.Errorf("max_turn_input_tokens = %d, want 2500", st.GetMaxTurnInputTokens())
	}
	if st.GetLoopGuardBlocks() != 2 || !st.GetBudgetTripped() || !st.GetNoProgressTripped() {
		t.Errorf("gates = %d/%v/%v, want 2/true/true", st.GetLoopGuardBlocks(), st.GetBudgetTripped(), st.GetNoProgressTripped())
	}
	if st.GetPruneCount() != 5 || st.GetPruneBytes() != 20000 || st.GetCompactCount() != 1 {
		t.Errorf("prune/compact = %d/%d/%d, want 5/20000/1", st.GetPruneCount(), st.GetPruneBytes(), st.GetCompactCount())
	}
	if st.GetParamRuleDenies() != 3 {
		t.Errorf("param_rule_denies = %d, want 3（H7 proto 映射钉死）", st.GetParamRuleDenies())
	}
	if len(st.GetMembers()) != 3 {
		t.Fatalf("members = %+v, want 3（planner + unknown + executor 纯 step 行）", st.GetMembers())
	}
	byKey := map[string]*v1.TeamRunMemberStats{}
	for _, m := range st.GetMembers() {
		byKey[m.GetAgentKey()] = m
	}
	if p := byKey["planner"]; p == nil || p.GetPromptTokens() != 3000 || p.GetCalls() != 2 || p.GetSteps() != 2 {
		t.Errorf("planner member = %+v, want tokens 3000 / calls 2 / steps 2", p)
	}
	if u := byKey[unknownMemberKey]; u == nil || u.GetPromptTokens() != 1000 || u.GetCalls() != 1 {
		t.Errorf("unknown member = %+v, want 空 agent_key 合流到 unknown 桶", u)
	}
	if e := byKey["executor"]; e == nil || e.GetSteps() != 1 || e.GetPromptTokens() != 0 {
		t.Errorf("executor member = %+v, want 纯 step 行 steps 1 / tokens 0", e)
	}
	// P5.1 排序稳定：members 输出按 agent_key 升序（executor < planner <
	// unknown），step 行来自 Go map 随机序，装配后必须统一排序。
	if got := []string{st.GetMembers()[0].GetAgentKey(), st.GetMembers()[1].GetAgentKey(), st.GetMembers()[2].GetAgentKey()}; got[0] != "executor" || got[1] != "planner" || got[2] != unknownMemberKey {
		t.Errorf("members order = %v, want [executor planner unknown]", got)
	}
}

// P5.1 M3：成员用量源读取失败时 members 段回落 step 维度成员行（不整体
// 消失），与 usageUC 未装配的降级语义一致；其余 stats 段不受影响。
func TestGetTeamRunStats_MemberUsageErrorFallback(t *testing.T) {
	repo := &statsTeamRepo{
		summaryTeamRepo: summaryTeamRepo{
			runs: map[string]biz.TeamRunRecord{
				"run-1": {ID: "run-1", TeamID: "t1", Status: biz.TeamRunStatusSuccess},
			},
			steps: map[string][]biz.TeamRunStep{
				"run-1": {{AgentKey: "b1", ToolCallCount: 1}, {AgentKey: "a1"}, {AgentKey: "b1"}},
			},
		},
		teams: map[string]biz.Team{"t1": {ID: "t1"}},
	}
	svc := newStatsTeamService(repo, &statsUsageRepo{
		hit:        bizusage.RunCacheHitRatio{Found: true, PromptTok: 100, CachedTok: 50, Ratio: 0.5},
		membersErr: errors.New("usage store down"),
	}, nil)

	resp, err := svc.GetTeamRunStats(context.Background(), &v1.GetTeamRunStatsRequest{Id: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	st := resp.GetStats()
	// hit 段正常透出（错误只影响 members 源）。
	if st.GetPromptTokens() != 100 || st.GetCacheHitRatio() != 0.5 {
		t.Errorf("hit 段 = %d/%v, want 100/0.5（不应受 members 源故障影响）", st.GetPromptTokens(), st.GetCacheHitRatio())
	}
	// members 回落 step 行：b1 steps=2（toolCalls 不来自 step 行，零值）、
	// a1 steps=1，按 key 升序。
	if len(st.GetMembers()) != 2 {
		t.Fatalf("fallback members = %+v, want 2 纯 step 行", st.GetMembers())
	}
	if st.GetMembers()[0].GetAgentKey() != "a1" || st.GetMembers()[0].GetSteps() != 1 {
		t.Errorf("members[0] = %+v, want a1 steps 1", st.GetMembers()[0])
	}
	if st.GetMembers()[1].GetAgentKey() != "b1" || st.GetMembers()[1].GetSteps() != 2 || st.GetMembers()[1].GetPromptTokens() != 0 {
		t.Errorf("members[1] = %+v, want b1 steps 2 tokens 0", st.GetMembers()[1])
	}
}

func TestGetTeamRunStats_DegradedSources(t *testing.T) {
	repo := &statsTeamRepo{
		summaryTeamRepo: summaryTeamRepo{
			runs: map[string]biz.TeamRunRecord{
				"run-1": {ID: "run-1", TeamID: "t1", Status: biz.TeamRunStatusSuccess},
			},
			steps: map[string][]biz.TeamRunStep{
				"run-1": {{AgentKey: "a1", ToolCallCount: 1}},
			},
		},
		teams: map[string]biz.Team{"t1": {ID: "t1"}},
	}
	// usage/decision 双源缺失（nil）：零值透出，不报错。
	svc := newStatsTeamService(repo, nil, nil)
	resp, err := svc.GetTeamRunStats(context.Background(), &v1.GetTeamRunStatsRequest{Id: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	st := resp.GetStats()
	if st.GetTurns() != 1 || st.GetPromptTokens() != 0 || st.GetLoopGuardBlocks() != 0 {
		t.Errorf("stats = %+v, want turns 1 且其余零值", st)
	}
	if len(st.GetMembers()) != 1 || st.GetMembers()[0].GetAgentKey() != "a1" {
		t.Errorf("members = %+v, want 纯 step 成员行", st.GetMembers())
	}
}

func TestGetTeamRunStats_CrossTenantDenied(t *testing.T) {
	repo := &statsTeamRepo{
		summaryTeamRepo: summaryTeamRepo{
			runs: map[string]biz.TeamRunRecord{
				"run-b": {ID: "run-b", TeamID: "t-b", Status: biz.TeamRunStatusSuccess},
			},
		},
		teams: map[string]biz.Team{"t-b": {ID: "t-b", WorkspaceID: "ws-b"}},
	}
	svc := newStatsTeamService(repo, nil, nil)
	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.GetTeamRunStats(ctx, &v1.GetTeamRunStatsRequest{Id: "run-b"})
	assertNotFound(t, err, "GetTeamRunStats")
}

func TestExportTeamRunStats_TenantFiltering(t *testing.T) {
	repo := &statsTeamRepo{
		summaryTeamRepo: summaryTeamRepo{
			steps: map[string][]biz.TeamRunStep{
				"run-a1": {{AgentKey: "a1"}},
				"run-b1": {{AgentKey: "b1"}},
				"run-sh": {{AgentKey: "sh"}},
			},
		},
		exportRuns: []biz.TeamRunRecord{
			{ID: "run-a1", TeamID: "t-a", Status: biz.TeamRunStatusSuccess, CreatedAt: "2026-08-27T01:00:00Z"},
			{ID: "run-b1", TeamID: "t-b", Status: biz.TeamRunStatusSuccess, CreatedAt: "2026-08-27T02:00:00Z"},
			{ID: "run-sh", TeamID: "t-sh", Status: biz.TeamRunStatusSuccess, CreatedAt: "2026-08-27T03:00:00Z"},
		},
		// ws-a 可见：own t-a + shared t-sh（t-b 不可见）。
		wsTeams: map[string][]biz.Team{
			"ws-a": {{ID: "t-a", WorkspaceID: "ws-a"}, {ID: "t-sh"}},
		},
	}
	svc := newStatsTeamService(repo, nil, nil)

	// system 调用方：全量 3 行，teamIDs nil 下推（不限）。
	sysResp, err := svc.ExportTeamRunStats(workspace.WithSystemWorkspace(context.Background()), &v1.ExportTeamRunStatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if sysResp.GetCount() != 3 {
		t.Fatalf("system export count = %d, want 3", sysResp.GetCount())
	}
	if repo.gotTeamIDs != nil {
		t.Errorf("system export teamIDs = %v, want nil（不限）", repo.gotTeamIDs)
	}
	lines := strings.Split(strings.TrimSpace(sysResp.GetJsonl()), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], `"runId":"run-a1"`) {
		t.Errorf("system jsonl lines = %v", lines)
	}

	// 租户 ws-a：可见 team 集 {t-a, t-sh} 下推（M1），过滤后 2 行。
	tenantResp, err := svc.ExportTeamRunStats(workspace.WithContext(context.Background(), "ws-a"), &v1.ExportTeamRunStatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if tenantResp.GetCount() != 2 {
		t.Fatalf("tenant export count = %d, want 2", tenantResp.GetCount())
	}
	if strings.Contains(tenantResp.GetJsonl(), "run-b1") {
		t.Errorf("tenant jsonl 泄露 ws-b run: %s", tenantResp.GetJsonl())
	}
	if len(repo.gotTeamIDs) != 2 {
		t.Fatalf("tenant export 下推 teamIDs = %v, want [t-a t-sh]", repo.gotTeamIDs)
	}
	gotSet := map[string]bool{repo.gotTeamIDs[0]: true, repo.gotTeamIDs[1]: true}
	if !gotSet["t-a"] || !gotSet["t-sh"] {
		t.Errorf("tenant export 下推 teamIDs = %v, want {t-a, t-sh}", repo.gotTeamIDs)
	}
}

// P5.1 M1：租户无任何可见 team 时短路空导出——不触 repo（防止 nil/空
// 语义混淆导致全量泄露）。
func TestExportTeamRunStats_TenantNoVisibleTeamsShortCircuits(t *testing.T) {
	repo := &statsTeamRepo{
		summaryTeamRepo: summaryTeamRepo{},
		exportRuns:      []biz.TeamRunRecord{{ID: "run-x", TeamID: "t-x", Status: biz.TeamRunStatusSuccess}},
		wsTeams:         map[string][]biz.Team{}, // ws-lonely 无任何可见 team
	}
	svc := newStatsTeamService(repo, nil, nil)
	resp, err := svc.ExportTeamRunStats(workspace.WithContext(context.Background(), "ws-lonely"), &v1.ExportTeamRunStatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCount() != 0 || resp.GetJsonl() != "" {
		t.Errorf("empty export = count %d jsonl %q, want 0/空", resp.GetCount(), resp.GetJsonl())
	}
	if repo.gotTeamIDsN != 0 {
		t.Errorf("repo 被调用 %d 次, want 0（空可见集短路）", repo.gotTeamIDsN)
	}
}

func TestExportTeamRunStats_InvalidWindow(t *testing.T) {
	repo := &statsTeamRepo{summaryTeamRepo: summaryTeamRepo{}}
	svc := newStatsTeamService(repo, nil, nil)
	_, err := svc.ExportTeamRunStats(workspace.WithSystemWorkspace(context.Background()), &v1.ExportTeamRunStatsRequest{From: "not-a-time"})
	if err == nil || !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want BadRequest", err)
	}
}
