package data

import (
	"context"
	"database/sql"
	"strings"
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

const officeMailAccountCols = `id, workspace_id, user_id, name, host, port, tls_mode, username, password_enc, last_uid, last_sync_at, created_at, updated_at, email, provider, smtp_host, smtp_port, smtp_tls, status, last_error, sync_since, junk_last_uid, signature, save_sent`

func scanOfficeMailAccount(row interface{ Scan(...any) error }) (*office.MailAccount, error) {
	var a office.MailAccount
	var lastSync, since sql.NullTime
	if err := row.Scan(&a.ID, &a.WorkspaceID, &a.UserID, &a.Name, &a.Host, &a.Port,
		&a.TLSMode, &a.Username, &a.PasswordEnc, &a.LastUID, &lastSync, &a.CreatedAt, &a.UpdatedAt,
		&a.Email, &a.Provider, &a.SMTPHost, &a.SMTPPort, &a.SMTPTLS, &a.Status, &a.LastError, &since,
		&a.JunkLastUID, &a.Signature, &a.SaveSent); err != nil {
		return nil, err
	}
	if lastSync.Valid {
		t := lastSync.Time
		a.LastSyncAt = &t
	}
	if since.Valid {
		t := since.Time
		a.SyncSince = &t
	}
	return &a, nil
}

func (r *OfficeMailRepo) CreateMailAccount(ctx context.Context, a *office.MailAccount) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_mail_accounts (`+officeMailAccountCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		a.ID, a.WorkspaceID, a.UserID, a.Name, a.Host, a.Port, a.TLSMode, a.Username,
		a.PasswordEnc, a.LastUID, a.LastSyncAt, a.CreatedAt, a.UpdatedAt,
		a.Email, a.Provider, a.SMTPHost, a.SMTPPort, a.SMTPTLS, a.Status, a.LastError, a.SyncSince,
		a.JunkLastUID, a.Signature, a.SaveSent)
	return err
}

func (r *OfficeMailRepo) UpdateMailAccount(ctx context.Context, a *office.MailAccount) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_accounts SET name=?, host=?, port=?, tls_mode=?, username=?, password_enc=?,
		 email=?, provider=?, smtp_host=?, smtp_port=?, smtp_tls=?, signature=?, save_sent=?, updated_at=NOW()
		 WHERE id=? AND user_id=?`),
		a.Name, a.Host, a.Port, a.TLSMode, a.Username, a.PasswordEnc,
		a.Email, a.Provider, a.SMTPHost, a.SMTPPort, a.SMTPTLS, a.Signature, a.SaveSent, a.ID, a.UserID)
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

// UpdateMailAccountStatus 记最近一次连接结果。
func (r *OfficeMailRepo) UpdateMailAccountStatus(ctx context.Context, id, status, lastErr string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_accounts SET status=?, last_error=? WHERE id=?`), status, lastErr, id)
	return err
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

func (r *OfficeMailRepo) UpdateMailJunkCursor(ctx context.Context, id string, lastUID int64) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_accounts SET junk_last_uid=?, updated_at=NOW() WHERE id=?`), lastUID, id)
	return err
}

// ---------- 邮件 ----------

const officeMailMessageCols = `id, workspace_id, user_id, account_id, uid, subject, sender, body, snippet, received_at, classification, quadrant, task_ref, created_at, to_addrs, cc_addrs, seen, flagged, archived_at, folder, body_html, message_id_hdr, references_hdr, trashed_at, spam, labels, send_at`

func scanOfficeMailMessage(row interface{ Scan(...any) error }) (*office.MailMessage, error) {
	var m office.MailMessage
	var receivedAt, archivedAt, trashedAt, sendAt sql.NullTime
	if err := row.Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.AccountID, &m.UID, &m.Subject,
		&m.Sender, &m.Body, &m.Snippet, &receivedAt, &m.Classification, &m.Quadrant, &m.TaskRef, &m.CreatedAt,
		&m.To, &m.Cc, &m.Seen, &m.Flagged, &archivedAt, &m.Folder, &m.BodyHTML, &m.MessageIDHdr, &m.ReferencesHdr,
		&trashedAt, &m.Spam, &m.Labels, &sendAt); err != nil {
		return nil, err
	}
	if receivedAt.Valid {
		t := receivedAt.Time
		m.ReceivedAt = &t
	}
	if archivedAt.Valid {
		t := archivedAt.Time
		m.ArchivedAt = &t
	}
	if trashedAt.Valid {
		t := trashedAt.Time
		m.TrashedAt = &t
	}
	if sendAt.Valid {
		t := sendAt.Time
		m.SendAt = &t
	}
	return &m, nil
}

