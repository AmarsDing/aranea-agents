package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz/approval"
	"aranea-agents/internal/biz/office"
	"aranea-agents/pkg/apierror"
)

// ============================================================================
// office 套件仓储（E 阶段 W1c；迁移 20261321）。非 ent 运营表，raw SQL
// 循 approval_risk_matrix_repo 先例；TIMESTAMPTZ 列由 pg 驱动直接扫
// time.Time。一个 OfficeRepo 具体型满足 office 包全部窄端口 +
// approval.ScheduleDraftReader（wire 按口窄化装配）。
//
// 注意：RWDB().ReadDB/WriteDB 返回 execer 面（仅 ExecContext/QueryContext，
// 无 QueryRowContext）——单行查询走 scanOne 辅助（rows.Next 判命中，
// 循 coding_task_repo.go 先例）。
// ============================================================================

type OfficeRepo struct {
	data *Data
}

var (
	_ office.EventRepo             = (*OfficeRepo)(nil)
	_ office.NotificationRepo      = (*OfficeRepo)(nil)
	_ office.ICalTokenRepo         = (*OfficeRepo)(nil)
	_ office.AppRepo               = (*OfficeRepo)(nil)
	_ office.DraftRepo             = (*OfficeRepo)(nil)
	_ office.ReminderStore         = (*OfficeRepo)(nil)
	_ approval.ScheduleDraftReader = (*OfficeRepo)(nil)
)

func NewOfficeRepo(d *Data) *OfficeRepo { return &OfficeRepo{data: d} }

func (r *OfficeRepo) readDB(ctx context.Context) (execer, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, apierror.Internal(office.DomainOffice, "repository unavailable")
	}
	return db, nil
}

func (r *OfficeRepo) writeDB(ctx context.Context) (execer, error) {
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return nil, apierror.Internal(office.DomainOffice, "repository unavailable")
	}
	return db, nil
}

// scanOne 在 execer 面上模拟 QueryRow().Scan：命中一行则扫描并返回
// found=true；零行返回 found=false（调用方映射领域 404 语义）。
func scanOne(ctx context.Context, db execer, query string, args []any, dest ...any) (bool, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	if err := rows.Scan(dest...); err != nil {
		return false, err
	}
	return true, rows.Err()
}

// ---------- 事件 ----------

const officeEventCols = `id, workspace_id, user_id, task_ref, task_title, title, note, start_at, end_at, timezone, reminder_minutes, source, reminded_at, created_at, updated_at, kind, status, all_day, outcome, location, url, color, rrule, exdates`

func scanOfficeEvent(row interface{ Scan(...any) error }) (*office.Event, error) {
	var ev office.Event
	if err := row.Scan(&ev.ID, &ev.WorkspaceID, &ev.UserID, &ev.TaskRef, &ev.TaskTitle, &ev.Title, &ev.Note,
		&ev.StartAt, &ev.EndAt, &ev.Timezone, &ev.ReminderMinutes, &ev.Source, &ev.RemindedAt, &ev.CreatedAt, &ev.UpdatedAt,
		&ev.Kind, &ev.Status, &ev.AllDay, &ev.Outcome, &ev.Location, &ev.URL, &ev.Color, &ev.RRule, &ev.ExDates); err != nil {
		return nil, err
	}
	return &ev, nil
}

