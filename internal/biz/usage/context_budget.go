package usage

import (
	"context"
	"sort"
)

// P2-1（29-token.development.md §18）：context_budget 台账的跨 turn 聚合。
// 每轮台账由 mergeContextBudgetMetadata 持久化进 model_token_usage_events
// .metadata_json 的 context_budget 键（S2，29-token.design.md §9.6）；本文件
// 提供 DB 侧聚合读取，回答「token 构成随时间/随 agent 如何变化」（例如
// Tool RAG 上线前后 tools_schema 应下降 ≥80% 的验收基准）。

const (
	// contextBudgetAgentLimit caps the per-agent breakdown rows in one
	// response (dashboard shows the top consumers only).
	contextBudgetAgentLimit = 50
	// contextBudgetTopToolsLimit caps the aggregated top tool schemas (N6
	// 观测：定位 tools_schema 大头，决定是否配置 deferred loading)。
	contextBudgetTopToolsLimit = 20
)

// ContextBudgetGrain is the repo-returned aggregation row at the
// (agent, day) grain — the finest grain needed to roll up overall,
// per-agent, and per-day compositions in Go with a single query pair.
type ContextBudgetGrain struct {
	AgentID   string
	AgentKey  string
	DateKey   string
	Samples   int // turns carrying context_budget metadata at this grain
	// EstTotalInputSum / ToolsCountSum are summed over samples; averages are
	// derived by the rollup (sum/samples).
	EstTotalInputSum float64
	ToolsCountSum    float64
	// CategorySums sums est_tokens per category over the turns WHERE THE
	// CATEGORY WAS RECORDED. A turn that did not inject a category (e.g. no
	// knowledge cue fired) contributes 0 implicitly: the rollup divides by
	// total samples, so missing categories dilute correctly.
	CategorySums map[string]float64
}

// ContextBudgetToolStat aggregates one tool's schema size across the turns
// where it appeared in the per-turn top_tools list.
type ContextBudgetToolStat struct {
	ToolName     string
	Appearances  int
	AvgEstTokens float64
	MaxEstTokens float64
}

// ContextBudgetComposition is the rolled-up per-turn average composition.
// CategoryAvgEstTokens averages over ALL samples (missing category = 0), so
// the map values sum to ≈ AvgEstTotalInput and shares are directly
// comparable: share = CategoryAvgEstTokens[c] / AvgEstTotalInput.
type ContextBudgetComposition struct {
	Samples              int
	AvgEstTotalInput     float64
	AvgToolsCount        float64
	CategoryAvgEstTokens map[string]float64
}

// ContextBudgetAgentStats is the per-agent breakdown row.
type ContextBudgetAgentStats struct {
	AgentID  string
	AgentKey string
	ContextBudgetComposition
}

// ContextBudgetTrendPoint is the per-day composition trend row.
type ContextBudgetTrendPoint struct {
	DateKey string
	ContextBudgetComposition
}

// ContextBudgetStats is the cross-turn aggregation result.
type ContextBudgetStats struct {
	ContextBudgetComposition
	// Agents sorted by AvgEstTotalInput desc, capped at contextBudgetAgentLimit.
	Agents []ContextBudgetAgentStats
	// Trends sorted by DateKey asc.
	Trends []ContextBudgetTrendPoint
	// TopTools sorted by AvgEstTokens desc (SQL-side), capped at
	// contextBudgetTopToolsLimit.
	TopTools []ContextBudgetToolStat
}

// ContextBudgetStatsRepo reads cross-turn aggregations of the persisted
// context budget ledger.
//
// Stability:evolving
type ContextBudgetStatsRepo interface {
	// ContextBudgetGrains aggregates the ledger at the (agent, day) grain.
	// Only turns whose metadata_json carries context_budget are counted.
	ContextBudgetGrains(ctx context.Context, query Query) ([]ContextBudgetGrain, error)
	// ContextBudgetTopTools aggregates the per-turn top_tools arrays across
	// turns, ordered by avg est tokens desc, capped at limit rows.
	ContextBudgetTopTools(ctx context.Context, query Query, limit int) ([]ContextBudgetToolStat, error)
}

