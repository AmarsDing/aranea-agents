package data

import (
	"context"
	"database/sql"
	"time"

	"aranea-agents/internal/biz/office"
	"aranea-agents/pkg/apierror"
)

// ============================================================================
// office 邮件仓储（E 阶段 W1d；迁移 20261322）。raw SQL 循 office_repo.go
// 先例（非 ent 运营表）；TIMESTAMPTZ 由 pg 驱动直扫 time.Time；可空时间
// 列经 sql.NullTime 转换。一个 OfficeMailRepo 满足 office 包邮件域全部
// 窄端口（账号/邮件/附件/任务四组），wire 按口窄化装配。
//
// 幂等：InsertMessageWithAttachmentsTx 用 ON CONFLICT (account_id,uid)
// DO NOTHING + RowsAffected=0 判命中（部分唯一索引，导入行 account_id=''
// 天然豁免）；附件随邮件同事务（同生共死，失败整体回滚可重试）。
// ============================================================================

type OfficeMailRepo struct {
	data *Data
}

var (
	_ office.MailAccountRepo    = (*OfficeMailRepo)(nil)
	_ office.MailMessageRepo    = (*OfficeMailRepo)(nil)
	_ office.MailAttachmentRepo = (*OfficeMailRepo)(nil)
	_ office.OfficeTaskRepo     = (*OfficeMailRepo)(nil)
)

func NewOfficeMailRepo(d *Data) *OfficeMailRepo { return &OfficeMailRepo{data: d} }

func (r *OfficeMailRepo) readDB(ctx context.Context) (execer, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, apierror.Internal(office.DomainOffice, "repository unavailable")
	}
	return db, nil
}

func (r *OfficeMailRepo) writeDB(ctx context.Context) (execer, error) {
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return nil, apierror.Internal(office.DomainOffice, "repository unavailable")
	}
	return db, nil
}

// ---------- 账号 ----------

const officeMailAccountCols = `id, workspace_id, user_id, name, host, port, tls_mode, username, password_enc, last_uid, last_sync_at, created_at, updated_at`

func scanOfficeMailAccount(row interface{ Scan(...any) error }) (*office.MailAccount, error) {
	var a office.MailAccount
	var lastSync sql.NullTime
	if err := row.Scan(&a.ID, &a.WorkspaceID, &a.UserID, &a.Name, &a.Host, &a.Port,
		&a.TLSMode, &a.Username, &a.PasswordEnc, &a.LastUID, &lastSync, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	if lastSync.Valid {
		t := lastSync.Time
		a.LastSyncAt = &t
	}
	return &a, nil
}

func (r *OfficeMailRepo) CreateMailAccount(ctx context.Context, a *office.MailAccount) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_mail_accounts (`+officeMailAccountCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		a.ID, a.WorkspaceID, a.UserID, a.Name, a.Host, a.Port, a.TLSMode, a.Username,
		a.PasswordEnc, a.LastUID, a.LastSyncAt, a.CreatedAt, a.UpdatedAt)
	return err
}

func (r *OfficeMailRepo) UpdateMailAccount(ctx context.Context, a *office.MailAccount) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_accounts SET name=?, host=?, port=?, tls_mode=?, username=?, password_enc=?, updated_at=NOW()
		 WHERE id=? AND user_id=?`),
		a.Name, a.Host, a.Port, a.TLSMode, a.Username, a.PasswordEnc, a.ID, a.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrMailAccountNotFound
	}
	return nil
}

func (r *OfficeMailRepo) DeleteMailAccount(ctx context.Context, id, userID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM office_mail_accounts WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrMailAccountNotFound
	}
	return nil
}