func (r *OfficeRepo) insertEvent(ctx context.Context, db execer, ev *office.Event) error {
	// created_at/updated_at 由仓储落当前时刻（列有 DEFAULT NOW()，但显式列
	// 清单会写入零值时间——调用方不传时间戳，语义集中于此）。
	now := time.Now().UTC()
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = now
	}
	ev.UpdatedAt = now
	// 任务排期、草案批准等路径不填类型 / 状态：按「未开始的计划」落库
	if ev.Kind == "" {
		ev.Kind = office.KindPlan
	}
	if ev.Status == "" {
		ev.Status = office.StatusTodo
	}
	_, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_events (`+officeEventCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		ev.ID, ev.WorkspaceID, ev.UserID, ev.TaskRef, ev.TaskTitle, ev.Title, ev.Note,
		ev.StartAt, ev.EndAt, ev.Timezone, ev.ReminderMinutes, ev.Source, ev.RemindedAt, ev.CreatedAt, ev.UpdatedAt,
		ev.Kind, ev.Status, ev.AllDay, ev.Outcome, ev.Location, ev.URL, ev.Color, ev.RRule, ev.ExDates)
	return err
}

func (r *OfficeRepo) CreateEvent(ctx context.Context, ev *office.Event) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	return r.insertEvent(ctx, db, ev)
}

func (r *OfficeRepo) UpdateEvent(ctx context.Context, ev *office.Event) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_events SET title=?, note=?, start_at=?, end_at=?, timezone=?, reminder_minutes=?, reminded_at=?,
		        kind=?, status=?, all_day=?, outcome=?, location=?, url=?, color=?, rrule=?, exdates=?, updated_at=NOW()
		 WHERE id=? AND user_id=?`),
		ev.Title, ev.Note, ev.StartAt, ev.EndAt, ev.Timezone, ev.ReminderMinutes, ev.RemindedAt,
		ev.Kind, ev.Status, ev.AllDay, ev.Outcome, ev.Location, ev.URL, ev.Color, ev.RRule, ev.ExDates, ev.ID, ev.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrEventNotFound
	}
	return nil
}

func (r *OfficeRepo) DeleteEvent(ctx context.Context, id, userID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM office_events WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrEventNotFound
	}
	return nil
}

func (r *OfficeRepo) GetEvent(ctx context.Context, id, userID string) (*office.Event, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeEventCols+` FROM office_events WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, office.ErrEventNotFound
	}
	return scanOfficeEvent(rows)
}

// ListEventsRange 范围聚合（相交判定与 workbench listEvents 同式：
// start_at < 窗尾 AND end_at > 窗头）。重复系列只要首次开始早于窗尾就带回系列本身，由 biz 按范围展开。
func (r *OfficeRepo) ListEventsRange(ctx context.Context, userID string, start, end time.Time) ([]*office.Event, error) {
	return r.queryEvents(ctx,
		`SELECT `+officeEventCols+` FROM office_events WHERE user_id=?
		   AND ((rrule='' AND start_at<? AND end_at>?) OR (rrule<>'' AND start_at<?)) ORDER BY start_at`,
		userID, end, start, end)
}

func (r *OfficeRepo) queryEvents(ctx context.Context, q string, args ...any) ([]*office.Event, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.Event, 0)
	for rows.Next() {
		ev, err := scanOfficeEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// SearchEvents 关键字检索（标题 / 说明 / 完成情况 / 地点）。
func (r *OfficeRepo) SearchEvents(ctx context.Context, userID, keyword string, limit int) ([]*office.Event, error) {
	like := "%" + escapeMailLike(keyword) + "%"
	return r.queryEvents(ctx,
		`SELECT `+officeEventCols+` FROM office_events WHERE user_id=?
		   AND (title ILIKE ? OR note ILIKE ? OR outcome ILIKE ? OR location ILIKE ?)
		 ORDER BY start_at DESC LIMIT ?`,
		userID, like, like, like, like, limit)
}

// ListRecurringReminders 带提醒、还没做完的重复计划系列。
func (r *OfficeRepo) ListRecurringReminders(ctx context.Context, limit int) ([]*office.Event, error) {
	if limit <= 0 {
		limit = 200
	}
	return r.queryEvents(ctx,
		`SELECT `+officeEventCols+` FROM office_events
		 WHERE rrule<>'' AND reminder_minutes>0 AND kind='plan' AND status IN ('todo','doing')
		 ORDER BY start_at LIMIT ?`, limit)
}

func (r *OfficeRepo) ListTaskEvents(ctx context.Context, userID, taskRef string) ([]*office.Event, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeEventCols+` FROM office_events WHERE user_id=? AND task_ref=? ORDER BY start_at`), userID, taskRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.Event, 0)
	for rows.Next() {
		ev, err := scanOfficeEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// CountOverlap 重叠判定（平移 workbench：NOT (end_at<=start OR start_at>=end)）。
func (r *OfficeRepo) CountOverlap(ctx context.Context, userID string, start, end time.Time, excludeID string) (int, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return 0, err
	}
	return r.countOverlapOn(ctx, db, userID, start, end, excludeID)
}

// countOverlapOn 在指定 execer（可为事务内连接）上执行重叠计数。
func (r *OfficeRepo) countOverlapOn(ctx context.Context, db execer, userID string, start, end time.Time, excludeID string) (int, error) {
	// 只有定了时间段的计划占用时间：工作记录（log）、全天计划不算重叠
	q := `SELECT count(*) FROM office_events WHERE user_id=? AND kind='plan' AND all_day=FALSE AND rrule='' AND NOT (end_at<=? OR start_at>=?)`
	args := []any{userID, start, end}
	if excludeID != "" {
		q += ` AND id<>?`
		args = append(args, excludeID)
	}
	var n int
	if _, err := scanOne(ctx, db, r.data.Dialect().RenumberPlaceholders(q), args, &n); err != nil {
		return 0, err
	}
	return n, nil
}

// ListDueReminders 到期未提醒拾取（due 判定不含上界——停机错过的窗口
// 恢复后首个 tick 补投，宁迟勿漏）。
func (r *OfficeRepo) ListDueReminders(ctx context.Context, now time.Time, limit int) ([]*office.Event, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeEventCols+` FROM office_events
		 WHERE reminder_minutes>0 AND reminded_at IS NULL AND rrule=''
		   AND kind='plan' AND status IN ('todo','doing')
		   AND start_at - make_interval(mins => reminder_minutes) <= ?
		 ORDER BY start_at LIMIT ?`), now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.Event, 0)
	for rows.Next() {
		ev, err := scanOfficeEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *OfficeRepo) MarkReminded(ctx context.Context, id string, at time.Time) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_events SET reminded_at=? WHERE id=? AND reminded_at IS NULL`), at, id)
	return err
}

