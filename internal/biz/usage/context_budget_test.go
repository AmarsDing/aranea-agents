package usage

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type mockContextBudgetRepo struct {
	mockUsageRepo
	grains    []ContextBudgetGrain
	tools     []ContextBudgetToolStat
	grainsErr error
	toolsErr  error
	gotQuery  Query
	gotLimit  int
}

func (m *mockContextBudgetRepo) ContextBudgetGrains(_ context.Context, q Query) ([]ContextBudgetGrain, error) {
	m.gotQuery = q
	return m.grains, m.grainsErr
}

func (m *mockContextBudgetRepo) ContextBudgetTopTools(_ context.Context, _ Query, limit int) ([]ContextBudgetToolStat, error) {
	m.gotLimit = limit
	return m.tools, m.toolsErr
}

// 两个 agent × 两天的粒度行，验证三视图（整体/按 agent/按日）rollup：
// - g1: agent-a 2026-08-12，2 轮，est_total 1000+3000=4000... 实际由 EstTotalInputSum 直接给出。
// - 缺失 category 的轮次按 0 摊入平均（sum/samples 口径）。
func contextBudgetTestGrains() []ContextBudgetGrain {
	return []ContextBudgetGrain{
		{
			AgentID: "a1", AgentKey: "agent-a", DateKey: "2026-08-12",
			Samples: 2, EstTotalInputSum: 4000, ToolsCountSum: 20,
			CategorySums: map[string]float64{"tools_schema": 2000, "history": 1000},
		},
		{
			AgentID: "a1", AgentKey: "agent-a", DateKey: "2026-08-13",
			Samples: 1, EstTotalInputSum: 3000, ToolsCountSum: 12,
			// 这一天没有 history 记录（未注入）——rollup 时按 0 摊入该 agent 均值。
			CategorySums: map[string]float64{"tools_schema": 1500},
		},
		{
			AgentID: "a2", AgentKey: "agent-b", DateKey: "2026-08-13",
			Samples: 3, EstTotalInputSum: 9000, ToolsCountSum: 30,
			CategorySums: map[string]float64{"tools_schema": 6000, "knowledge_cue": 600},
		},
	}
}

func TestContextBudgetStats_Rollup(t *testing.T) {
	repo := &mockContextBudgetRepo{
		grains: contextBudgetTestGrains(),
		tools: []ContextBudgetToolStat{
			{ToolName: "big_tool", Appearances: 5, AvgEstTokens: 800, MaxEstTokens: 1200},
		},
	}
	uc := NewUsecase(repo, loggateway.NewNoop())

	stats, err := uc.ContextBudgetStats(context.Background(), Query{})
	if err != nil {
		t.Fatalf("ContextBudgetStats: %v", err)
	}

	// 整体：samples=6，est_total=16000，tools=62。
	if stats.Samples != 6 {
		t.Errorf("overall Samples = %d, want 6", stats.Samples)
	}
	if want := 16000.0 / 6; stats.AvgEstTotalInput != want {
		t.Errorf("overall AvgEstTotalInput = %v, want %v", stats.AvgEstTotalInput, want)
	}
	if want := 62.0 / 6; stats.AvgToolsCount != want {
		t.Errorf("overall AvgToolsCount = %v, want %v", stats.AvgToolsCount, want)
	}
	// tools_schema: (2000+1500+6000)/6 = 1583.33；history: 1000/6（缺失日记 0 摊入）。
	if got, want := stats.CategoryAvgEstTokens["tools_schema"], 9500.0/6; got != want {
		t.Errorf("overall tools_schema avg = %v, want %v", got, want)
	}
	if got, want := stats.CategoryAvgEstTokens["history"], 1000.0/6; got != want {
		t.Errorf("overall history avg = %v, want %v (missing days count as 0)", got, want)
	}

	// Per-agent：agent-b（avg 3000）排前，agent-a（avg 7000/3）排后。
	if len(stats.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(stats.Agents))
	}
	if stats.Agents[0].AgentKey != "agent-b" || stats.Agents[1].AgentKey != "agent-a" {
		t.Errorf("agents sort = %s,%s, want agent-b,agent-a (avg est_total desc)",
			stats.Agents[0].AgentKey, stats.Agents[1].AgentKey)
	}
	a := stats.Agents[1]
	if a.Samples != 3 || a.AvgEstTotalInput != 7000.0/3 {
		t.Errorf("agent-a = samples %d avg %v, want 3 / %v", a.Samples, a.AvgEstTotalInput, 7000.0/3)
	}
	// agent-a history: 1000/3（08-13 未记录按 0 摊入）。
	if got, want := a.CategoryAvgEstTokens["history"], 1000.0/3; got != want {
		t.Errorf("agent-a history avg = %v, want %v", got, want)
	}

	// Per-day：日期升序；08-13 合并 agent-a + agent-b（samples=4, est_total=12000）。
	if len(stats.Trends) != 2 {
		t.Fatalf("len(Trends) = %d, want 2", len(stats.Trends))
	}
	if stats.Trends[0].DateKey != "2026-08-12" || stats.Trends[1].DateKey != "2026-08-13" {
		t.Errorf("trend order = %s,%s, want 2026-08-12,2026-08-13",
			stats.Trends[0].DateKey, stats.Trends[1].DateKey)
	}
	day2 := stats.Trends[1]
	if day2.Samples != 4 || day2.AvgEstTotalInput != 12000.0/4 {
		t.Errorf("2026-08-13 = samples %d avg %v, want 4 / %v", day2.Samples, day2.AvgEstTotalInput, 12000.0/4)
	}
	if got, want := day2.CategoryAvgEstTokens["tools_schema"], 7500.0/4; got != want {
		t.Errorf("2026-08-13 tools_schema avg = %v, want %v", got, want)
	}

	// TopTools 透传。
	if len(stats.TopTools) != 1 || stats.TopTools[0].ToolName != "big_tool" {
		t.Errorf("TopTools = %+v, want big_tool row", stats.TopTools)
	}
}