// insertMailMessageOn 在指定 execer（可为事务内连接）插入邮件；返回
// inserted=false 表示幂等命中（收件箱、垃圾邮件文件夹各有 (account_id,uid) 部分唯一索引，
// 不指定冲突目标，两个索引都当仲裁）。
func (r *OfficeMailRepo) insertMailMessageOn(ctx context.Context, db execer, m *office.MailMessage) (bool, error) {
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`INSERT INTO office_mail_messages (`+officeMailMessageCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT DO NOTHING`),
		m.ID, m.WorkspaceID, m.UserID, m.AccountID, m.UID, m.Subject, m.Sender,
		m.Body, m.Snippet, m.ReceivedAt, m.Classification, m.Quadrant, m.TaskRef, m.CreatedAt,
		m.To, m.Cc, m.Seen, m.Flagged, m.ArchivedAt, mailFolderOrInbox(m.Folder), m.BodyHTML, m.MessageIDHdr, m.ReferencesHdr,
		m.TrashedAt, m.Spam, m.Labels, m.SendAt)
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
		// 同一账户收件箱 / 垃圾箱里已有同一 Message-ID 的信（在别处或这边挪过文件夹，换了 UID）：跳过
		if m.AccountID != "" && m.MessageIDHdr != "" && (m.Folder == "" || m.Folder == office.MailFolderInbox || m.Folder == office.MailFolderJunk) {
			var dup int
			if _, err := scanOne(txCtx, db, r.data.Dialect().RenumberPlaceholders(
				`SELECT count(*) FROM office_mail_messages WHERE account_id=? AND message_id_hdr=? AND folder IN ('inbox','junk')`),
				[]any{m.AccountID, m.MessageIDHdr}, &dup); err != nil {
				return err
			}
			if dup > 0 {
				return nil
			}
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

func mailFolderOrInbox(f string) string {
	if f == "" {
		return office.MailFolderInbox
	}
	return f
}

// mailViewWhere 视图 → WHERE 片段（只拼常量，不拼用户输入）。
// 已删除、垃圾邮件各是一个独立视图，别的视图都不列它们；垃圾邮件文件夹同步来的信被点了
// 「不是垃圾邮件」后（spam=FALSE）算收件箱。
func mailViewWhere(view string) string {
	const live = `trashed_at IS NULL AND spam=FALSE`
	const inbox = `folder IN ('inbox','junk') AND archived_at IS NULL AND ` + live
	switch view {
	case office.MailViewUnread:
		return inbox + ` AND seen=FALSE`
	case office.MailViewFlagged:
		return `flagged=TRUE AND folder<>'drafts' AND ` + live
	case office.MailViewTask:
		return `task_ref<>'' AND ` + live
	case office.MailViewAttachments:
		return `folder<>'drafts' AND ` + live + ` AND EXISTS (SELECT 1 FROM office_mail_attachments a WHERE a.message_id=office_mail_messages.id)`
	case office.MailViewSent:
		return `folder='sent' AND trashed_at IS NULL`
	case office.MailViewDrafts:
		return `folder='drafts' AND trashed_at IS NULL`
	case office.MailViewArchived:
		return `archived_at IS NOT NULL AND ` + live
	case office.MailViewSpam:
		return `spam=TRUE AND trashed_at IS NULL`
	case office.MailViewTrash:
		return `trashed_at IS NOT NULL`
	default:
		return inbox
	}
}

// QueryMailMessages 邮件页列表：视图 + 账号 + 关键字（主题 / 发件人 / 收件人 / 抄送 / 正文 / 附件名）。
func (r *OfficeMailRepo) QueryMailMessages(ctx context.Context, userID string, q office.MailQuery) ([]*office.MailMessage, int64, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := `user_id=? AND ` + mailViewWhere(q.View)
	args := []any{userID}
	if q.AccountID != "" {
		where += ` AND account_id=?`
		args = append(args, q.AccountID)
	}
	if q.Label != "" {
		where += ` AND (','||labels||',') LIKE ?`
		args = append(args, "%,"+escapeMailLike(q.Label)+",%")
	}
	if q.Keyword != "" {
		like := "%" + escapeMailLike(q.Keyword) + "%"
		where += ` AND (subject ILIKE ? OR sender ILIKE ? OR to_addrs ILIKE ? OR cc_addrs ILIKE ? OR body ILIKE ?
		 OR EXISTS (SELECT 1 FROM office_mail_attachments a WHERE a.message_id=office_mail_messages.id AND a.filename ILIKE ?))`
		args = append(args, like, like, like, like, like, like)
	}
	var total int64
	if _, err := scanOne(ctx, db, r.data.Dialect().RenumberPlaceholders(
		`SELECT count(*) FROM office_mail_messages WHERE `+where), args, &total); err != nil {
		return nil, 0, err
	}
	qargs := append(append([]any{}, args...), q.PageSize, (q.Page-1)*q.PageSize)
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
		m.Body, m.BodyHTML = "", "" // 列表不带正文
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := r.fillHasAttachments(ctx, db, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// escapeMailLike 关键字里的 % _ 和反斜杠按字面量匹配（PG 默认转义符是反斜杠）。
func escapeMailLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// CountMailbox 左栏计数（一趟聚合）+ 各账号收件箱未读。
func (r *OfficeMailRepo) CountMailbox(ctx context.Context, userID string) (*office.MailboxCounts, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	c := &office.MailboxCounts{AccountUnread: map[string]int64{}}
	if _, err := scanOne(ctx, db, r.data.Dialect().RenumberPlaceholders(`SELECT
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewUnread)+`),
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewFlagged)+`),
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewTask)+`),
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewAttachments)+`),
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewSent)+`),
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewDrafts)+`),
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewArchived)+`),
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewSpam)+` AND seen=FALSE),
		 count(*) FILTER (WHERE `+mailViewWhere(office.MailViewTrash)+`)
		 FROM office_mail_messages WHERE user_id=?`), []any{userID},
		&c.InboxUnread, &c.Flagged, &c.Task, &c.Attachments, &c.Sent, &c.Drafts, &c.Archived,
		&c.SpamUnread, &c.Trash); err != nil {
		return nil, err
	}
	lrows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT l, count(*) FROM office_mail_messages, unnest(string_to_array(labels, ',')) AS l
		 WHERE user_id=? AND labels<>'' AND trashed_at IS NULL GROUP BY l ORDER BY l`), userID)
	if err != nil {
		return nil, err
	}
	for lrows.Next() {
		var lc office.MailLabelCount
		if err := lrows.Scan(&lc.Name, &lc.Count); err != nil {
			lrows.Close()
			return nil, err
		}
		c.Labels = append(c.Labels, lc)
	}
	lrows.Close()
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT account_id, count(*) FROM office_mail_messages
		 WHERE user_id=? AND account_id<>'' AND `+mailViewWhere(office.MailViewUnread)+` GROUP BY account_id`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		c.AccountUnread[id] = n
	}
	return c, rows.Err()
}

// MarkMailMessages 批量标记（只改调用者自己的信）。
func (r *OfficeMailRepo) MarkMailMessages(ctx context.Context, userID string, ids []string, mk office.MailMark) (int64, error) {
	db, err := r.writeDB(ctx)
	if err != nil {
		return 0, err
	}
	var sets []string
	var args []any
	if mk.Seen != nil {
		sets = append(sets, `seen=?`)
		args = append(args, *mk.Seen)
	}
	if mk.Flagged != nil {
		sets = append(sets, `flagged=?`)
		args = append(args, *mk.Flagged)
	}
	if mk.Archived != nil {
		if *mk.Archived {
			sets = append(sets, `archived_at=COALESCE(archived_at, NOW())`)
		} else {
			sets = append(sets, `archived_at=NULL`)
		}
	}
	if mk.Trashed != nil {
		if *mk.Trashed {
			sets = append(sets, `trashed_at=COALESCE(trashed_at, NOW())`)
		} else {
			sets = append(sets, `trashed_at=NULL`)
		}
	}
	if mk.Spam != nil {
		sets = append(sets, `spam=?`)
		args = append(args, *mk.Spam)
	}
	if mk.AddLabel != "" {
		sets = append(sets, `labels=CASE WHEN (','||labels||',') LIKE ? THEN labels WHEN labels='' THEN ? ELSE labels||','||? END`)
		args = append(args, "%,"+escapeMailLike(mk.AddLabel)+",%", mk.AddLabel, mk.AddLabel)
	}
	if mk.RemoveLabel != "" {
		sets = append(sets, `labels=trim(both ',' from replace(','||labels||',', ?, ','))`)
		args = append(args, ","+mk.RemoveLabel+",")
	}
	if len(sets) == 0 || len(ids) == 0 {
		return 0, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_messages SET `+strings.Join(sets, ", ")+` WHERE user_id=? AND id IN (`+marks+`)`), args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// PurgeMailMessages 彻底删除「已删除」/「垃圾邮件」视图里的信（附件 ON DELETE CASCADE）。