// DeliverReminderTx 提醒投递事务：先置事件 reminded_at（CAS 抢占；0 行=
// 并发 tick 已投递 → 回滚返回 false），再插通知行。
func (r *OfficeRepo) DeliverReminderTx(ctx context.Context, ev *office.Event, n *office.Notification, at time.Time) (bool, error) {
	delivered := false
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		db := r.data.RWDB().WriteDB(txCtx)
		q, args := `UPDATE office_events SET reminded_at=? WHERE id=? AND reminded_at IS NULL`, []any{at, ev.ID}
		if ev.RecurrenceDay != "" {
			// 重复系列：reminded_at 记已提醒那次的开始时间，只往后推
			q, args = `UPDATE office_events SET reminded_at=? WHERE id=? AND (reminded_at IS NULL OR reminded_at<?)`,
				[]any{ev.StartAt, ev.ID, ev.StartAt}
		}
		res, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(q), args...)
		if err != nil {
			return err
		}
		if aff, _ := res.RowsAffected(); aff == 0 {
			return errOfficeReminderLost
		}
		if _, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
			`INSERT INTO office_notifications (id, user_id, kind, title, body, link) VALUES (?,?,?,?,?,?)`),
			n.ID, n.UserID, n.Kind, n.Title, n.Body, n.Link); err != nil {
			return err
		}
		delivered = true
		return nil
	})
	if err == errOfficeReminderLost {
		return false, nil
	}
	return delivered, err
}

var errOfficeReminderLost = apierror.Conflict(office.DomainOffice, "reminder already delivered")

// ---------- 通知 ----------