func TestContextBudgetStats_Empty(t *testing.T) {
	repo := &mockContextBudgetRepo{}
	uc := NewUsecase(repo, loggateway.NewNoop())
	stats, err := uc.ContextBudgetStats(context.Background(), Query{})
	if err != nil {
		t.Fatalf("ContextBudgetStats: %v", err)
	}
	if stats.Samples != 0 || len(stats.Agents) != 0 || len(stats.Trends) != 0 {
		t.Errorf("empty stats = %+v, want zero value", stats)
	}
	if stats.CategoryAvgEstTokens == nil {
		t.Error("CategoryAvgEstTokens must be non-nil empty map (proto map marshal)")
	}
}

// repo 未实现窄接口时返回空 stats 而非报错（与 wire.go 类型断言收窄模式一致：
// 复合 usage.Repo 不强制携带该能力）。
func TestContextBudgetStats_RepoWithoutCapability(t *testing.T) {
	uc := NewUsecase(&mockUsageRepo{}, loggateway.NewNoop())
	stats, err := uc.ContextBudgetStats(context.Background(), Query{})
	if err != nil {
		t.Fatalf("ContextBudgetStats: %v", err)
	}
	if stats.Samples != 0 {
		t.Errorf("stats.Samples = %d, want 0 for repo without ContextBudgetStatsRepo", stats.Samples)
	}
}

func TestContextBudgetStats_RepoError(t *testing.T) {
	repo := &mockContextBudgetRepo{grainsErr: errors.New("db down")}
	uc := NewUsecase(repo, loggateway.NewNoop())
	if _, err := uc.ContextBudgetStats(context.Background(), Query{}); err == nil {
		t.Fatal("want error propagated from ContextBudgetGrains")
	}

	repo = &mockContextBudgetRepo{toolsErr: errors.New("db down")}
	uc = NewUsecase(repo, loggateway.NewNoop())
	if _, err := uc.ContextBudgetStats(context.Background(), Query{}); err == nil {
		t.Fatal("want error propagated from ContextBudgetTopTools")
	}
}

// Range 必须经 normalizeQuery 解析为 StartDate/EndDate（默认 30d 窗口），
// repo 收到的 query 带解析后的日期；TopTools limit 用例会常量。
func TestContextBudgetStats_NormalizesRange(t *testing.T) {
	repo := &mockContextBudgetRepo{}
	uc := NewUsecase(repo, loggateway.NewNoop())
	if _, err := uc.ContextBudgetStats(context.Background(), Query{Range: "7d"}); err != nil {
		t.Fatalf("ContextBudgetStats: %v", err)
	}
	if repo.gotQuery.StartDate == "" || repo.gotQuery.EndDate == "" {
		t.Errorf("repo query dates = %q ~ %q, want resolved non-empty", repo.gotQuery.StartDate, repo.gotQuery.EndDate)
	}
	if repo.gotLimit != contextBudgetTopToolsLimit {
		t.Errorf("top tools limit = %d, want %d", repo.gotLimit, contextBudgetTopToolsLimit)
	}

	// 显式日期透传，不被 range 默认覆盖。
	repo2 := &mockContextBudgetRepo{}
	uc2 := NewUsecase(repo2, loggateway.NewNoop())
	if _, err := uc2.ContextBudgetStats(context.Background(), Query{StartDate: "2026-08-01", EndDate: "2026-08-10"}); err != nil {
		t.Fatalf("ContextBudgetStats explicit range: %v", err)
	}
	if repo2.gotQuery.StartDate != "2026-08-01" || repo2.gotQuery.EndDate != "2026-08-10" {
		t.Errorf("explicit range = %q ~ %q, want 2026-08-01 ~ 2026-08-10",
			repo2.gotQuery.StartDate, repo2.gotQuery.EndDate)
	}
}
