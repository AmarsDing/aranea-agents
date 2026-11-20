package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
)

// toolCategoryFacet 列出可见范围内实际出现过的分类，供前端分类下拉用。
// 只按工作区可见性过滤，不受分类 / 搜索等筛选影响，否则选中一个分类后下拉只剩它自己。
func (r *toolRepo) toolCategoryFacet(ctx context.Context, client *ent.Client, workspaceID string) ([]string, error) {
	where, args := toolWhereClause(biz.ToolListQuery{WorkspaceID: workspaceID})
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT DISTINCT t.category FROM tools t WHERE `+where+` AND t.category <> '' ORDER BY t.category`), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, entErrToBizErr(err, "TOOL")
		}
		out = append(out, c)
	}
	return out, entErrToBizErr(rows.Err(), "TOOL")
}

// usedToolCount7d 近 7 天至少被调用过一次的工具数。工作区口径同 24h 汇总：租户只算本租户的调用。
func (r *toolRepo) usedToolCount7d(ctx context.Context, client *ent.Client, workspaceID string) (int, error) {
	where := "started_at >= ?"
	args := []any{time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)}
	if ws := strings.TrimSpace(workspaceID); ws != "" {
		where += " AND workspace_id = ?"
		args = append(args, ws)
	}
	var n int
	if err := entQueryRowScan(client, ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT COUNT(DISTINCT tool_key) FROM tool_invocations WHERE `+where), args, &n); err != nil {
		return 0, entErrToBizErr(err, "TOOL")
	}
	return n, nil
}

// withSummaryFacets 给工具汇总补上分类分面与近 7 天用过的工具数（都不受列表筛选影响）。
func (r *toolRepo) withSummaryFacets(ctx context.Context, client *ent.Client, workspaceID string, s biz.ToolSummary) (biz.ToolSummary, error) {
	cats, err := r.toolCategoryFacet(ctx, client, workspaceID)
	if err != nil {
		return biz.ToolSummary{}, err
	}
	s.Categories = cats
	if s.UsedTools7d, err = r.usedToolCount7d(ctx, client, workspaceID); err != nil {
		return biz.ToolSummary{}, err
	}
	return s, nil
}