func (r *OfficeRepo) CreateNotification(ctx context.Context, n *office.Notification) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_notifications (id, user_id, kind, title, body, link) VALUES (?,?,?,?,?,?)`),
		n.ID, n.UserID, n.Kind, n.Title, n.Body, n.Link)
	return err
}

func (r *OfficeRepo) ListNotifications(ctx context.Context, userID string, unreadOnly bool, page, pageSize int) ([]*office.Notification, int64, int64, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	where := `user_id=?`
	args := []any{userID}
	if unreadOnly {
		where += ` AND NOT read`
	}
	var total, unread int64
	if _, err := scanOne(ctx, db, r.data.Dialect().RenumberPlaceholders(
		`SELECT count(*) FROM office_notifications WHERE `+where), args, &total); err != nil {
		return nil, 0, 0, err
	}
	if _, err := scanOne(ctx, db, r.data.Dialect().RenumberPlaceholders(
		`SELECT count(*) FROM office_notifications WHERE user_id=? AND NOT read`), []any{userID}, &unread); err != nil {
		return nil, 0, 0, err
	}
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, user_id, kind, title, body, link, read, created_at FROM office_notifications
		 WHERE `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`), args...)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()
	out := make([]*office.Notification, 0)
	for rows.Next() {
		var n office.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Kind, &n.Title, &n.Body, &n.Link, &n.Read, &n.CreatedAt); err != nil {
			return nil, 0, 0, err
		}
		out = append(out, &n)
	}
	return out, total, unread, rows.Err()
}

func (r *OfficeRepo) DismissNotification(ctx context.Context, id, userID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_notifications SET read=true WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrNotificationNotFound
	}
	return nil
}

func (r *OfficeRepo) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_notifications SET read=true WHERE user_id=? AND NOT read`), userID)
	return err
}

// ---------- ics 令牌 ----------

func (r *OfficeRepo) CreateICalToken(ctx context.Context, t *office.ICalToken) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_ical_tokens (id, user_id, token_hash, name) VALUES (?,?,?,?)`),
		t.ID, t.UserID, t.TokenHash, t.Name)
	return err
}

func (r *OfficeRepo) RevokeICalToken(ctx context.Context, id, userID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_ical_tokens SET revoked_at=NOW() WHERE id=? AND user_id=? AND revoked_at IS NULL`), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrICalTokenNotFound
	}
	return nil
}

func (r *OfficeRepo) ListICalTokens(ctx context.Context, userID string) ([]*office.ICalToken, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, user_id, name, created_at, revoked_at FROM office_ical_tokens WHERE user_id=? ORDER BY created_at DESC`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.ICalToken, 0)
	for rows.Next() {
		var t office.ICalToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (r *OfficeRepo) ResolveICalToken(ctx context.Context, tokenHash string) (string, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return "", err
	}
	var userID string
	found, err := scanOne(ctx, db, r.data.Dialect().RenumberPlaceholders(
		`SELECT user_id FROM office_ical_tokens WHERE token_hash=? AND revoked_at IS NULL`), []any{tokenHash}, &userID)
	if err != nil {
		return "", err
	}
	if !found {
		return "", office.ErrICalTokenInvalid
	}
	return userID, nil
}

// ---------- 应用启动器 ----------

func (r *OfficeRepo) UpsertApp(ctx context.Context, a *office.App) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_apps (id, workspace_id, name, url, icon, group_name, sort_order) VALUES (?,?,?,?,?,?,?)
		 ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, url=EXCLUDED.url, icon=EXCLUDED.icon,
		   group_name=EXCLUDED.group_name, sort_order=EXCLUDED.sort_order`),
		a.ID, a.WorkspaceID, a.Name, a.URL, a.Icon, a.GroupName, a.SortOrder)
	return err
}

