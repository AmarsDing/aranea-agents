package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz/office"
	"aranea-agents/pkg/apierror"
)

// 个人任务的列表、改写、删除与附件（工作台「我的任务」「进行中」）；建任务与
// 直查在 office_mail_repo.go。属主隔离在 SQL 内完成。

// officeTaskColumns 任务表的可写列序（INSERT 用）。
const officeTaskColumns = `id, workspace_id, user_id, title, body, source, source_ref, status,
		 assignee_kind, assignee_id, assignee_name, session_id, due_at, created_at, updated_at`

// officeTaskSelect 读路径列序：可写列 + 物化的附件数，与 scanOfficeTask 一一对应。
const officeTaskSelect = officeTaskColumns +
	`, (SELECT COUNT(*) FROM office_task_files f WHERE f.task_id = office_tasks.id)`

type officeTaskScanner interface{ Scan(dest ...any) error }

func scanOfficeTask(row officeTaskScanner, t *office.OfficeTask) error {
	var due sql.NullTime
	if err := row.Scan(&t.ID, &t.WorkspaceID, &t.UserID, &t.Title, &t.Body, &t.Source, &t.SourceRef, &t.Status,
		&t.AssigneeKind, &t.AssigneeID, &t.AssigneeName, &t.SessionID, &due, &t.CreatedAt, &t.UpdatedAt,
		&t.FileCount); err != nil {
		return err
	}
	if due.Valid {
		at := due.Time
		t.DueAt = &at
	}
	return nil
}

func sqlIn(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func (r *OfficeMailRepo) ListOfficeTasks(ctx context.Context, userID string, f office.OfficeTaskFilter, limit int) ([]*office.OfficeTask, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	where := []string{"user_id=?"}
	args := []any{userID}
	if len(f.Statuses) > 0 {
		where = append(where, "status IN ("+sqlIn(len(f.Statuses))+")")
		for _, s := range f.Statuses {
			args = append(args, s)
		}
	}
	if len(f.Sources) > 0 {
		where = append(where, "source IN ("+sqlIn(len(f.Sources))+")")
		for _, s := range f.Sources {
			args = append(args, s)
		}
	}
	switch {
	case f.Assigned > 0:
		where = append(where, "assignee_kind <> ''")
	case f.Assigned < 0:
		where = append(where, "assignee_kind = ''")
	}
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeTaskSelect+`
		 FROM office_tasks WHERE `+strings.Join(where, " AND ")+`
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

func (r *OfficeMailRepo) UpdateOfficeTask(ctx context.Context, t *office.OfficeTask) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_tasks SET title=?, body=?, source=?, source_ref=?, status=?,
		 assignee_kind=?, assignee_id=?, assignee_name=?, session_id=?, due_at=?, updated_at=?
		 WHERE id=? AND user_id=?`),
		t.Title, t.Body, t.Source, t.SourceRef, t.Status,
		t.AssigneeKind, t.AssigneeID, t.AssigneeName, t.SessionID, t.DueAt, t.UpdatedAt,
		t.ID, t.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrOfficeTaskNotFound
	}
	return nil
}

// DeleteOfficeTask 单事务：清邮件回链 + 删任务（附件经外键级联）。
func (r *OfficeMailRepo) DeleteOfficeTask(ctx context.Context, id, userID string) error {
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		db := r.data.RWDB().WriteDB(txCtx)
		if db == nil {
			return apierror.Internal(office.DomainOffice, "repository unavailable")
		}
		if _, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
			`UPDATE office_mail_messages SET task_ref='' WHERE task_ref=? AND user_id=?`), id, userID); err != nil {
			return err
		}
		res, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
			`DELETE FROM office_tasks WHERE id=? AND user_id=?`), id, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return office.ErrOfficeTaskNotFound
		}
		return nil
	})
}

