package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// aggregateTraceCounts runs SELECT <col>, COUNT(*) over monitor_traces with
// the query filters minus the omitted dimension.
func (r *monitorRepo) aggregateTraceCounts(ctx context.Context, d Dialect, query biz.MonitorTracesQuery, col string, omit traceWhereOmit) (map[string]int32, error) {
	where, args := monitorTracesWhere(query, d, omit)
	sql := d.RenumberPlaceholders(`SELECT ` + col + `, COUNT(*) FROM monitor_traces WHERE deleted_at = ''` + where + ` GROUP BY ` + col)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MONITOR")
	}
	defer rows.Close()
	out := map[string]int32{}
	for rows.Next() {
		var k string
		var n int32
		if err := rows.Scan(&k, &n); err != nil {
			return nil, entErrToBizErr(err, "MONITOR")
		}
		out[k] = n
	}
	return out, entErrToBizErr(rows.Err(), "MONITOR")
}

// traceWhereOmit selectively drops filter conditions for aggregate
// side-queries: chip counts must ignore their own dimension's filter,
// otherwise selecting a value would collapse its own count to the total.
type traceWhereOmit struct {
	status bool
	domain bool // drops both Domain and ExcludeInternal conditions
}

func monitorTracesWhere(q biz.MonitorTracesQuery, d Dialect, omit traceWhereOmit) (string, []any) {
	parts := []string{}
	args := []any{}
	// TODO(debt): After backfill completes for all rows, simplify to direct column
	// comparison (e.g. "agent_id = ?") for index utilization. The COALESCE fallback
	// to json_extract is a transition pattern for rows created before the column existed.
	if q.AgentID != "" {
		// Three-level fallback matching the SELECT column: stored column →
		// metadata_json (backfill) → session.agent_id (chat traces where the
		// runner.completion backfill never populated agent_id).
		parts = append(parts, `COALESCE(NULLIF(agent_id, ''), `+d.JSONExtract("metadata_json", "agent_id")+`,
  (SELECT s.agent_id FROM sessions s WHERE s.id = monitor_traces.session_id AND s.deleted_at = '' LIMIT 1), '') = ?`)
		args = append(args, q.AgentID)
	}
	if q.Provider != "" {
		parts = append(parts, "COALESCE(NULLIF(provider, ''), "+d.JSONExtract("metadata_json", "provider")+") = ?")
		args = append(args, q.Provider)
	}
	if q.Model != "" {
		parts = append(parts, "COALESCE(NULLIF(model, ''), "+d.JSONExtract("metadata_json", "model")+") = ?")
		args = append(args, q.Model)
	}
	if q.Status != "" && !omit.status {
		parts = append(parts, "status = ?")
		args = append(args, q.Status)
	}
	if !omit.domain {
		// name 列承载运行域（chat/team/graph/...，见 trace_projector.ensureTrace）。
		// 显式 Domain 优先；否则 ExcludeInternal 排除内部域（cron、skill 同步、
		// MCP 健康检查等高频噪音）。
		if q.Domain != "" {
			parts = append(parts, "name = ?")
			args = append(args, q.Domain)
		} else if q.ExcludeInternal {
			parts = append(parts, "name NOT IN ('system', 'skill')")
		}
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + escapeLikePattern(kw) + "%"
		const esc = ` ESCAPE '\'`
		// 裸列 + 显示名 EXISTS，与 sqlMonitorTracesNames 的解析逻辑（含
		// session→agent 回退）保持一致，让用户按所见名称搜索。
		parts = append(parts, "(name LIKE ?"+esc+" OR trace_key LIKE ?"+esc+" OR agent_id LIKE ?"+esc+
			" OR provider LIKE ?"+esc+" OR model LIKE ?"+esc+
			" OR EXISTS (SELECT 1 FROM agents ka WHERE (ka.id = monitor_traces.agent_id OR ka.agent_key = monitor_traces.agent_id) AND ka.deleted_at = '' AND ka.display_name LIKE ?"+esc+")"+
			" OR EXISTS (SELECT 1 FROM sessions ks WHERE ks.id = monitor_traces.session_id AND ks.deleted_at = '' AND ks.title LIKE ?"+esc+")"+
			" OR EXISTS (SELECT 1 FROM teams kt WHERE (kt.id = monitor_traces.team_id OR kt.team_key = monitor_traces.team_id) AND kt.deleted_at = '' AND kt.display_name LIKE ?"+esc+")"+
			" OR EXISTS (SELECT 1 FROM sessions ks2 JOIN agents ka2 ON (ka2.id = ks2.agent_id OR ka2.agent_key = ks2.agent_id)"+
			" WHERE ks2.id = monitor_traces.session_id AND ks2.deleted_at = '' AND ka2.deleted_at = '' AND ka2.display_name LIKE ?"+esc+"))")
		args = append(args, like, like, like, like, like, like, like, like, like)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " AND " + strings.Join(parts, " AND "), args
}

// escapeLikePattern escapes LIKE metacharacters in user input so keyword
// search stays literal (paired with ESCAPE '\' in SQL).
func escapeLikePattern(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
