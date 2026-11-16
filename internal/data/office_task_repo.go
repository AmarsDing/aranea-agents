package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz/office"
)

// 个人任务的列表与状态写路径（工作台「我的任务」）；建任务与直查在
// office_mail_repo.go。属主隔离在 SQL 内完成。

// officeTaskColumns 任务行的固定列序，与 scanOfficeTask 一一对应。
const officeTaskColumns = `id, workspace_id, user_id, title, body, source, source_ref, status,
		 assignee_kind, assignee_id, assignee_name, created_at, updated_at`

type officeTaskScanner interface{ Scan(dest ...any) error }

func scanOfficeTask(row officeTaskScanner, t *office.OfficeTask) error {
	return row.Scan(&t.ID, &t.WorkspaceID, &t.UserID, &t.Title, &t.Body, &t.Source, &t.SourceRef, &t.Status,
		&t.AssigneeKind, &t.AssigneeID, &t.AssigneeName, &t.CreatedAt, &t.UpdatedAt)
}

func (r *OfficeMailRepo) ListOfficeTasks(ctx context.Context, userID, status string, sources []string, limit int) ([]*office.OfficeTask, error) {
	if len(sources) == 0 {
		return []*office.OfficeTask{}, nil
	}
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	args := make([]any, 0, len(sources)+3)
	args = append(args, userID, status)
	for _, s := range sources {
		args = append(args, s)
	}
	args = append(args, limit)
	in := strings.TrimSuffix(strings.Repeat("?,", len(sources)), ",")
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeTaskColumns+`
		 FROM office_tasks WHERE user_id=? AND status=? AND source IN (`+in+`)
		 ORDER BY created_at DESC, id DESC LIMIT ?`), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.OfficeTask, 0)
	for rows.Next() {
		var t office.OfficeTask
		if err := scanOfficeTask(rows, &t); err != nil {
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
