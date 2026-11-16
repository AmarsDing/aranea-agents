package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz/office"
)

// 个人任务的列表与状态写路径（工作台「我的任务」）；建任务与直查在
// office_mail_repo.go。属主隔离在 SQL 内完成。

func (r *OfficeMailRepo) ListOfficeTasks(ctx context.Context, userID, status string, limit int) ([]*office.OfficeTask, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, workspace_id, user_id, title, body, source, source_ref, status, created_at, updated_at
		 FROM office_tasks WHERE user_id=? AND status=? ORDER BY created_at DESC, id DESC LIMIT ?`), userID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.OfficeTask, 0)
	for rows.Next() {
		var t office.OfficeTask
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.UserID, &t.Title, &t.Body, &t.Source, &t.SourceRef, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (r *OfficeMailRepo) SetOfficeTaskStatus(ctx context.Context, id, userID, status string, at time.Time) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_tasks SET status=?, updated_at=? WHERE id=? AND user_id=?`), status, at, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrOfficeTaskNotFound
	}
	return nil
}