// 视图条件照样拼上：ids 里混进收件箱的信也不会被删。
func (r *OfficeMailRepo) PurgeMailMessages(ctx context.Context, userID, view string, ids []string) (int64, error) {
	if view != office.MailViewTrash && view != office.MailViewSpam {
		return 0, office.ErrMailViewInvalid
	}
	db, err := r.writeDB(ctx)
	if err != nil {
		return 0, err
	}
	where := `user_id=? AND ` + mailViewWhere(view)
	args := []any{userID}
	if len(ids) > 0 {
		where += ` AND id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + `)`
		for _, id := range ids {
			args = append(args, id)
		}
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM office_mail_messages WHERE `+where), args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// SaveMailDraftTx 单事务：删旧草稿（只删 drafts 文件夹里的）→ 插新草稿和附件。
func (r *OfficeMailRepo) SaveMailDraftTx(ctx context.Context, userID, replaceID string, m *office.MailMessage) error {
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		db := r.data.RWDB().WriteDB(txCtx)
		if db == nil {
			return apierror.Internal(office.DomainOffice, "repository unavailable")
		}
		if replaceID != "" {
			if _, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
				`DELETE FROM office_mail_messages WHERE id=? AND user_id=? AND folder='drafts'`), replaceID, userID); err != nil {
				return err
			}
		}
		if _, err := r.insertMailMessageOn(txCtx, db, m); err != nil {
			return err
		}
		for _, a := range m.Attachments {
			if err := r.insertMailAttachmentOn(txCtx, db, a); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListMailMessages 本人邮件分页（received_at NULLS LAST 倒序 + created_at 倒序
// 双键；accountID 空=全账号聚合；quadrant 可选过滤 F2）。
func (r *OfficeMailRepo) ListMailMessages(ctx context.Context, userID, accountID, quadrant string, page, pageSize int) ([]*office.MailMessage, int64, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := `user_id=? AND trashed_at IS NULL AND spam=FALSE`
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
		 WHERE user_id=? AND received_at>=? AND trashed_at IS NULL AND spam=FALSE ORDER BY received_at ASC NULLS LAST, created_at ASC LIMIT ?`),
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
		 WHERE received_at>=? AND trashed_at IS NULL AND spam=FALSE GROUP BY user_id, workspace_id`), since)
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
		 WHERE user_id=? AND trashed_at IS NULL AND spam=FALSE AND (subject ILIKE '%'||?||'%' OR sender ILIKE '%'||?||'%' OR body ILIKE '%'||?||'%')
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
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		t.ID, t.WorkspaceID, t.UserID, t.Title, t.Body, t.Source, t.SourceRef, t.Status,
		t.AssigneeKind, t.AssigneeID, t.AssigneeName, t.SessionID, t.DueAt, t.CreatedAt, t.UpdatedAt)
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
		`SELECT `+officeTaskSelect+`
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

// ---------- 第二轮：会话 / 联想 / 全部已读 / 后台清理 / 定时发送 / 拒收 ----------

func (r *OfficeMailRepo) queryMailRows(ctx context.Context, q string, args ...any) ([]*office.MailMessage, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
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

// ListThread 主题去回复 / 转发前缀后小写相等（同 biz threadKey 的口径）。
func (r *OfficeMailRepo) ListThread(ctx context.Context, userID, subjectKey string, limit int) ([]*office.MailMessage, error) {
	out, err := r.queryMailRows(ctx,
		`SELECT `+officeMailMessageCols+` FROM office_mail_messages
		 WHERE user_id=? AND trashed_at IS NULL AND folder<>'drafts'
		   AND lower(btrim(regexp_replace(subject, '^\s*((re|fw|fwd|aw|回复|答复|转发)\s*[:：]\s*)+', '', 'i'))) = ?
		 ORDER BY received_at ASC NULLS LAST, created_at ASC LIMIT ?`, userID, subjectKey, limit)
	if err != nil {
		return nil, err
	}
	for _, m := range out {
		m.Body, m.BodyHTML = "", ""
	}
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	return out, r.fillHasAttachments(ctx, db, out)
}

func (r *OfficeMailRepo) ListRecentAddressFields(ctx context.Context, userID string, limit int) ([]string, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT sender, to_addrs, cc_addrs FROM office_mail_messages
		 WHERE user_id=? AND spam=FALSE ORDER BY created_at DESC LIMIT ?`), userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var a, b, c string
		if err := rows.Scan(&a, &b, &c); err != nil {
			return nil, err
		}
		for _, f := range []string{a, b, c} {
			if f != "" {
				out = append(out, f)
			}
		}
	}
	return out, rows.Err()
}