func (r *OfficeMailRepo) GetMailAccount(ctx context.Context, id, userID string) (*office.MailAccount, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeMailAccountCols+` FROM office_mail_accounts WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, office.ErrMailAccountNotFound
	}
	return scanOfficeMailAccount(rows)
}

func (r *OfficeMailRepo) ListMailAccounts(ctx context.Context, userID string) ([]*office.MailAccount, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeMailAccountCols+` FROM office_mail_accounts WHERE user_id=? ORDER BY created_at`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.MailAccount, 0)
	for rows.Next() {
		a, err := scanOfficeMailAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAllMailAccounts 进程内生产方专用（IDLE supervisor/轮询兜底跨用户）。
func (r *OfficeMailRepo) ListAllMailAccounts(ctx context.Context) ([]*office.MailAccount, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT `+officeMailAccountCols+` FROM office_mail_accounts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.MailAccount, 0)
	for rows.Next() {
		a, err := scanOfficeMailAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *OfficeMailRepo) UpdateMailSyncCursor(ctx context.Context, id string, lastUID int64, at time.Time) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_accounts SET last_uid=?, last_sync_at=?, updated_at=NOW() WHERE id=?`),
		lastUID, at, id)
	return err
}

// ---------- 邮件 ----------

const officeMailMessageCols = `id, workspace_id, user_id, account_id, uid, subject, sender, body, snippet, received_at, classification, quadrant, task_ref, created_at`

func scanOfficeMailMessage(row interface{ Scan(...any) error }) (*office.MailMessage, error) {
	var m office.MailMessage
	var receivedAt sql.NullTime
	if err := row.Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.AccountID, &m.UID, &m.Subject,
		&m.Sender, &m.Body, &m.Snippet, &receivedAt, &m.Classification, &m.Quadrant, &m.TaskRef, &m.CreatedAt); err != nil {
		return nil, err
	}
	if receivedAt.Valid {
		t := receivedAt.Time
		m.ReceivedAt = &t
	}
	return &m, nil
}

// insertMailMessageOn 在指定 execer（可为事务内连接）插入邮件；返回
// inserted=false 表示 (account_id,uid) 幂等命中。
func (r *OfficeMailRepo) insertMailMessageOn(ctx context.Context, db execer, m *office.MailMessage) (bool, error) {
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_mail_messages (`+officeMailMessageCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT (account_id, uid) WHERE account_id <> '' DO NOTHING`),
		m.ID, m.WorkspaceID, m.UserID, m.AccountID, m.UID, m.Subject, m.Sender,
		m.Body, m.Snippet, m.ReceivedAt, m.Classification, m.Quadrant, m.TaskRef, m.CreatedAt)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *OfficeMailRepo) insertMailAttachmentOn(ctx context.Context, db execer, a *office.MailAttachment) error {
	if len(a.Content) > office.MailAttachmentMaxBytes {
		return office.ErrMailAttachmentTooLarge
	}
	_, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_mail_attachments (id, message_id, filename, content_type, size, content) VALUES (?,?,?,?,?,?)`),
		a.ID, a.MessageID, a.Filename, a.ContentType, a.Size, a.Content)
	return err
}

// InsertMessageWithAttachmentsTx 每封邮件一个事务：邮件+附件同生共死；
// 幂等命中（部分唯一索引冲突）时跳过附件插入返回 inserted=false。
func (r *OfficeMailRepo) InsertMessageWithAttachmentsTx(ctx context.Context, m *office.MailMessage) (bool, error) {
	inserted := false
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		db := r.data.RWDB().WriteDB(txCtx)
		if db == nil {
			return apierror.Internal(office.DomainOffice, "repository unavailable")
		}
		ok, err := r.insertMailMessageOn(txCtx, db, m)
		if err != nil {
			return err
		}
		if !ok {
			return nil // 幂等命中：不插附件，事务空提交
		}
		for _, a := range m.Attachments {
			if err := r.insertMailAttachmentOn(txCtx, db, a); err != nil {
				return err // 回滚整封邮件，可重试
			}
		}
		inserted = true
		return nil
	})
	return inserted, err
}

// ListMailMessages 本人邮件分页（received_at NULLS LAST 倒序 + created_at 倒序
// 双键；accountID 空=全账号聚合；quadrant 可选过滤 F2）。
func (r *OfficeMailRepo) ListMailMessages(ctx context.Context, userID, accountID, quadrant string, page, pageSize int) ([]*office.MailMessage, int64, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := `user_id=?`
	args := []any{userID}
	if accountID != "" {
		where += ` AND account_id=?`
		args = append(args, accountID)
	}
	if quadrant != "" {
		where += ` AND quadrant=?`
		args = append(args, quadrant)
	}
	var total int64
	if _, err := scanOne(ctx, db, r.data.Dialect().RenumberPlaceholders(
		`SELECT count(*) FROM office_mail_messages WHERE `+where), args, &total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	qargs := append(append([]any{}, args...), pageSize, offset)
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeMailMessageCols+` FROM office_mail_messages WHERE `+where+`
		 ORDER BY received_at DESC NULLS LAST, created_at DESC LIMIT ? OFFSET ?`), qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*office.MailMessage, 0)
	for rows.Next() {
		m, err := scanOfficeMailMessage(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// has_attachments 聚出（一趟 IN 查询，避免 N+1 逐封 SELECT）。
	if err := r.fillHasAttachments(ctx, db, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// fillHasAttachments 批量填充附件存在标记（message_id IN (...)）。
func (r *OfficeMailRepo) fillHasAttachments(ctx context.Context, db execer, msgs []*office.MailMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]any, 0, len(msgs))
	marks := make([]byte, 0, len(msgs))
	for i, m := range msgs {
		if i > 0 {
			marks = append(marks, ',')
		}
		marks = append(marks, '?')
		ids = append(ids, m.ID)
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT DISTINCT message_id FROM office_mail_attachments WHERE message_id IN (`+string(marks)+`)`), ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		have[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, m := range msgs {
		m.HasAttachments = have[m.ID]
	}
	return nil
}