// ContextBudgetStats returns the cross-turn token-composition aggregation.
// The narrow capability is resolved via type assertion (same pattern as
// wire.go's CacheHitRatioStatsRepo narrowing) so the composite usage.Repo
// stays untouched; a repo without the capability yields empty stats.
func (u *Usecase) ContextBudgetStats(ctx context.Context, query Query) (ContextBudgetStats, error) {
	repo, ok := u.repo.(ContextBudgetStatsRepo)
	if !ok {
		return ContextBudgetStats{ContextBudgetComposition: ContextBudgetComposition{CategoryAvgEstTokens: map[string]float64{}}}, nil
	}
	query = u.normalizeQuery(query, u.now())
	grains, err := repo.ContextBudgetGrains(ctx, query)
	if err != nil {
		return ContextBudgetStats{}, err
	}
	tools, err := repo.ContextBudgetTopTools(ctx, query, contextBudgetTopToolsLimit)
	if err != nil {
		return ContextBudgetStats{}, err
	}
	return rollupContextBudgetGrains(grains, tools), nil
}

// contextBudgetAccumulator accumulates grain rows into one composition.
type contextBudgetAccumulator struct {
	samples     int
	estTotalSum float64
	toolsSum    float64
	catSums     map[string]float64
}

func (a *contextBudgetAccumulator) add(g ContextBudgetGrain) {
	a.samples += g.Samples
	a.estTotalSum += g.EstTotalInputSum
	a.toolsSum += g.ToolsCountSum
	for cat, sum := range g.CategorySums {
		a.catSums[cat] += sum
	}
}

func (a *contextBudgetAccumulator) composition() ContextBudgetComposition {
	c := ContextBudgetComposition{
		Samples:              a.samples,
		CategoryAvgEstTokens: make(map[string]float64, len(a.catSums)),
	}
	if a.samples == 0 {
		return c
	}
	n := float64(a.samples)
	c.AvgEstTotalInput = a.estTotalSum / n
	c.AvgToolsCount = a.toolsSum / n
	for cat, sum := range a.catSums {
		c.CategoryAvgEstTokens[cat] = sum / n
	}
	return c
}

// rollupContextBudgetGrains rolls (agent, day) grains up into the overall,
// per-agent, and per-day compositions in one pass.
func rollupContextBudgetGrains(grains []ContextBudgetGrain, tools []ContextBudgetToolStat) ContextBudgetStats {
	overall := &contextBudgetAccumulator{catSums: map[string]float64{}}
	type agentKey struct{ id, key string }
	agentAcc := map[agentKey]*contextBudgetAccumulator{}
	dayAcc := map[string]*contextBudgetAccumulator{}
	for _, g := range grains {
		overall.add(g)
		ak := agentKey{g.AgentID, g.AgentKey}
		aa := agentAcc[ak]
		if aa == nil {
			aa = &contextBudgetAccumulator{catSums: map[string]float64{}}
			agentAcc[ak] = aa
		}
		aa.add(g)
		da := dayAcc[g.DateKey]
		if da == nil {
			da = &contextBudgetAccumulator{catSums: map[string]float64{}}
			dayAcc[g.DateKey] = da
		}
		da.add(g)
	}

	stats := ContextBudgetStats{
		ContextBudgetComposition: overall.composition(),
		TopTools:                 tools,
	}
	for ak, aa := range agentAcc {
		stats.Agents = append(stats.Agents, ContextBudgetAgentStats{
			AgentID:                  ak.id,
			AgentKey:                 ak.key,
			ContextBudgetComposition: aa.composition(),
		})
	}
	sort.Slice(stats.Agents, func(i, j int) bool {
		if stats.Agents[i].AvgEstTotalInput != stats.Agents[j].AvgEstTotalInput {
			return stats.Agents[i].AvgEstTotalInput > stats.Agents[j].AvgEstTotalInput
		}
		return stats.Agents[i].AgentKey < stats.Agents[j].AgentKey
	})
	if len(stats.Agents) > contextBudgetAgentLimit {
		stats.Agents = stats.Agents[:contextBudgetAgentLimit]
	}
	for day, da := range dayAcc {
		stats.Trends = append(stats.Trends, ContextBudgetTrendPoint{
			DateKey:                  day,
			ContextBudgetComposition: da.composition(),
		})
	}
	sort.Slice(stats.Trends, func(i, j int) bool {
		return stats.Trends[i].DateKey < stats.Trends[j].DateKey
	})
	return stats
}