func (r *OfficeMailRepo) MarkViewSeen(ctx context.Context, userID, view, accountID string) ([]string, error) {
	db, err := r.writeDB(ctx)
	if err != nil {
		return nil, err
	}
	q := `UPDATE office_mail_messages SET seen=TRUE WHERE user_id=? AND seen=FALSE AND ` + mailViewWhere(view)
	args := []any{userID}
	if accountID != "" {
		q += ` AND account_id=?`
		args = append(args, accountID)
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q+` RETURNING id`), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *OfficeMailRepo) PurgeTrashedBefore(ctx context.Context, before time.Time) (int64, error) {
	db, err := r.writeDB(ctx)
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM office_mail_messages WHERE trashed_at IS NOT NULL AND trashed_at<?`), before)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *OfficeMailRepo) ListDueScheduledDrafts(ctx context.Context, now time.Time, limit int) ([]*office.MailMessage, error) {
	return r.queryMailRows(ctx,
		`SELECT `+officeMailMessageCols+` FROM office_mail_messages
		 WHERE folder='drafts' AND send_at IS NOT NULL AND send_at<=? AND trashed_at IS NULL
		 ORDER BY send_at LIMIT ?`, now, limit)
}

func (r *OfficeMailRepo) ClearDraftSendAt(ctx context.Context, id string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE office_mail_messages SET send_at=NULL WHERE id=?`), id)
	return err
}