func (r *OfficeMailRepo) ListOfficeTaskFiles(ctx context.Context, taskID, userID string) ([]*office.OfficeTaskFile, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, task_id, user_id, filename, content_type, size, created_at
		 FROM office_task_files WHERE task_id=? AND user_id=? ORDER BY created_at, id`), taskID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.OfficeTaskFile, 0)
	for rows.Next() {
		var f office.OfficeTaskFile
		if err := rows.Scan(&f.ID, &f.TaskID, &f.UserID, &f.Filename, &f.ContentType, &f.Size, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

func (r *OfficeMailRepo) CreateOfficeTaskFile(ctx context.Context, f *office.OfficeTaskFile) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_task_files (id, task_id, user_id, filename, content_type, size, content, created_at)
		 VALUES (?,?,?,?,?,?,?,?)`),
		f.ID, f.TaskID, f.UserID, f.Filename, f.ContentType, f.Size, f.Content, f.CreatedAt)
	return err
}

func (r *OfficeMailRepo) GetOfficeTaskFile(ctx context.Context, id, userID string) (*office.OfficeTaskFile, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, task_id, user_id, filename, content_type, size, content, created_at
		 FROM office_task_files WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, office.ErrOfficeTaskFileNotFound
	}
	var f office.OfficeTaskFile
	if err := rows.Scan(&f.ID, &f.TaskID, &f.UserID, &f.Filename, &f.ContentType, &f.Size, &f.Content, &f.CreatedAt); err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *OfficeMailRepo) DeleteOfficeTaskFile(ctx context.Context, id, userID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM office_task_files WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrOfficeTaskFileNotFound
	}
	return nil
}

const officeTaskExecutionCols = "id, task_id, user_id, kind, ref_id, executor, executor_name, hand, created_at"

func scanOfficeTaskExecution(row officeTaskScanner, e *office.OfficeTaskExecution) error {
	return row.Scan(&e.ID, &e.TaskID, &e.UserID, &e.Kind, &e.RefID, &e.Executor, &e.ExecutorName, &e.Hand, &e.CreatedAt)
}

func (r *OfficeMailRepo) ListOfficeTaskExecutions(ctx context.Context, userID, taskID string, limit int) ([]*office.OfficeTaskExecution, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	// 取最近的 limit 条，再按先后排回去（执行链按时间正序）
	q := "SELECT " + officeTaskExecutionCols + " FROM office_task_executions WHERE user_id=?"
	args := []any{userID}
	if taskID != "" {
		q += " AND task_id=?"
		args = append(args, taskID)
	}
	q = "SELECT " + officeTaskExecutionCols + " FROM (" + q + " ORDER BY created_at DESC, id DESC LIMIT ?) t ORDER BY created_at, id"
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.OfficeTaskExecution, 0)
	for rows.Next() {
		var e office.OfficeTaskExecution
		if err := scanOfficeTaskExecution(rows, &e); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (r *OfficeMailRepo) AddOfficeTaskExecution(ctx context.Context, e *office.OfficeTaskExecution) (*office.OfficeTaskExecution, error) {
	db, err := r.writeDB(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		"INSERT INTO office_task_executions ("+officeTaskExecutionCols+") VALUES (?,?,?,?,?,?,?,?,?) "+
			"ON CONFLICT (task_id, kind, ref_id) DO NOTHING"),
		e.ID, e.TaskID, e.UserID, e.Kind, e.RefID, e.Executor, e.ExecutorName, e.Hand, e.CreatedAt); err != nil {
		return nil, err
	}
	// 已挂过时上面什么都没插：读回实际那一行（同一事务内可见）
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		"SELECT "+officeTaskExecutionCols+" FROM office_task_executions WHERE task_id=? AND kind=? AND ref_id=?"),
		e.TaskID, e.Kind, e.RefID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, office.ErrOfficeTaskExecutionNotFound
	}
	var got office.OfficeTaskExecution
	if err := scanOfficeTaskExecution(rows, &got); err != nil {
		return nil, err
	}
	return &got, nil
}

func (r *OfficeMailRepo) DeleteOfficeTaskExecution(ctx context.Context, id, userID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		"DELETE FROM office_task_executions WHERE id=? AND user_id=?"), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrOfficeTaskExecutionNotFound
	}
	return nil
}
