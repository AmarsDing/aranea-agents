package data

import (
	"context"

	"aranea-agents/internal/biz"
)

// MemoryFootprint 平台视角的记忆分布：哪些 Agent 名下有记忆（有效事实 + 未删情景），
// 以及不属于任何 Agent 的事实按作用域各有多少。记忆中心据此默认选「有记忆的 Agent」，
// 而不是列表里第一个（往往什么都没有）。
func (r *l3FactRepo) MemoryFootprint(ctx context.Context, limit int) ([]biz.MemoryAgentFootprint, []biz.MemoryScopeCount, error) {
	if limit <= 0 {
		limit = 20
	}
	db := r.data.RWDB().ReadDB(ctx)
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT agent_id, CAST(SUM(f) AS INTEGER) AS facts, CAST(SUM(e) AS INTEGER) AS episodes FROM (
			SELECT agent_id, COUNT(*) AS f, 0 AS e FROM memory_facts WHERE status = 'active' AND agent_id <> '' GROUP BY agent_id
			UNION ALL
			SELECT agent_id, 0 AS f, COUNT(*) AS e FROM memory_episodes WHERE deleted_at = '' AND agent_id <> '' GROUP BY agent_id
		) t GROUP BY agent_id ORDER BY SUM(f) + SUM(e) DESC, agent_id LIMIT ?`), limit)
	if err != nil {
		return nil, nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	agents := []biz.MemoryAgentFootprint{}
	for rows.Next() {
		var a biz.MemoryAgentFootprint
		if err := rows.Scan(&a.AgentID, &a.Facts, &a.Episodes); err != nil {
			return nil, nil, entErrToBizErr(err, "MEMORY_L3")
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, entErrToBizErr(err, "MEMORY_L3")
	}

	srows, err := db.QueryContext(ctx, `SELECT scope_type, CAST(COUNT(*) AS INTEGER) FROM memory_facts
		WHERE status = 'active' AND agent_id = '' GROUP BY scope_type ORDER BY COUNT(*) DESC, scope_type`)
	if err != nil {
		return nil, nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer srows.Close()
	scopes := []biz.MemoryScopeCount{}
	for srows.Next() {
		var s biz.MemoryScopeCount
		if err := srows.Scan(&s.ScopeType, &s.Facts); err != nil {
			return nil, nil, entErrToBizErr(err, "MEMORY_L3")
		}
		scopes = append(scopes, s)
	}
	return agents, scopes, entErrToBizErr(srows.Err(), "MEMORY_L3")
}