func (r *OfficeRepo) DeleteApp(ctx context.Context, id, workspaceID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM office_apps WHERE id=? AND workspace_id=?`), id, workspaceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrAppNotFound
	}
	return nil
}

func (r *OfficeRepo) ListApps(ctx context.Context, workspaceID string) ([]*office.App, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, workspace_id, name, url, icon, group_name, sort_order, created_at FROM office_apps
		 WHERE workspace_id=? ORDER BY group_name, sort_order, created_at`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.App, 0)
	for rows.Next() {
		var a office.App
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.URL, &a.Icon, &a.GroupName, &a.SortOrder, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ---------- 排期草案 ----------

func scanOfficeDraft(row interface{ Scan(...any) error }) (*office.ScheduleDraft, error) {
	var d office.ScheduleDraft
	var itemsJSON string
	if err := row.Scan(&d.ID, &d.WorkspaceID, &d.UserID, &d.AgentID, &d.SessionID, &d.Status,
		&itemsJSON, &d.DecidedBy, &d.DecideNote, &d.DecidedAt, &d.CreatedAt); err != nil {
		return nil, err
	}
	// items_json 由本服务写入（SubmitDraft 已校验形状）；解析失败按空条目
	// 继续而非 500——审批人仍可见元数据，批准时零条目事务内也无害。
	_ = json.Unmarshal([]byte(itemsJSON), &d.Items)
	return &d, nil
}

const officeDraftCols = `id, workspace_id, user_id, agent_id, session_id, status, items_json, decided_by, decide_note, decided_at, created_at`

func (r *OfficeRepo) CreateDraft(ctx context.Context, d *office.ScheduleDraft) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	itemsJSON, err := json.Marshal(d.Items)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_schedule_drafts (id, workspace_id, user_id, agent_id, session_id, status, items_json) VALUES (?,?,?,?,?,?,?)`),
		d.ID, d.WorkspaceID, d.UserID, d.AgentID, d.SessionID, office.DraftStatusPending, string(itemsJSON))
	return err
}

func (r *OfficeRepo) GetDraft(ctx context.Context, id string) (*office.ScheduleDraft, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeDraftCols+` FROM office_schedule_drafts WHERE id=?`), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, office.ErrDraftNotFound
	}
	return scanOfficeDraft(rows)
}

func (r *OfficeRepo) ListPendingDrafts(ctx context.Context, workspaceID string, limit int) ([]*office.ScheduleDraft, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeDraftCols+` FROM office_schedule_drafts WHERE workspace_id=? AND status='pending' ORDER BY created_at DESC LIMIT ?`),
		workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.ScheduleDraft, 0)
	for rows.Next() {
		d, err := scanOfficeDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *OfficeRepo) CountPendingDrafts(ctx context.Context, workspaceID string) (int64, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	if _, err := scanOne(ctx, db, r.data.Dialect().RenumberPlaceholders(
		`SELECT count(*) FROM office_schedule_drafts WHERE workspace_id=? AND status='pending'`), []any{workspaceID}, &n); err != nil {
		return 0, err
	}
	return n, nil
}

// ApproveDraftTx 单事务批准：逐项复检冲突 → CAS 置 approved → 落事件。
// 任一冲突/已被决议即整体回滚（不部分落库）。
func (r *OfficeRepo) ApproveDraftTx(ctx context.Context, draftID, decidedBy string, events []*office.Event) error {
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		db := r.data.RWDB().WriteDB(txCtx)
		if db == nil {
			return apierror.Internal(office.DomainOffice, "repository unavailable")
		}
		for _, ev := range events {
			n, err := r.countOverlapOn(txCtx, db, ev.UserID, ev.StartAt, ev.EndAt, "")
			if err != nil {
				return err
			}
			if n > 0 {
				return office.ErrEventConflict
			}
		}
		res, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
			`UPDATE office_schedule_drafts SET status=?, decided_by=?, decided_at=NOW() WHERE id=? AND status='pending'`),
			office.DraftStatusApproved, decidedBy, draftID)
		if err != nil {
			return err
		}
		if aff, _ := res.RowsAffected(); aff == 0 {
			return office.ErrDraftNotPending
		}
		for _, ev := range events {
			if err := r.insertEvent(txCtx, db, ev); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *OfficeRepo) RejectDraft(ctx context.Context, draftID, decidedBy, note string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_schedule_drafts SET status=?, decided_by=?, decide_note=?, decided_at=NOW() WHERE id=? AND status='pending'`),
		office.DraftStatusRejected, decidedBy, note, draftID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		// 区分 404 与 409：先查存在性（决议并发语义=409）。
		if _, gerr := r.GetDraft(ctx, draftID); gerr != nil {
			return gerr
		}
		return office.ErrDraftNotPending
	}
	return nil
}
