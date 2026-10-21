package data

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
)

var _ biz.OrchestrationCacheRepo = (*orchestrationCacheRepo)(nil)

type orchestrationCacheRepo struct {
	data *Data
}

func NewOrchestrationCacheRepo(data *Data) biz.OrchestrationCacheRepo {
	return &orchestrationCacheRepo{data: data}
}

func (r *orchestrationCacheRepo) readDB(ctx context.Context) execer {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RWDB().ReadDB(ctx)
}

func (r *orchestrationCacheRepo) writeDB(ctx context.Context) execer {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RWDB().WriteDB(ctx)
}

// normalizeCacheWS 与 biz.normalizeCacheWorkspace 同规则：空/系统桶 → default。
func normalizeCacheWS(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || id == workspace.SystemWorkspaceID {
		return workspace.DefaultWorkspaceID
	}
	return id
}

// LoadCacheJSON 读 legacy system_settings 整包 JSON——仅供 biz InitFromRepo
// 一次性导入新表（D7-F5），导入完成后该方法不再被调用。
func (r *orchestrationCacheRepo) LoadCacheJSON(ctx context.Context) (string, error) {
	var jsonStr string
	db := r.readDB(ctx)
	if db == nil {
		return "", nil
	}
	rows, err := db.QueryContext(ctx,
		`SELECT orchestration_cache_json FROM system_settings WHERE id = 1 LIMIT 1`,
	)
	if err != nil {
		return "", entErrToBizErr(err, "ORCHESTRATION_CACHE")
	}
	defer rows.Close()
	if !rows.Next() {
		return "", nil
	}
	if err := rows.Scan(&jsonStr); err != nil {
		return "", entErrToBizErr(err, "ORCHESTRATION_CACHE")
	}
	return jsonStr, nil
}

// LoadEntries 读 orchestration_cache_entries 全量行（D7-F5）。
// 列级 workspace_id/task_pattern 为权威值（防御 entry_json 内字段漂移）。
func (r *orchestrationCacheRepo) LoadEntries(ctx context.Context) ([]biz.OrchestrationCacheEntry, error) {
	db := r.readDB(ctx)
	if db == nil {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT workspace_id, task_pattern, entry_json FROM orchestration_cache_entries`,
	))
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION_CACHE")
	}
	defer rows.Close()
	var out []biz.OrchestrationCacheEntry
	for rows.Next() {
		var ws, pattern, entryJSON string
		if err := rows.Scan(&ws, &pattern, &entryJSON); err != nil {
			return nil, entErrToBizErr(err, "ORCHESTRATION_CACHE")
		}
		var entry biz.OrchestrationCacheEntry
		if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
			return nil, entErrToBizErr(err, "ORCHESTRATION_CACHE")
		}
		entry.WorkspaceID = normalizeCacheWS(ws)
		entry.TaskPattern = pattern
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION_CACHE")
	}
	return out, nil
}

// UpsertEntry 逐条写回（D7-F5/F6）：PK(workspace_id, task_pattern) upsert。
func (r *orchestrationCacheRepo) UpsertEntry(ctx context.Context, entry biz.OrchestrationCacheEntry) error {
	db := r.writeDB(ctx)
	if db == nil {
		return nil
	}
	entry.WorkspaceID = normalizeCacheWS(entry.WorkspaceID)
	if strings.TrimSpace(entry.TaskPattern) == "" {
		return nil
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return entErrToBizErr(err, "ORCHESTRATION_CACHE")
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
INSERT INTO orchestration_cache_entries (workspace_id, task_pattern, entry_json, dq_score, updated_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, task_pattern) DO UPDATE SET entry_json = excluded.entry_json, dq_score = excluded.dq_score, updated_at = excluded.updated_at`),
		entry.WorkspaceID, entry.TaskPattern, string(entryJSON), entry.DQScore, entry.UpdatedAt,
	)
	return entErrToBizErr(err, "ORCHESTRATION_CACHE")
}

// DeleteEntry 逐条删除（D7-F1 触底淘汰 / D7-F5 TTL 清理）。
func (r *orchestrationCacheRepo) DeleteEntry(ctx context.Context, workspaceID, taskPattern string) error {
	db := r.writeDB(ctx)
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM orchestration_cache_entries WHERE workspace_id = ? AND task_pattern = ?`),
		normalizeCacheWS(workspaceID), taskPattern,
	)
	return entErrToBizErr(err, "ORCHESTRATION_CACHE")
}
