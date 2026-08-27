package data

import (
	"context"
	"strings"

	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

// toolParamRuleRepo is the raw-SQL dual-dialect implementation of
// biztool.ToolParamRuleStore (79-runtime-governance R9 Phase 5.4).
// Table is created by DDL migration 20261262; rows are keyed by canonical
// tool_key (biztool.CanonicalParamRuleToolKey 归一在 biz 层完成，repo 原样落库)。
type toolParamRuleRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biztool.ToolParamRuleStore = (*toolParamRuleRepo)(nil)

// NewToolParamRuleRepo constructs the param-rule store over the DDL-managed table.
func NewToolParamRuleRepo(d *Data, lg loggateway.Logger) *toolParamRuleRepo {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &toolParamRuleRepo{data: d, lg: lg.With(loggateway.Domain("tool_param_rule_repo"))}
}

const toolParamRuleCols = "id, tool_key, pattern, effect, priority, enabled, created_at"

// scanToolParamRuleRows scans id/tool_key/pattern/effect/priority/enabled/created_at rows.
func scanToolParamRuleRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]biztool.ToolParamRule, error) {
	out := []biztool.ToolParamRule{}
	for rows.Next() {
		var rec biztool.ToolParamRule
		var enabled int
		if err := rows.Scan(&rec.ID, &rec.ToolKey, &rec.Pattern, &rec.Effect, &rec.Priority, &enabled, &rec.CreatedAt); err != nil {
			return nil, entErrToBizErr(err, "TOOL_PARAM_RULE")
		}
		rec.Enabled = enabled != 0
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "TOOL_PARAM_RULE")
	}
	return out, nil
}

// GetParamRuleByID returns one rule by primary key (across all tool_keys).
// 无行返回 shared.ErrNotFound，供 biz 层 builtin 只读校验区分「不存在」与
// 「查询失败」（P5.4 H3）。
func (r *toolParamRuleRepo) GetParamRuleByID(ctx context.Context, id string) (biztool.ToolParamRule, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(id) == "" {
		return biztool.ToolParamRule{}, shared.ErrNotFound
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"SELECT " + toolParamRuleCols + " FROM tool_param_rules WHERE id = ?")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, strings.TrimSpace(id))
	if err != nil {
		return biztool.ToolParamRule{}, entErrToBizErr(err, "TOOL_PARAM_RULE")
	}
	defer rows.Close()
	recs, err := scanToolParamRuleRows(rows)
	if err != nil {
		return biztool.ToolParamRule{}, err
	}
	if len(recs) == 0 {
		return biztool.ToolParamRule{}, shared.ErrNotFound
	}
	return recs[0], nil
}

// ListEnabledParamRules returns enabled rules for the tool, evaluation order
// (priority 升序，同 priority 按 id 字典序保证确定性）。
func (r *toolParamRuleRepo) ListEnabledParamRules(ctx context.Context, toolKey string) ([]biztool.ToolParamRule, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(toolKey) == "" {
		return nil, nil
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"SELECT " + toolParamRuleCols + " FROM tool_param_rules" +
			" WHERE tool_key = ? AND enabled = 1 ORDER BY priority ASC, id ASC")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, strings.TrimSpace(toolKey))
	if err != nil {
		return nil, entErrToBizErr(err, "TOOL_PARAM_RULE")
	}
	defer rows.Close()
	return scanToolParamRuleRows(rows)
}

// ListParamRules returns all rules (incl. disabled) for the tool, priority 升序。
func (r *toolParamRuleRepo) ListParamRules(ctx context.Context, toolKey string) ([]biztool.ToolParamRule, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(toolKey) == "" {
		return nil, nil
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"SELECT " + toolParamRuleCols + " FROM tool_param_rules" +
			" WHERE tool_key = ? ORDER BY priority ASC, id ASC")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, strings.TrimSpace(toolKey))
	if err != nil {
		return nil, entErrToBizErr(err, "TOOL_PARAM_RULE")
	}
	defer rows.Close()
	return scanToolParamRuleRows(rows)
}

// UpsertParamRule inserts or updates by primary key. ON CONFLICT DO UPDATE
// 语法 PG 与 SQLite（3.24+）同形，excluded 引用双方言通用。
func (r *toolParamRuleRepo) UpsertParamRule(ctx context.Context, rule biztool.ToolParamRule) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	enabled := 0
	if rule.Enabled {
		enabled = 1
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"INSERT INTO tool_param_rules (" + toolParamRuleCols + ") VALUES (?, ?, ?, ?, ?, ?, ?)" +
			" ON CONFLICT (id) DO UPDATE SET" +
			" tool_key = excluded.tool_key, pattern = excluded.pattern," +
			" effect = excluded.effect, priority = excluded.priority, enabled = excluded.enabled")
	_, err := r.data.RWDB().WriteHandle().ExecContext(ctx, q,
		rule.ID, rule.ToolKey, rule.Pattern, rule.Effect, rule.Priority, enabled, rule.CreatedAt)
	if err != nil {
		return entErrToBizErr(err, "TOOL_PARAM_RULE")
	}
	return nil
}

// DeleteParamRule removes the rule by id; idempotent (absent = no-op).
func (r *toolParamRuleRepo) DeleteParamRule(ctx context.Context, id string) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	q := r.data.Dialect().RenumberPlaceholders("DELETE FROM tool_param_rules WHERE id = ?")
	_, err := r.data.RWDB().WriteHandle().ExecContext(ctx, q, strings.TrimSpace(id))
	if err != nil {
		return entErrToBizErr(err, "TOOL_PARAM_RULE")
	}
	return nil
}
