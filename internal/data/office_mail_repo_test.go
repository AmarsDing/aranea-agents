package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/office"
)

// ensureOfficeMailTables 在隔离 schema 内建表——逐句镜像迁移
// 20261322_office_mail.sql（raw-SQL 表不经 ent Schema.Create；迁移文件改动
// 必须同步本镜像，列序钉死测试依赖不漂移）。
func ensureOfficeMailTables(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	db := d.RWDB().WriteDB(ctx)
	if db == nil {
		t.Fatal("test data rwdb unavailable")
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS office_mail_accounts (
  "id"            TEXT PRIMARY KEY,
  "workspace_id"  TEXT NOT NULL DEFAULT '',
  "user_id"       TEXT NOT NULL,
  "name"          TEXT NOT NULL DEFAULT '',
  "host"          TEXT NOT NULL,
  "port"          INTEGER NOT NULL DEFAULT 993,
  "tls_mode"      TEXT NOT NULL DEFAULT 'tls',
  "username"      TEXT NOT NULL,
  "password_enc"  TEXT NOT NULL,
  "last_uid"      BIGINT NOT NULL DEFAULT 0,
  "last_sync_at"  TIMESTAMPTZ,
  "created_at"    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at"    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_mail_accounts_user ON office_mail_accounts ("user_id");

CREATE TABLE IF NOT EXISTS office_mail_messages (
  "id"             TEXT PRIMARY KEY,
  "workspace_id"   TEXT NOT NULL DEFAULT '',
  "user_id"        TEXT NOT NULL,
  "account_id"     TEXT NOT NULL DEFAULT '',
  "uid"            BIGINT NOT NULL DEFAULT 0,
  "subject"        TEXT NOT NULL DEFAULT '',
  "sender"         TEXT NOT NULL DEFAULT '',
  "body"           TEXT NOT NULL DEFAULT '',
  "snippet"        TEXT NOT NULL DEFAULT '',
  "received_at"    TIMESTAMPTZ,
  "classification" INTEGER NOT NULL DEFAULT 0,
  "quadrant"       TEXT NOT NULL DEFAULT '',
  "task_ref"       TEXT NOT NULL DEFAULT '',
  "created_at"     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS office_mail_messages_acc_uid
  ON office_mail_messages ("account_id", "uid") WHERE "account_id" <> '';
CREATE INDEX IF NOT EXISTS idx_office_mail_messages_user ON office_mail_messages ("user_id", "received_at" DESC NULLS LAST, "created_at" DESC);
CREATE INDEX IF NOT EXISTS idx_office_mail_messages_account ON office_mail_messages ("account_id", "uid");

CREATE TABLE IF NOT EXISTS office_mail_attachments (
  "id"                TEXT PRIMARY KEY,
  "message_id"        TEXT NOT NULL REFERENCES office_mail_messages("id") ON DELETE CASCADE,
  "filename"          TEXT NOT NULL DEFAULT '',
  "content_type"      TEXT NOT NULL DEFAULT '',
  "size"              BIGINT NOT NULL DEFAULT 0,
  "content"           BYTEA NOT NULL,
  "knowledge_doc_id"  TEXT NOT NULL DEFAULT '',
  "created_at"        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_mail_attachments_msg ON office_mail_attachments ("message_id");

CREATE TABLE IF NOT EXISTS office_tasks (
  "id"           TEXT PRIMARY KEY,
  "workspace_id" TEXT NOT NULL DEFAULT '',
  "user_id"      TEXT NOT NULL,
  "title"        TEXT NOT NULL,
  "body"         TEXT NOT NULL DEFAULT '',
  "source"       TEXT NOT NULL DEFAULT 'manual',
  "source_ref"   TEXT NOT NULL DEFAULT '',
  "status"       TEXT NOT NULL DEFAULT 'open',
  "created_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_tasks_user ON office_tasks ("user_id", "status", "created_at" DESC);
`)
	if err != nil {
		t.Fatalf("create office mail mirror DDL: %v", err)
	}
}

func openOfficeMailRepo(t *testing.T) *OfficeMailRepo {
	t.Helper()
	d := openTestData(t)
	ensureOfficeMailTables(t, d)
	return NewOfficeMailRepo(d)
}

func mailTestMessage(id, userID, accountID string, uid int64, subject string, receivedAt time.Time) *office.MailMessage {
	return &office.MailMessage{
		ID: id, WorkspaceID: "ws-1", UserID: userID, AccountID: accountID, UID: uid,
		Subject: subject, Sender: "a@example.com", Body: "正文 " + subject, Snippet: "正文 " + subject,
		ReceivedAt: &receivedAt, CreatedAt: time.Now(),
	}
}

// TestOfficeMailRepo_AccountCRUDAndCursor 账号回路：建/改/查/删（属主隔离）
// + 同步游标推进 + 全量扫描（IDLE supervisor 读口）。
func TestOfficeMailRepo_AccountCRUDAndCursor(t *testing.T) {
	t.Parallel()
	repo := openOfficeMailRepo(t)
	ctx := context.Background()

	acc := &office.MailAccount{
		ID: "omac_t1", WorkspaceID: "ws-1", UserID: "u-1", Name: "测试邮箱",
		Host: "imap.example.com", Port: 993, TLSMode: "tls", Username: "a@b.c",
		PasswordEnc: "enc:v1:cipher", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.CreateMailAccount(ctx, acc); err != nil {
		t.Fatalf("create account: %v", err)
	}
	got, err := repo.GetMailAccount(ctx, "omac_t1", "u-1")
	if err != nil || got.PasswordEnc != "enc:v1:cipher" {
		t.Fatalf("get account: %+v err=%v", got, err)
	}
	// 属主隔离。
	if _, err := repo.GetMailAccount(ctx, "omac_t1", "u-2"); !errors.Is(err, office.ErrMailAccountNotFound) {
		t.Fatalf("cross-user get: err=%v", err)
	}
	// 游标推进。
	syncAt := time.Now()
	if err := repo.UpdateMailSyncCursor(ctx, "omac_t1", 42, syncAt); err != nil {
		t.Fatalf("update cursor: %v", err)
	}
	got, _ = repo.GetMailAccount(ctx, "omac_t1", "u-1")
	if got.LastUID != 42 || got.LastSyncAt == nil {
		t.Fatalf("cursor last_uid=%d last_sync_at=%v", got.LastUID, got.LastSyncAt)
	}
	// 列表（本人/全量）。
	mine, err := repo.ListMailAccounts(ctx, "u-1")
	if err != nil || len(mine) != 1 {
		t.Fatalf("list mine: n=%d err=%v", len(mine), err)
	}
	all, err := repo.ListAllMailAccounts(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list all: n=%d err=%v", len(all), err)
	}
	// 更新 + 删除。
	acc.Name = "改名"
	if err := repo.UpdateMailAccount(ctx, acc); err != nil {
		t.Fatalf("update account: %v", err)
	}
	if err := repo.DeleteMailAccount(ctx, "omac_t1", "u-2"); !errors.Is(err, office.ErrMailAccountNotFound) {
		t.Fatalf("cross-user delete: err=%v", err)
	}
	if err := repo.DeleteMailAccount(ctx, "omac_t1", "u-1"); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
}

// TestOfficeMailRepo_InsertIdempotentAndImportExempt (account_id,uid) 部分
// 唯一索引：同步行幂等命中 inserted=false；.eml 导入行 account_id=” 豁免
// 可重复插入。
func TestOfficeMailRepo_InsertIdempotentAndImportExempt(t *testing.T) {
	t.Parallel()
	repo := openOfficeMailRepo(t)
	ctx := context.Background()
	now := time.Now()

	m := mailTestMessage("omm_1", "u-1", "omac_1", 7, "同步邮件", now)
	m.Attachments = []*office.MailAttachment{{
		ID: "omat_1", MessageID: "omm_1", Filename: "a.txt", ContentType: "text/plain",
		Size: 5, Content: []byte("hello"), CreatedAt: now,
	}}
	inserted, err := repo.InsertMessageWithAttachmentsTx(ctx, m)
	if err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v", inserted, err)
	}
	// 幂等命中。
	dup := mailTestMessage("omm_2", "u-1", "omac_1", 7, "同 UID 重复", now)
	inserted, err = repo.InsertMessageWithAttachmentsTx(ctx, dup)
	if err != nil || inserted {
		t.Fatalf("dup insert: inserted=%v err=%v, want false/nil", inserted, err)
	}
	// 导入行豁免（account_id='' 可重复）。
	imp1 := mailTestMessage("omm_i1", "u-1", "", 0, "导入一", now)
	imp2 := mailTestMessage("omm_i2", "u-1", "", 0, "导入二", now)
	if inserted, err = repo.InsertMessageWithAttachmentsTx(ctx, imp1); err != nil || !inserted {
		t.Fatalf("import 1: inserted=%v err=%v", inserted, err)
	}
	if inserted, err = repo.InsertMessageWithAttachmentsTx(ctx, imp2); err != nil || !inserted {
		t.Fatalf("import 2: inserted=%v err=%v", inserted, err)
	}

	// 附件随行入库；列表物化 has_attachments。
	got, err := repo.GetMailMessage(ctx, "omm_1", "u-1")
	if err != nil || !got.HasAttachments {
		t.Fatalf("get message: has_attachments=%v err=%v", got.HasAttachments, err)
	}
	atts, err := repo.ListMailAttachments(ctx, "omm_1")
	if err != nil || len(atts) != 1 || atts[0].Filename != "a.txt" {
		t.Fatalf("list attachments: %+v err=%v", atts, err)
	}
	full, err := repo.GetMailAttachment(ctx, "omat_1", "u-1")
	if err != nil || string(full.Content) != "hello" {
		t.Fatalf("get attachment content=%q err=%v", string(full.Content), err)
	}
	// JOIN 属主隔离。
	if _, err := repo.GetMailAttachment(ctx, "omat_1", "u-2"); !errors.Is(err, office.ErrMailAttachmentNotFound) {
		t.Fatalf("cross-user attachment: err=%v", err)
	}
}

// TestOfficeMailRepo_ListSortPagination 列表排序（received_at DESC NULLS
// LAST → created_at DESC）+ 分页 + 账号过滤 + total。
func TestOfficeMailRepo_ListSortPagination(t *testing.T) {
	t.Parallel()
	repo := openOfficeMailRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	// 三封：账号 A 两封（t+2h、t+1h），账号 B 一封（t+3h），另加一封
	// received_at NULL（走 created_at 排序键）。
	for _, m := range []*office.MailMessage{
		mailTestMessage("m-a1", "u-1", "acc-a", 1, "A1", base.Add(2*time.Hour)),
		mailTestMessage("m-a2", "u-1", "acc-a", 2, "A2", base.Add(1*time.Hour)),
		mailTestMessage("m-b1", "u-1", "acc-b", 1, "B1", base.Add(3*time.Hour)),
	} {
		if _, err := repo.InsertMessageWithAttachmentsTx(ctx, m); err != nil {
			t.Fatalf("insert %s: %v", m.ID, err)
		}
	}
	nullRecv := mailTestMessage("m-null", "u-1", "", 0, "无收件时间", base)
	nullRecv.ReceivedAt = nil
	if _, err := repo.InsertMessageWithAttachmentsTx(ctx, nullRecv); err != nil {
		t.Fatalf("insert null received: %v", err)
	}

	items, total, err := repo.ListMailMessages(ctx, "u-1", "", "", 1, 2)
	if err != nil || total != 4 || len(items) != 2 {
		t.Fatalf("page1: total=%d n=%d err=%v", total, len(items), err)
	}
	if items[0].ID != "m-b1" || items[1].ID != "m-a1" {
		t.Fatalf("page1 order: %s,%s want m-b1,m-a1", items[0].ID, items[1].ID)
	}
	items, total, err = repo.ListMailMessages(ctx, "u-1", "", "", 2, 2)
	if err != nil || len(items) != 2 || items[0].ID != "m-a2" || items[1].ID != "m-null" {
		t.Fatalf("page2 order: %+v", items)
	}
	// 账号过滤。
	items, total, err = repo.ListMailMessages(ctx, "u-1", "acc-a", "", 1, 50)
	if err != nil || total != 2 || items[0].ID != "m-a1" {
		t.Fatalf("account filter: total=%d first=%s", total, items[0].ID)
	}
	// 他用户空集。
	if _, total, _ := repo.ListMailMessages(ctx, "u-2", "", "", 1, 50); total != 0 {
		t.Fatalf("cross-user total=%d", total)
	}
}

// TestOfficeMailRepo_DeleteCascade 删除邮件级联附件；属主隔离。
func TestOfficeMailRepo_DeleteCascade(t *testing.T) {
	t.Parallel()
	repo := openOfficeMailRepo(t)
	ctx := context.Background()
	now := time.Now()

	m := mailTestMessage("omm_d1", "u-1", "acc-d", 1, "待删", now)
	m.Attachments = []*office.MailAttachment{{
		ID: "omat_d1", MessageID: "omm_d1", Filename: "x.bin", ContentType: "application/octet-stream",
		Size: 2, Content: []byte("xy"), CreatedAt: now,
	}}
	if _, err := repo.InsertMessageWithAttachmentsTx(ctx, m); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := repo.DeleteMailMessage(ctx, "omm_d1", "u-2"); !errors.Is(err, office.ErrMailMessageNotFound) {
		t.Fatalf("cross-user delete: err=%v", err)
	}
	if err := repo.DeleteMailMessage(ctx, "omm_d1", "u-1"); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if _, err := repo.GetMailMessage(ctx, "omm_d1", "u-1"); !errors.Is(err, office.ErrMailMessageNotFound) {
		t.Fatalf("get after delete: err=%v", err)
	}
	atts, err := repo.ListMailAttachments(ctx, "omm_d1")
	if err != nil || len(atts) != 0 {
		t.Fatalf("cascade attachments=%d err=%v", len(atts), err)
	}
}

// TestOfficeMailRepo_TaskFromMailTx 转任务同事务：建任务 + task_ref 回写；
// 邮件不存在整体失败（无孤儿任务）。
func TestOfficeMailRepo_TaskFromMailTx(t *testing.T) {
	t.Parallel()
	repo := openOfficeMailRepo(t)
	ctx := context.Background()
	now := time.Now()

	if _, err := repo.InsertMessageWithAttachmentsTx(ctx, mailTestMessage("omm_tk1", "u-1", "acc-t", 1, "转任务源", now)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	task := &office.OfficeTask{
		ID: "otk_1", WorkspaceID: "ws-1", UserID: "u-1", Title: "邮件：转任务源",
		Body: "来自邮件：omm_tk1\n\n正文", Source: "mail", SourceRef: "omm_tk1",
		Status: "open", CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskFromMailTx(ctx, task, "omm_tk1", "u-1"); err != nil {
		t.Fatalf("create task from mail: %v", err)
	}
	got, err := repo.GetOfficeTask(ctx, "otk_1", "u-1")
	if err != nil || got.SourceRef != "omm_tk1" {
		t.Fatalf("get task: %+v err=%v", got, err)
	}
	m, _ := repo.GetMailMessage(ctx, "omm_tk1", "u-1")
	if m.TaskRef != "otk_1" {
		t.Fatalf("task_ref=%q", m.TaskRef)
	}
	// 邮件不存在 → 整体失败，任务不落。
	task2 := &office.OfficeTask{ID: "otk_2", UserID: "u-1", Title: "孤儿", Source: "mail", SourceRef: "omm_ghost", Status: "open", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateTaskFromMailTx(ctx, task2, "omm_ghost", "u-1"); !errors.Is(err, office.ErrMailMessageNotFound) {
		t.Fatalf("ghost mail tx: err=%v", err)
	}
	if _, err := repo.GetOfficeTask(ctx, "otk_2", "u-1"); err == nil {
		t.Fatal("orphan task should not exist")
	}
	// SetMailTaskRef 单写。
	if err := repo.SetMailTaskRef(ctx, "omm_tk1", "u-1", "otk_1"); err != nil {
		t.Fatalf("set task ref: %v", err)
	}
}

// TestOfficeMailRepo_DigestAndSearch 摘要窗口扫描 + 候选人 + 关键词检索。
func TestOfficeMailRepo_DigestAndSearch(t *testing.T) {
	t.Parallel()
	repo := openOfficeMailRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	for _, m := range []*office.MailMessage{
		mailTestMessage("d-1", "u-1", "acc-d", 1, "窗口内一", base),
		mailTestMessage("d-2", "u-1", "acc-d", 2, "窗口内二", base.Add(time.Hour)),
		mailTestMessage("d-3", "u-2", "acc-e", 1, "他人的资金周报", base),
		mailTestMessage("d-old", "u-1", "acc-d", 3, "窗口外旧件", base.Add(-48*time.Hour)),
	} {
		if _, err := repo.InsertMessageWithAttachmentsTx(ctx, m); err != nil {
			t.Fatalf("insert %s: %v", m.ID, err)
		}
	}
	since := base.Add(-time.Hour)

	// 摘要窗口（本人、升序）。
	msgs, err := repo.ListMailMessagesSince(ctx, "u-1", since, 200)
	if err != nil || len(msgs) != 2 || msgs[0].ID != "d-1" {
		t.Fatalf("since list: n=%d first=%v err=%v", len(msgs), msgs[0].ID, err)
	}
	// 候选人（窗口内有邮件的用户去重）。
	cands, err := repo.ListMailDigestUsers(ctx, since)
	if err != nil || len(cands) != 2 {
		t.Fatalf("candidates: %+v err=%v", cands, err)
	}
	// 检索（subject/sender/body ILIKE + 属主隔离）。
	hits, err := repo.SearchMailMessages(ctx, "u-1", "窗口内", 10)
	if err != nil || len(hits) != 2 {
		t.Fatalf("search: n=%d err=%v", len(hits), err)
	}
	hits, _ = repo.SearchMailMessages(ctx, "u-1", "资金", 10)
	if len(hits) != 0 {
		t.Fatalf("cross-user search leak: n=%d", len(hits))
	}
	hits, _ = repo.SearchMailMessages(ctx, "u-2", "资金", 10)
	if len(hits) != 1 {
		t.Fatalf("u-2 search: n=%d", len(hits))
	}
}
