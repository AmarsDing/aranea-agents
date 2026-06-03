package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizplugin "aranea-agents/internal/biz/plugin"
)

type pluginRunRepo struct {
	data *Data
}

var _ bizplugin.RunRepo = (*pluginRunRepo)(nil)

func NewPluginRunRepo(data *Data) biz.PluginRunRepo {
	return &pluginRunRepo{data: data}
}

func (r *pluginRunRepo) Insert(ctx context.Context, run biz.PluginRun) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	now := strings.TrimSpace(run.CreatedAt)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `
INSERT INTO plugin_runs (id, plugin_key, plugin_id, session_id, agent_id, callback_point, status, duration_ms, detail_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.PluginKey, run.PluginID, run.SessionID, run.AgentID, run.CallbackPoint, run.Status, run.DurationMS, run.DetailJSON, now,
	)
	return err
}

func (r *pluginRunRepo) List(ctx context.Context, q biz.PluginRunQuery) (biz.PluginRunListResult, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return biz.PluginRunListResult{}, nil
	}
	limit := int(q.Limit)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := int(q.Offset)
	if offset < 0 {
		offset = 0
	}
	where := " WHERE 1=1"
	args := []any{}
	if k := strings.TrimSpace(q.PluginKey); k != "" {
		if strings.HasSuffix(k, ":") {
			where += " AND plugin_key LIKE ?"
			args = append(args, k+"%")
		} else {
			where += " AND plugin_key = ?"
			args = append(args, k)
		}
	}
	if k := strings.TrimSpace(q.PluginID); k != "" {
		where += " AND plugin_id = ?"
		args = append(args, k)
	}
	if k := strings.TrimSpace(q.SessionID); k != "" {
		where += " AND session_id = ?"
		args = append(args, k)
	}
	if k := strings.TrimSpace(q.AgentID); k != "" {
		where += " AND agent_id = ?"
		args = append(args, k)
	}
	if k := strings.TrimSpace(q.CallbackPoint); k != "" {
		where += " AND callback_point = ?"
		args = append(args, k)
	}
	if k := strings.TrimSpace(q.Status); k != "" {
		where += " AND status = ?"
		args = append(args, k)
	}
	if k := strings.TrimSpace(q.From); k != "" {
		where += " AND created_at >= ?"
		args = append(args, k)
	}
	if k := strings.TrimSpace(q.To); k != "" {
		where += " AND created_at <= ?"
		args = append(args, k)
	}
	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), "SELECT COUNT(*) FROM plugin_runs"+where, args, &total); err != nil {
		return biz.PluginRunListResult{}, err
	}
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, `
SELECT id, plugin_key, plugin_id, session_id, agent_id, callback_point, status, duration_ms, detail_json, created_at
FROM plugin_runs`+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return biz.PluginRunListResult{}, err
	}
	defer rows.Close()
	items := make([]biz.PluginRun, 0, limit)
	for rows.Next() {
		var run biz.PluginRun
		if err := rows.Scan(&run.ID, &run.PluginKey, &run.PluginID, &run.SessionID, &run.AgentID, &run.CallbackPoint, &run.Status, &run.DurationMS, &run.DetailJSON, &run.CreatedAt); err != nil {
			return biz.PluginRunListResult{}, err
		}
		items = append(items, run)
	}
	return biz.PluginRunListResult{Items: items, Total: total, Limit: limit, Offset: offset}, rows.Err()
}
