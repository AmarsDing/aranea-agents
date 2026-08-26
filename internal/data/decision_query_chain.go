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

// FindVirtualParentPlanner 为无父记录解析虚拟父 planner 决策（设计 §5，
// 2026-08-26 Gap 修复）。旧实现按 source_ref.run_id 直查 planner——但
// planner 决策只写 flow_trace_id（设计 §3.2 row 1），永落空。两段解析：
//
//  1. ref.FlowTraceID 非空：同 trace planner 决策精确匹配。planner 侧闸
//     （team_count_mismatch）与 planner 决策同 plan_and_execute 作用域
//     trace，flow_trace_id 是确定性最强的关联键。
//  2. ref.RunID 非空：team run 闸（token_budget/no_progress）的 RunID 是
//     team_runs.id；planner 决策 1:N 先于 run 产生、不回写（证据链只读
//     追加），桥接走 team_runs→teams.spirit_session_id→planner
//     metadata.spirit_session_id。
//
// 两者皆空或不命中返回 nil, nil：invocation 级闸（loop_guard）与手动
// run（teams.spirit_session_id 为空）本就无 planner 前置，落空即语义。
func (r *decisionQueryRepo) FindVirtualParentPlanner(ctx context.Context, ref decision.SourceRef, beforeCreatedAt string, excludeID int64) (*decision.Record, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	d := r.data.Dialect()
	// 路径①：flow_trace_id 精确匹配。
	if ft := strings.TrimSpace(ref.FlowTraceID); ft != "" {
		return r.findLatestPlanner(ctx, d.JSONExtract("source_ref", "flow_trace_id"), ft, beforeCreatedAt, excludeID)
	}
	// 路径②：run_id → teams.spirit_session_id 桥接。
	runID := strings.TrimSpace(ref.RunID)
	if runID == "" {
		return nil, nil
	}
	sid, err := r.spiritSessionIDByRun(ctx, runID)
	if err != nil || sid == "" {
		return nil, err
	}
	return r.findLatestPlanner(ctx, d.JSONExtract("metadata", "spirit_session_id"), sid, beforeCreatedAt, excludeID)
}

// spiritSessionIDByRun 经 team_runs→teams 主键 join 取 run 所属 spirit
// 会话 id；run 不存在或 team 非 spirit 编排（空串）时返回 ""。
func (r *decisionQueryRepo) spiritSessionIDByRun(ctx context.Context, runID string) (string, error) {
	q := r.data.Dialect().RenumberPlaceholders(
		`SELECT tm.spirit_session_id FROM team_runs t
		 JOIN teams tm ON tm.id = t.team_id WHERE t.id = ?`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, runID)
	if err != nil {
		return "", entErrToBizErr(err, "DECISION")
	}
	defer rows.Close()
	if !rows.Next() {
		return "", rows.Err()
	}
	var sid string
	if err := rows.Scan(&sid); err != nil {
		return "", entErrToBizErr(err, "DECISION")
	}
	return strings.TrimSpace(sid), rows.Err()
}

// findLatestPlanner 按单一 JSON 键等值条件查 created_at <= before 的
// 最近前置 planner_orchestration 决策（稳定序 created_at DESC, id DESC）。
func (r *decisionQueryRepo) findLatestPlanner(ctx context.Context, keyExpr, keyVal, beforeCreatedAt string, excludeID int64) (*decision.Record, error) {
	d := r.data.Dialect()
	q := d.RenumberPlaceholders(
		`SELECT ` + decisionRecordSelectCols + ` FROM decision_records
		 WHERE category = ? AND ` + keyExpr + ` = ?
		   AND id != ? AND created_at <= ?
		 ORDER BY created_at DESC, id DESC LIMIT 1`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q,
		string(decision.CategoryPlannerOrchestration), keyVal, excludeID, beforeCreatedAt)
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