func (r *OfficeMailRepo) GetMailMessage(ctx context.Context, id, userID string) (*office.MailMessage, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeMailMessageCols+` FROM office_mail_messages WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, office.ErrMailMessageNotFound
	}
	m, err := scanOfficeMailMessage(rows)
	if err != nil {
		return nil, err
	}
	if err := r.fillHasAttachments(ctx, db, []*office.MailMessage{m}); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *OfficeMailRepo) DeleteMailMessage(ctx context.Context, id, userID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM office_mail_messages WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrMailMessageNotFound
	}
	return nil
}

func (r *OfficeMailRepo) SetMailTaskRef(ctx context.Context, id, userID, taskRef string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_messages SET task_ref=? WHERE id=? AND user_id=?`), taskRef, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrMailMessageNotFound
	}
	return nil
}

func (r *OfficeMailRepo) ListMailMessagesSince(ctx context.Context, userID string, since time.Time, limit int) ([]*office.MailMessage, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeMailMessageCols+` FROM office_mail_messages
		 WHERE user_id=? AND received_at>=? ORDER BY received_at ASC NULLS LAST, created_at ASC LIMIT ?`),
		userID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.MailMessage, 0)
	for rows.Next() {
		m, err := scanOfficeMailMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *OfficeMailRepo) ListMailDigestUsers(ctx context.Context, since time.Time) ([]*office.MailDigestCandidate, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT user_id, workspace_id, count(*) FROM office_mail_messages
		 WHERE received_at>=? GROUP BY user_id, workspace_id`), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.MailDigestCandidate, 0)
	for rows.Next() {
		var c office.MailDigestCandidate
		if err := rows.Scan(&c.UserID, &c.WorkspaceID, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (r *OfficeMailRepo) SearchMailMessages(ctx context.Context, userID, query string, limit int) ([]*office.MailMessage, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeMailMessageCols+` FROM office_mail_messages
		 WHERE user_id=? AND (subject ILIKE '%'||?||'%' OR sender ILIKE '%'||?||'%' OR body ILIKE '%'||?||'%')
		 ORDER BY received_at DESC NULLS LAST, created_at DESC LIMIT ?`),
		userID, query, query, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.MailMessage, 0)
	for rows.Next() {
		m, err := scanOfficeMailMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---------- 附件 ----------

func scanOfficeMailAttachment(row interface{ Scan(...any) error }, withContent bool) (*office.MailAttachment, error) {
	var a office.MailAttachment
	var err error
	if withContent {
		err = row.Scan(&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.Size, &a.Content, &a.KnowledgeDocID, &a.CreatedAt)
	} else {
		err = row.Scan(&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.Size, &a.KnowledgeDocID, &a.CreatedAt)
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListMailAttachments 附件元数据列表（不含字节——列表页免载内容）。
func (r *OfficeMailRepo) ListMailAttachments(ctx context.Context, messageID string) ([]*office.MailAttachment, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, message_id, filename, content_type, size, knowledge_doc_id, created_at
		 FROM office_mail_attachments WHERE message_id=? ORDER BY created_at`), messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*office.MailAttachment, 0)
	for rows.Next() {
		a, err := scanOfficeMailAttachment(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetMailAttachment 含字节装载；属主隔离经 JOIN messages(user_id)。
func (r *OfficeMailRepo) GetMailAttachment(ctx context.Context, id, userID string) (*office.MailAttachment, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT a.id, a.message_id, a.filename, a.content_type, a.size, a.content, a.knowledge_doc_id, a.created_at
		 FROM office_mail_attachments a JOIN office_mail_messages m ON m.id=a.message_id
		 WHERE a.id=? AND m.user_id=?`), id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, office.ErrMailAttachmentNotFound
	}
	return scanOfficeMailAttachment(rows, true)
}

func (r *OfficeMailRepo) SetMailAttachmentKnowledgeDoc(ctx context.Context, id, docID string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_attachments SET knowledge_doc_id=? WHERE id=?`), docID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return office.ErrMailAttachmentNotFound
	}
	return nil
}

