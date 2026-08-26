package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/decision"
)

// decision_query_chain.go：M80 1.8 决策链 trace（设计 §5）的双方言
// 递归 CTE 实现。WITH RECURSIVE 在 SQLite/PG 均可用；深度闸（depth < ?）
// 保证脏数据自环时递归有界，最终环去重在 biz 层路径集合完成。

var _ decision.ChainRepo = (*decisionQueryRepo)(nil)

// decisionChainSelectCols：链 CTE 查询 JOIN chain 后 id/parent_decision_id
// 两表同名，列清单必须 dr. 限定（scan 顺序与 decisionRecordSelectCols 一致）。
const decisionChainSelectCols = "dr.id, dr.decision_key, dr.category, dr.scenario, dr.reasoning, dr.outcome, dr.confidence, " +
	"dr.actor_type, dr.actor_key, dr.parent_decision_id, dr.related_entities, dr.source_ref, dr.metadata, " +
	"dr.workspace_id, dr.created_at, dr.updated_at"

// ListUpstream 沿 parent_decision_id 向上追溯（[0]=直接父，逐级递远）。
// 锚点无父（parent_decision_id IS NULL）时种子为空 → 返回空。
func (r *decisionQueryRepo) ListUpstream(ctx context.Context, startID int64, maxDepth int) ([]decision.Record, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || startID <= 0 {
		return nil, nil
	}
	if maxDepth <= 0 {
		maxDepth = 20
	}
	q := r.data.Dialect().RenumberPlaceholders(`
WITH RECURSIVE chain AS (
  SELECT dr.id, dr.parent_decision_id, 1 AS depth
  FROM decision_records dr
  WHERE dr.id = (SELECT parent_decision_id FROM decision_records WHERE id = ?)
  UNION ALL
  SELECT dr.id, dr.parent_decision_id, c.depth + 1
  FROM decision_records dr
  JOIN chain c ON dr.id = c.parent_decision_id
  WHERE c.depth < ?
)
SELECT ` + decisionChainSelectCols + `
FROM decision_records dr JOIN chain c ON dr.id = c.id
ORDER BY c.depth ASC`)
	return r.queryChainRows(ctx, q, startID, maxDepth)
}

// ListDownstream 沿 parent_decision_id = id 向下追溯（深度升序，同层
// 按 created_at/id 稳定序）。
func (r *decisionQueryRepo) ListDownstream(ctx context.Context, startID int64, maxDepth int) ([]decision.Record, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || startID <= 0 {
		return nil, nil
	}
	if maxDepth <= 0 {
		maxDepth = 20
	}
	q := r.data.Dialect().RenumberPlaceholders(`
WITH RECURSIVE chain AS (
  SELECT dr.id, 1 AS depth
  FROM decision_records dr
  WHERE dr.parent_decision_id = ?
  UNION ALL
  SELECT dr.id, c.depth + 1
  FROM decision_records dr
  JOIN chain c ON dr.parent_decision_id = c.id
  WHERE c.depth < ?
)
SELECT ` + decisionChainSelectCols + `
FROM decision_records dr JOIN chain c ON dr.id = c.id
ORDER BY c.depth ASC, dr.created_at ASC, dr.id ASC`)
	return r.queryChainRows(ctx, q, startID, maxDepth)
}

// FindLatestPlannerByRun 找同 run 内 created_at <= before 的最近
// planner_orchestration 决策（虚拟父兜底，设计 §5）。
func (r *decisionQueryRepo) FindLatestPlannerByRun(ctx context.Context, runID, beforeCreatedAt string, excludeID int64) (*decision.Record, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	d := r.data.Dialect()
	q := d.RenumberPlaceholders(
		`SELECT ` + decisionRecordSelectCols + ` FROM decision_records
		 WHERE category = ? AND ` + d.JSONExtract("source_ref", "run_id") + ` = ?
		   AND id != ? AND created_at <= ?
		 ORDER BY created_at DESC, id DESC LIMIT 1`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q,
		string(decision.CategoryPlannerOrchestration), runID, excludeID, beforeCreatedAt)
	if err != nil {
		return nil, entErrToBizErr(err, "DECISION")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanDecisionRecord(rows)
}

// queryChainRows 执行链 CTE 并扫描全部行。
func (r *decisionQueryRepo) queryChainRows(ctx context.Context, q string, args ...any) ([]decision.Record, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "DECISION")
	}
	defer rows.Close()
	out := make([]decision.Record, 0, 8)
	for rows.Next() {
		rec, err := scanDecisionRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "DECISION")
	}
	return out, nil
}