func (r *OfficeMailRepo) ListBlockedSenders(ctx context.Context, userID string) ([]string, error) {
	db, err := r.readDB(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT address FROM office_mail_blocked WHERE user_id=? ORDER BY address`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// BlockSender 加进名单，并把这个发件人已有的、还在收件箱的信标成垃圾邮件（同一事务）。
func (r *OfficeMailRepo) BlockSender(ctx context.Context, userID, address string) (int64, error) {
	var marked int64
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		db := r.data.RWDB().WriteDB(txCtx)
		if db == nil {
			return apierror.Internal(office.DomainOffice, "repository unavailable")
		}
		if _, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
			`INSERT INTO office_mail_blocked (user_id, address) VALUES (?,?) ON CONFLICT DO NOTHING`), userID, address); err != nil {
			return err
		}
		res, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
			`UPDATE office_mail_messages SET spam=TRUE
			 WHERE user_id=? AND spam=FALSE AND folder IN ('inbox','junk') AND lower(sender) LIKE ?`),
			userID, "%"+escapeMailLike(address)+"%")
		if err != nil {
			return err
		}
		marked, _ = res.RowsAffected()
		return nil
	})
	return marked, err
}

func (r *OfficeMailRepo) UnblockSender(ctx context.Context, userID, address string) error {
	db, err := r.writeDB(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`DELETE FROM office_mail_blocked WHERE user_id=? AND address=?`), userID, address)
	return err
}