// ---------- 个人任务（邮件转任务 F1 落点） ----------

func (r *OfficeMailRepo) insertOfficeTaskOn(ctx context.Context, db execer, t *office.OfficeTask) error {
	_, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_tasks (`+officeTaskColumns+`)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		t.ID, t.WorkspaceID, t.UserID, t.Title, t.Body, t.Source, t.SourceRef, t.Status,
		t.AssigneeKind, t.AssigneeID, t.AssigneeName, t.CreatedAt, t.UpdatedAt)
	return err
}

func (r *OfficeMailRepo) CreateOfficeTask(ctx context.Context, t *office.OfficeTask) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	return r.insertOfficeTaskOn(ctx, db, t)
}

func (r *OfficeMailRepo) GetOfficeTask(ctx context.Context, id, userID string) (*office.OfficeTask, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT `+officeTaskColumns+`
		 FROM office_tasks WHERE id=? AND user_id=?`), id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, office.ErrMailMessageNotFound // 任务 404 同族语义（F1 不另开哨兵）
	}
	var t office.OfficeTask
	if err := scanOfficeTask(rows, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateTaskFromMailTx 单事务：建任务 + 回写邮件 task_ref（防孤儿任务）。
func (r *OfficeMailRepo) CreateTaskFromMailTx(ctx context.Context, t *office.OfficeTask, mailID, userID string) error {
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		db := r.data.RWDB().WriteDB(txCtx)
		if db == nil {
			return apierror.Internal(office.DomainOffice, "repository unavailable")
		}
		if err := r.insertOfficeTaskOn(txCtx, db, t); err != nil {
			return err
		}
		res, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
			`UPDATE office_mail_messages SET task_ref=? WHERE id=? AND user_id=?`), t.ID, mailID, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return office.ErrMailMessageNotFound
		}
		return nil
	})
}
