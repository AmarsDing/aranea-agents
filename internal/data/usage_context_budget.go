package data

import (
	"context"
	"sort"

	bizusage "aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/apierror"
)

var _ bizusage.ContextBudgetStatsRepo = (*usageRepo)(nil)

// P2-1（29-token.development.md §18）：context_budget 台账的跨 turn 聚合。
// 数据源是 model_token_usage_events.metadata_json 的 context_budget 键
// （mergeContextBudgetMetadata 每轮写入，S2）。最细粒度 (agent, day) 一次取回，
// 整体/按 agent/按日三视图由 biz 层 rollup。
//
// PG-only：jsonb_each_text / jsonb_array_elements / #>> 无 SQLite 等价物，
// 唯一调用方是生产 Postgres 上的 UsageService（与 CacheHitRatioStats 的
// percentile_cont 同一先例）。CLI SQLite 工具不调用本方法。
//
// 注意：SQL 文本禁止使用 jsonb `?` 存在性算子——会被 RenumberPlaceholders
// 当成占位符改写；"是否携带台账" 谓词用 `-> 'context_budget' IS NOT NULL`。

const (
	// contextBudgetJSONBBase 归一化 metadata_json（TEXT）为 jsonb 基表达式，
	// 与 dialect.JSONBBase 同构；直接写常量避免逐查询拼接函数调用。
	contextBudgetJSONBBase = `COALESCE(NULLIF(metadata_json::text, '')::jsonb, '{}'::jsonb)`
	// contextBudgetHasLedger 是"该行携带 context_budget 台账"的谓词。
	contextBudgetHasLedger = `(` + contextBudgetJSONBBase + ` -> 'context_budget') IS NOT NULL`
)

// contextBudgetNumericPath renders a DOUBLE PRECISION extraction of a numeric
// leaf under the context_budget object (missing/empty → NULL → SUM/AVG skip).
func contextBudgetNumericPath(path string) string {
	return `CAST(NULLIF(` + contextBudgetJSONBBase + ` #>> '{context_budget,` + path + `}', '') AS DOUBLE PRECISION)`
}

func (r *usageRepo) ContextBudgetGrains(ctx context.Context, query bizusage.Query) ([]bizusage.ContextBudgetGrain, error) {
	where, args := usageWhere(query, true)
	samplesWhere := where
	if samplesWhere == "" {
		samplesWhere = " WHERE " + contextBudgetHasLedger
	} else {
		samplesWhere += " AND " + contextBudgetHasLedger
	}

	// 查询 B：(agent, day) 粒度的轮次数 + est_total_input/tools_count 合计。
	samplesQuery := r.data.Dialect().RenumberPlaceholders(
		`SELECT agent_id, agent_key, date_key, COUNT(*), ` +
			`COALESCE(SUM(` + contextBudgetNumericPath("est_total_input") + `), 0), ` +
			`COALESCE(SUM(` + contextBudgetNumericPath("tools_count") + `), 0) ` +
			`FROM model_token_usage_events` + samplesWhere +
			` GROUP BY agent_id, agent_key, date_key`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, samplesQuery, args...)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	type grainKey struct{ agentID, agentKey, dateKey string }
	merged := map[grainKey]*bizusage.ContextBudgetGrain{}
	for rows.Next() {
		var g bizusage.ContextBudgetGrain
		if err = rows.Scan(&g.AgentID, &g.AgentKey, &g.DateKey, &g.Samples, &g.EstTotalInputSum, &g.ToolsCountSum); err != nil {
			rows.Close()
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		merged[grainKey{g.AgentID, g.AgentKey, g.DateKey}] = &g
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	rows.Close()

	// 查询 A：(agent, day, category) 粒度的 est_tokens 合计。jsonb_each_text
	// 对无台账/无 est_tokens 的行返回空集（INNER LATERAL 天然排除），缺失
	// category 的行不产生该 category 的记录——biz rollup 按总轮数摊 0。
	catQuery := r.data.Dialect().RenumberPlaceholders(
		`SELECT agent_id, agent_key, date_key, e.key, SUM(e.value::double precision) ` +
			`FROM model_token_usage_events, LATERAL jsonb_each_text(` +
			contextBudgetJSONBBase + ` #> '{context_budget,est_tokens}') AS e(key, value)` +
			where +
			` GROUP BY agent_id, agent_key, date_key, e.key`)
	catRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, catQuery, args...)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer catRows.Close()
	for catRows.Next() {
		var agentID, agentKey, dateKey, category string
		var sum float64
		if err = catRows.Scan(&agentID, &agentKey, &dateKey, &category, &sum); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		k := grainKey{agentID, agentKey, dateKey}
		g := merged[k]
		if g == nil {
			// 理论不可达：有 category 记录的行必携带台账（查询 B 必覆盖）。
			// 防御：保留行，Samples=0 由 biz 端表现为纯 category 贡献。
			g = &bizusage.ContextBudgetGrain{AgentID: agentID, AgentKey: agentKey, DateKey: dateKey}
			merged[k] = g
		}
		if g.CategorySums == nil {
			g.CategorySums = map[string]float64{}
		}
		g.CategorySums[category] += sum
	}
	if err = catRows.Err(); err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}

	grains := make([]bizusage.ContextBudgetGrain, 0, len(merged))
	for _, g := range merged {
		grains = append(grains, *g)
	}
	sort.Slice(grains, func(i, j int) bool {
		if grains[i].AgentID != grains[j].AgentID {
			return grains[i].AgentID < grains[j].AgentID
		}
		return grains[i].DateKey < grains[j].DateKey
	})
	return grains, nil
}

func (r *usageRepo) ContextBudgetTopTools(ctx context.Context, query bizusage.Query, limit int) ([]bizusage.ContextBudgetToolStat, error) {
	if limit <= 0 {
		limit = 20
	}
	where, args := usageWhere(query, true)
	estTokens := `CAST(NULLIF(t.value ->> 'est_tokens', '') AS DOUBLE PRECISION)`
	nameFilter := `(t.value ->> 'name') IS NOT NULL`
	if where == "" {
		where = " WHERE " + nameFilter
	} else {
		where += " AND " + nameFilter
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`SELECT t.value ->> 'name', COUNT(*), AVG(` + estTokens + `), MAX(` + estTokens + `) ` +
			`FROM model_token_usage_events, LATERAL jsonb_array_elements(` +
			contextBudgetJSONBBase + ` #> '{context_budget,top_tools}') AS t(value)` +
			where +
			` GROUP BY t.value ->> 'name' ORDER BY AVG(` + estTokens + `) DESC, t.value ->> 'name' ASC LIMIT ?`)
	args = append(args, limit)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()
	var out []bizusage.ContextBudgetToolStat
	for rows.Next() {
		var s bizusage.ContextBudgetToolStat
		if err = rows.Scan(&s.ToolName, &s.Appearances, &s.AvgEstTokens, &s.MaxEstTokens); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		out = append(out, s)
	}
	return out, entErrToBizErr(rows.Err(), apierror.DomainData)
}
