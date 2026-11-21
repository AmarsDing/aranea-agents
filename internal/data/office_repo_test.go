package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/office"
)

// ensureOfficeTables 在隔离 schema 内建表——逐句镜像迁移
// 20261321_office_suite.sql（raw-SQL 表不经 ent Schema.Create；迁移文件改动
// 必须同步本镜像，列序钉死测试依赖不漂移）。
func ensureOfficeTables(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	db := d.RWDB().WriteDB(ctx)
	if db == nil {
		t.Fatal("test data rwdb unavailable")
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS office_events (
  "id"               TEXT PRIMARY KEY,
  "workspace_id"     TEXT NOT NULL DEFAULT '',
  "user_id"          TEXT NOT NULL,
  "task_ref"         TEXT NOT NULL DEFAULT '',
  "task_title"       TEXT NOT NULL DEFAULT '',
  "title"            TEXT NOT NULL,
  "note"             TEXT NOT NULL DEFAULT '',
  "start_at"         TIMESTAMPTZ NOT NULL,
  "end_at"           TIMESTAMPTZ NOT NULL,
  "timezone"         TEXT NOT NULL DEFAULT 'Asia/Shanghai',
  "reminder_minutes" INTEGER NOT NULL DEFAULT 0,
  "source"           TEXT NOT NULL DEFAULT 'manual',
  "reminded_at"      TIMESTAMPTZ,
  "created_at"       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at"       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 镜像 20261333_office_events_plan_log.sql
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "kind"    TEXT    NOT NULL DEFAULT 'plan';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "status"  TEXT    NOT NULL DEFAULT 'todo';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "all_day" BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "outcome" TEXT    NOT NULL DEFAULT '';
-- 镜像 20261339_office_events_more.sql
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "location" TEXT NOT NULL DEFAULT '';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "url"      TEXT NOT NULL DEFAULT '';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "color"    TEXT NOT NULL DEFAULT '';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "rrule"    TEXT NOT NULL DEFAULT '';
ALTER TABLE office_events ADD COLUMN IF NOT EXISTS "exdates"  TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_office_events_user_start ON office_events ("user_id", "start_at");
CREATE INDEX IF NOT EXISTS idx_office_events_task ON office_events ("task_ref") WHERE "task_ref" <> '';
CREATE INDEX IF NOT EXISTS idx_office_events_reminder ON office_events ("start_at")
  WHERE "reminder_minutes" > 0 AND "reminded_at" IS NULL;

CREATE TABLE IF NOT EXISTS office_notifications (
  "id"         TEXT PRIMARY KEY,
  "user_id"    TEXT NOT NULL,
  "kind"       TEXT NOT NULL,
  "title"      TEXT NOT NULL,
  "body"       TEXT NOT NULL DEFAULT '',
  "link"       TEXT NOT NULL DEFAULT '',
  "read"       BOOLEAN NOT NULL DEFAULT FALSE,
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_notifications_user ON office_notifications ("user_id", "read", "created_at" DESC);

CREATE TABLE IF NOT EXISTS office_ical_tokens (
  "id"         TEXT PRIMARY KEY,
  "user_id"    TEXT NOT NULL,
  "token_hash" TEXT NOT NULL UNIQUE,
  "name"       TEXT NOT NULL DEFAULT '',
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "revoked_at" TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_office_ical_tokens_user ON office_ical_tokens ("user_id");

CREATE TABLE IF NOT EXISTS office_apps (
  "id"           TEXT PRIMARY KEY,
  "workspace_id" TEXT NOT NULL DEFAULT '',
  "name"         TEXT NOT NULL,
  "url"          TEXT NOT NULL,
  "icon"         TEXT NOT NULL DEFAULT '',
  "group_name"   TEXT NOT NULL DEFAULT '',
  "sort_order"   INTEGER NOT NULL DEFAULT 0,
  "created_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_apps_ws ON office_apps ("workspace_id", "group_name", "sort_order");

CREATE TABLE IF NOT EXISTS office_schedule_drafts (
  "id"           TEXT PRIMARY KEY,
  "workspace_id" TEXT NOT NULL DEFAULT '',
  "user_id"      TEXT NOT NULL,
  "agent_id"     TEXT NOT NULL DEFAULT '',
  "session_id"   TEXT NOT NULL DEFAULT '',
  "status"       TEXT NOT NULL DEFAULT 'pending',
  "items_json"   TEXT NOT NULL,
  "decided_by"   TEXT NOT NULL DEFAULT '',
  "decide_note"  TEXT NOT NULL DEFAULT '',
  "decided_at"   TIMESTAMPTZ,
  "created_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_schedule_drafts_pending ON office_schedule_drafts ("workspace_id", "created_at" DESC)
  WHERE "status" = 'pending';
`)
	if err != nil {
		t.Fatalf("create office mirror DDL: %v", err)
	}
}

func openOfficeRepo(t *testing.T) *OfficeRepo {
	t.Helper()
	d := openTestData(t)
	ensureOfficeTables(t, d)
	return NewOfficeRepo(d)
}

func officeDay(hour int) time.Time {
	return time.Date(2026, 11, 10, hour, 0, 0, 0, time.UTC)
}

// TestOfficeRepo_EventCRUDAndOverlap 覆盖事件读写回路：Create→Get→Update→
// Delete（属主隔离在 SQL 内）+ 范围相交语义（start_at<窗尾 AND end_at>窗头，
// 贴边不算相交）+ CountOverlap（excludeID 自排除，409 双路径的仓储底座）。
func TestOfficeRepo_EventCRUDAndOverlap(t *testing.T) {
	t.Parallel()
	repo := openOfficeRepo(t)
	ctx := context.Background()

	ev := &office.Event{
		ID: "ev-1", WorkspaceID: "ws-1", UserID: "u-1",
		Title: "晨会", StartAt: officeDay(9), EndAt: officeDay(10),
		Timezone: "Asia/Shanghai", ReminderMinutes: 15, Source: "manual",
	}
	if err := repo.CreateEvent(ctx, ev); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	got, err := repo.GetEvent(ctx, "ev-1", "u-1")
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.Title != "晨会" || got.ReminderMinutes != 15 || got.Source != "manual" || got.RemindedAt != nil {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps must be stamped by repo: %+v", got)
	}
	// 属主隔离：他人不可见。
	if _, err := repo.GetEvent(ctx, "ev-1", "u-2"); !errors.Is(err, office.ErrEventNotFound) {
		t.Fatalf("cross-owner Get must be NotFound, got %v", err)
	}

	// 范围相交：窗 [8,10) 命中 [9,10)；贴边事件 [10,11) 不命中（start_at>=窗尾）。
	edge := &office.Event{ID: "ev-edge", UserID: "u-1", Title: "贴边", StartAt: officeDay(10), EndAt: officeDay(11), Timezone: "Asia/Shanghai", Source: "manual"}
	if err := repo.CreateEvent(ctx, edge); err != nil {
		t.Fatalf("CreateEvent edge: %v", err)
	}
	list, err := repo.ListEventsRange(ctx, "u-1", officeDay(8), officeDay(10))
	if err != nil {
		t.Fatalf("ListEventsRange: %v", err)
	}
	if len(list) != 1 || list[0].ID != "ev-1" {
		t.Fatalf("range intersection mismatch: %+v", list)
	}
	// 窗 [10,12) 只命中贴边事件。
	list, err = repo.ListEventsRange(ctx, "u-1", officeDay(10), officeDay(12))
	if err != nil || len(list) != 1 || list[0].ID != "ev-edge" {
		t.Fatalf("range right window mismatch: (%v,%+v)", err, list)
	}

	// 重叠计数：[9:30,10:30) 与两事件均相交？ev-1 [9,10) 相交；ev-edge [10,11)
	// 贴 9:30-10:30 内 → 2。排除 ev-1 后 → 1。
	n, err := repo.CountOverlap(ctx, "u-1", officeDay(9).Add(30*time.Minute), officeDay(10).Add(30*time.Minute), "")
	if err != nil || n != 2 {
		t.Fatalf("CountOverlap = (%d,%v), want 2", n, err)
	}
	n, err = repo.CountOverlap(ctx, "u-1", officeDay(9).Add(30*time.Minute), officeDay(10).Add(30*time.Minute), "ev-1")
	if err != nil || n != 1 {
		t.Fatalf("CountOverlap exclude = (%d,%v), want 1", n, err)
	}
	// 他人时间轴无重叠。
	n, err = repo.CountOverlap(ctx, "u-2", officeDay(9), officeDay(11), "")
	if err != nil || n != 0 {
		t.Fatalf("cross-owner CountOverlap = (%d,%v), want 0", n, err)
	}

	// task_ref 反查。
	tk := &office.Event{ID: "ev-task", UserID: "u-1", TaskRef: "task-9", TaskTitle: "需求评审", Title: "评审会", StartAt: officeDay(14), EndAt: officeDay(15), Timezone: "Asia/Shanghai", Source: "task_schedule"}
	if err := repo.CreateEvent(ctx, tk); err != nil {
		t.Fatalf("CreateEvent task: %v", err)
	}
	tlist, err := repo.ListTaskEvents(ctx, "u-1", "task-9")
	if err != nil || len(tlist) != 1 || tlist[0].TaskTitle != "需求评审" {
		t.Fatalf("ListTaskEvents mismatch: (%v,%+v)", err, tlist)
	}

	// Update：改时间窗 + reminder；他人更新 NotFound。
	got.StartAt = officeDay(9).Add(30 * time.Minute)
	got.EndAt = officeDay(10).Add(30 * time.Minute)
	got.ReminderMinutes = 5
	if err := repo.UpdateEvent(ctx, got); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	ug, err := repo.GetEvent(ctx, "ev-1", "u-1")
	if err != nil || ug.ReminderMinutes != 5 || !ug.StartAt.Equal(officeDay(9).Add(30*time.Minute)) {
		t.Fatalf("update roundtrip mismatch: (%v,%+v)", err, ug)
	}
	other := *got
	other.UserID = "u-2"
	if err := repo.UpdateEvent(ctx, &other); !errors.Is(err, office.ErrEventNotFound) {
		t.Fatalf("cross-owner Update must be NotFound, got %v", err)
	}

	if err := repo.DeleteEvent(ctx, "ev-1", "u-2"); !errors.Is(err, office.ErrEventNotFound) {
		t.Fatalf("cross-owner Delete must be NotFound, got %v", err)
	}
	if err := repo.DeleteEvent(ctx, "ev-1", "u-1"); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if _, err := repo.GetEvent(ctx, "ev-1", "u-1"); !errors.Is(err, office.ErrEventNotFound) {
		t.Fatalf("deleted Get must be NotFound, got %v", err)
	}
}

// TestOfficeRepo_ReminderDeliveryCAS 覆盖提醒投递事务：ListDueReminders 拾取
// 到期未提醒 → DeliverReminderTx 首次投递（事件置 reminded + 通知落库）→
// 重入（并发 tick/重复触发）CAS 丢失返回 (false,nil) 且不重复落通知。
func TestOfficeRepo_ReminderDeliveryCAS(t *testing.T) {
	t.Parallel()
	repo := openOfficeRepo(t)
	ctx := context.Background()
	now := officeDay(8).Add(50 * time.Minute)

	// 到期：9:00 开始、提前 15 分钟 → 8:45 到期（now=8:50 应拾取）。
	due := &office.Event{ID: "ev-due", UserID: "u-1", Title: "到期提醒", StartAt: officeDay(9), EndAt: officeDay(10), Timezone: "Asia/Shanghai", ReminderMinutes: 15, Source: "manual"}
	// 未到期：9:30 开始、提前 15 分 → 9:15 到期（now 不拾取）。
	notDue := &office.Event{ID: "ev-notdue", UserID: "u-1", Title: "未到期", StartAt: officeDay(9).Add(30 * time.Minute), EndAt: officeDay(10), Timezone: "Asia/Shanghai", ReminderMinutes: 15, Source: "manual"}
	// 无提醒配置不拾取。
	noRemind := &office.Event{ID: "ev-none", UserID: "u-1", Title: "无提醒", StartAt: officeDay(9), EndAt: officeDay(10), Timezone: "Asia/Shanghai", Source: "manual"}
	for _, ev := range []*office.Event{due, notDue, noRemind} {
		if err := repo.CreateEvent(ctx, ev); err != nil {
			t.Fatalf("CreateEvent %s: %v", ev.ID, err)
		}
	}
	dueList, err := repo.ListDueReminders(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListDueReminders: %v", err)
	}
	if len(dueList) != 1 || dueList[0].ID != "ev-due" {
		t.Fatalf("due pickup mismatch: %+v", dueList)
	}

	n := &office.Notification{ID: "ntf-1", UserID: "u-1", Kind: "schedule_reminder", Title: "到期提醒", Body: "9:00 开始", Link: "/schedule"}
	delivered, err := repo.DeliverReminderTx(ctx, due, n, now)
	if err != nil || !delivered {
		t.Fatalf("first deliver = (%v,%v), want (true,nil)", delivered, err)
	}
	got, err := repo.GetEvent(ctx, "ev-due", "u-1")
	if err != nil || got.RemindedAt == nil {
		t.Fatalf("reminded_at must be set: (%v,%+v)", err, got)
	}
	// 重入：CAS 丢失 → (false,nil)，通知不翻倍。
	n2 := &office.Notification{ID: "ntf-2", UserID: "u-1", Kind: "schedule_reminder", Title: "dup", Link: "/schedule"}
	delivered, err = repo.DeliverReminderTx(ctx, due, n2, now)
	if err != nil || delivered {
		t.Fatalf("redeliver = (%v,%v), want (false,nil)", delivered, err)
	}
	// 已投递事件不再被拾取。
	dueList, err = repo.ListDueReminders(ctx, now, 10)
	if err != nil || len(dueList) != 0 {
		t.Fatalf("delivered event must not be re-picked: (%v,%+v)", err, dueList)
	}
	rows, total, unread, err := repo.ListNotifications(ctx, "u-1", false, 1, 10)
	if err != nil || total != 1 || unread != 1 || len(rows) != 1 || rows[0].ID != "ntf-1" {
		t.Fatalf("notification side-effect mismatch: (%v,total=%d,unread=%d,%+v)", err, total, unread, rows)
	}
}

// TestOfficeRepo_Notifications 覆盖通知中心：unreadOnly 过滤、总数/未读数、
// 分页、Dismiss（属主隔离 + 幂等不存在 404）、MarkAllRead。
func TestOfficeRepo_Notifications(t *testing.T) {
	t.Parallel()
	repo := openOfficeRepo(t)
	ctx := context.Background()

	for _, n := range []*office.Notification{
		{ID: "n-1", UserID: "u-1", Kind: "schedule_reminder", Title: "a", Link: "/schedule"},
		{ID: "n-2", UserID: "u-1", Kind: "system", Title: "b"},
		{ID: "n-3", UserID: "u-2", Kind: "system", Title: "other-user"},
	} {
		if err := repo.CreateNotification(ctx, n); err != nil {
			t.Fatalf("CreateNotification %s: %v", n.ID, err)
		}
	}
	rows, total, unread, err := repo.ListNotifications(ctx, "u-1", false, 1, 10)
	if err != nil || total != 2 || unread != 2 || len(rows) != 2 {
		t.Fatalf("list = (%v,total=%d,unread=%d,len=%d)", err, total, unread, len(rows))
	}
	// 分页：pageSize=1 第二页。
	page2, total, _, err := repo.ListNotifications(ctx, "u-1", false, 2, 1)
	if err != nil || total != 2 || len(page2) != 1 {
		t.Fatalf("page2 = (%v,total=%d,len=%d)", err, total, len(page2))
	}
	// 未读过滤。
	if err := repo.DismissNotification(ctx, "n-1", "u-1"); err != nil {
		t.Fatalf("DismissNotification: %v", err)
	}
	unreadRows, _, unread, err := repo.ListNotifications(ctx, "u-1", true, 1, 10)
	if err != nil || unread != 1 || len(unreadRows) != 1 || unreadRows[0].ID != "n-2" {
		t.Fatalf("unread filter mismatch: (%v,unread=%d,%+v)", err, unread, unreadRows)
	}
	// 属主隔离。
	if err := repo.DismissNotification(ctx, "n-3", "u-1"); !errors.Is(err, office.ErrNotificationNotFound) {
		t.Fatalf("cross-owner dismiss must be NotFound, got %v", err)
	}
	// MarkAllRead 幂等。
	if err := repo.MarkAllNotificationsRead(ctx, "u-1"); err != nil {
		t.Fatalf("MarkAllNotificationsRead: %v", err)
	}
	_, _, unread, err = repo.ListNotifications(ctx, "u-1", true, 1, 10)
	if err != nil || unread != 0 {
		t.Fatalf("after markAll unread = (%d,%v), want 0", unread, err)
	}
}

// TestOfficeRepo_ICalTokenLifecycle 覆盖 ics 令牌：创建→解析（SHA-256 哈希
// 落库，明文不出现）→吊销→解析 401 语义（ErrICalTokenInvalid）→重复吊销
// 404（ErrICalTokenNotFound）→他人吊销 404。
func TestOfficeRepo_ICalTokenLifecycle(t *testing.T) {
	t.Parallel()
	repo := openOfficeRepo(t)
	ctx := context.Background()

	tok := &office.ICalToken{ID: "ic-1", UserID: "u-1", TokenHash: "hash-aaa", Name: "手机日历"}
	if err := repo.CreateICalToken(ctx, tok); err != nil {
		t.Fatalf("CreateICalToken: %v", err)
	}
	uid, err := repo.ResolveICalToken(ctx, "hash-aaa")
	if err != nil || uid != "u-1" {
		t.Fatalf("resolve = (%q,%v)", uid, err)
	}
	if _, err := repo.ResolveICalToken(ctx, "hash-nonexistent"); !errors.Is(err, office.ErrICalTokenInvalid) {
		t.Fatalf("unknown token must be Invalid, got %v", err)
	}
	toks, err := repo.ListICalTokens(ctx, "u-1")
	if err != nil || len(toks) != 1 || toks[0].RevokedAt != nil {
		t.Fatalf("list = (%v,%+v)", err, toks)
	}

	if err := repo.RevokeICalToken(ctx, "ic-1", "u-2"); !errors.Is(err, office.ErrICalTokenNotFound) {
		t.Fatalf("cross-owner revoke must be NotFound, got %v", err)
	}
	if err := repo.RevokeICalToken(ctx, "ic-1", "u-1"); err != nil {
		t.Fatalf("RevokeICalToken: %v", err)
	}
	if _, err := repo.ResolveICalToken(ctx, "hash-aaa"); !errors.Is(err, office.ErrICalTokenInvalid) {
		t.Fatalf("revoked token must be Invalid (feed 401), got %v", err)
	}
	if err := repo.RevokeICalToken(ctx, "ic-1", "u-1"); !errors.Is(err, office.ErrICalTokenNotFound) {
		t.Fatalf("re-revoke must be NotFound, got %v", err)
	}
	toks, err = repo.ListICalTokens(ctx, "u-1")
	if err != nil || len(toks) != 1 || toks[0].RevokedAt == nil {
		t.Fatalf("revoked list mismatch: (%v,%+v)", err, toks)
	}
}

// TestOfficeRepo_Apps 覆盖应用启动器：upsert 新建/更新同 id、列表按
// group_name+sort_order 排序、删除属主隔离 + 不存在 404。
func TestOfficeRepo_Apps(t *testing.T) {
	t.Parallel()
	repo := openOfficeRepo(t)
	ctx := context.Background()

	for _, a := range []*office.App{
		{ID: "app-1", WorkspaceID: "ws-1", Name: "Jira", URL: "https://jira.example.com", GroupName: "dev", SortOrder: 2},
		{ID: "app-2", WorkspaceID: "ws-1", Name: "Confluence", URL: "https://wiki.example.com", GroupName: "dev", SortOrder: 1},
		{ID: "app-3", WorkspaceID: "ws-1", Name: "Grafana", URL: "https://grafana.example.com", GroupName: "ops", SortOrder: 1},
	} {
		if err := repo.UpsertApp(ctx, a); err != nil {
			t.Fatalf("UpsertApp %s: %v", a.ID, err)
		}
	}
	apps, err := repo.ListApps(ctx, "ws-1")
	if err != nil || len(apps) != 3 {
		t.Fatalf("ListApps = (%v,len=%d)", err, len(apps))
	}
	// 排序钉死：group_name 升序 → 同组 sort_order 升序（ASCII 组名免 collation 歧义）。
	wantOrder := []string{"app-2", "app-1", "app-3"}
	for i, id := range wantOrder {
		if apps[i].ID != id {
			t.Fatalf("order mismatch at %d: got %v, want %v", i, apps[i].ID, id)
		}
	}
	// upsert 更新同 id。
	if err := repo.UpsertApp(ctx, &office.App{ID: "app-1", WorkspaceID: "ws-1", Name: "Jira Pro", URL: "https://jira2.example.com", GroupName: "dev", SortOrder: 3}); err != nil {
		t.Fatalf("UpsertApp update: %v", err)
	}
	apps, err = repo.ListApps(ctx, "ws-1")
	if err != nil || len(apps) != 3 {
		t.Fatalf("after upsert len = (%v,%d)", err, len(apps))
	}
	found := false
	for _, a := range apps {
		if a.ID == "app-1" {
			found = true
			if a.Name != "Jira Pro" || a.SortOrder != 3 {
				t.Fatalf("upsert update mismatch: %+v", a)
			}
		}
	}
	if !found {
		t.Fatal("app-1 missing after upsert")
	}
	// 工作区隔离。
	other, err := repo.ListApps(ctx, "ws-2")
	if err != nil || len(other) != 0 {
		t.Fatalf("workspace leak: (%v,%+v)", err, other)
	}
	if err := repo.DeleteApp(ctx, "app-1", "ws-2"); !errors.Is(err, office.ErrAppNotFound) {
		t.Fatalf("cross-workspace delete must be NotFound, got %v", err)
	}
	if err := repo.DeleteApp(ctx, "app-1", "ws-1"); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
}

// TestOfficeRepo_DraftApproveTx 覆盖排期草案审批事务（审批中心 schedule_draft
// 的仓储底座）：Create→ListPending/CountPending→Approve 单事务落事件（逐项
// 复检 409；source=agent_draft）→重复批准 409（ErrDraftNotPending）→冲突批准
// 整体回滚（草案仍 pending、事件不落）→Reject 幂等语义。
func TestOfficeRepo_DraftApproveTx(t *testing.T) {
	t.Parallel()
	repo := openOfficeRepo(t)
	ctx := context.Background()

	draft := &office.ScheduleDraft{
		ID: "dr-1", WorkspaceID: "ws-1", UserID: "u-1", AgentID: "ag-1", SessionID: "sess-1",
		Items: []office.DraftItem{
			{Title: "排期A", StartAt: officeDay(9), EndAt: officeDay(10), ReminderMinutes: 5, TaskRef: "task-1"},
			{Title: "排期B", StartAt: officeDay(11), EndAt: officeDay(12)},
		},
	}
	if err := repo.CreateDraft(ctx, draft); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	pending, err := repo.ListPendingDrafts(ctx, "ws-1", 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("ListPendingDrafts = (%v,len=%d)", err, len(pending))
	}
	if len(pending[0].Items) != 2 || pending[0].Items[0].Title != "排期A" || pending[0].Items[0].ReminderMinutes != 5 {
		t.Fatalf("items_json roundtrip mismatch: %+v", pending[0].Items)
	}
	cnt, err := repo.CountPendingDrafts(ctx, "ws-1")
	if err != nil || cnt != 1 {
		t.Fatalf("CountPendingDrafts = (%d,%v)", cnt, err)
	}
	// 工作区隔离。
	otherPending, err := repo.ListPendingDrafts(ctx, "ws-2", 10)
	if err != nil || len(otherPending) != 0 {
		t.Fatalf("workspace leak: (%v,%+v)", err, otherPending)
	}

	// 批准：两事件事务落库。
	events := []*office.Event{
		{ID: "ev-d1", WorkspaceID: "ws-1", UserID: "u-1", TaskRef: "task-1", Title: "排期A", StartAt: officeDay(9), EndAt: officeDay(10), Timezone: "Asia/Shanghai", ReminderMinutes: 5, Source: "agent_draft"},
		{ID: "ev-d2", WorkspaceID: "ws-1", UserID: "u-1", Title: "排期B", StartAt: officeDay(11), EndAt: officeDay(12), Timezone: "Asia/Shanghai", Source: "agent_draft"},
	}
	if err := repo.ApproveDraftTx(ctx, "dr-1", "boss-1", events); err != nil {
		t.Fatalf("ApproveDraftTx: %v", err)
	}
	got, err := repo.GetDraft(ctx, "dr-1")
	if err != nil || got.Status != office.DraftStatusApproved || got.DecidedBy != "boss-1" || got.DecidedAt == nil {
		t.Fatalf("approved draft mismatch: (%v,%+v)", err, got)
	}
	ev, err := repo.GetEvent(ctx, "ev-d1", "u-1")
	if err != nil || ev.Source != "agent_draft" || ev.TaskRef != "task-1" {
		t.Fatalf("approved event mismatch: (%v,%+v)", err, ev)
	}
	cnt, err = repo.CountPendingDrafts(ctx, "ws-1")
	if err != nil || cnt != 0 {
		t.Fatalf("pending after approve = (%d,%v)", cnt, err)
	}
	// 重复批准 409。
	if err := repo.ApproveDraftTx(ctx, "dr-1", "boss-1", events); !errors.Is(err, office.ErrDraftNotPending) {
		t.Fatalf("re-approve must be NotPending, got %v", err)
	}

	// 冲突批准：草案与既有事件重叠 → 409 整体回滚，草案仍 pending。
	conflictDraft := &office.ScheduleDraft{
		ID: "dr-2", WorkspaceID: "ws-1", UserID: "u-1",
		Items: []office.DraftItem{{Title: "冲突", StartAt: officeDay(9).Add(30 * time.Minute), EndAt: officeDay(10).Add(30 * time.Minute)}},
	}
	if err := repo.CreateDraft(ctx, conflictDraft); err != nil {
		t.Fatalf("CreateDraft conflict: %v", err)
	}
	conflictEvents := []*office.Event{
		{ID: "ev-c1", UserID: "u-1", Title: "冲突", StartAt: officeDay(9).Add(30 * time.Minute), EndAt: officeDay(10).Add(30 * time.Minute), Timezone: "Asia/Shanghai", Source: "agent_draft"},
	}
	if err := repo.ApproveDraftTx(ctx, "dr-2", "boss-1", conflictEvents); !errors.Is(err, office.ErrEventConflict) {
		t.Fatalf("conflict approve must be 409, got %v", err)
	}
	got, err = repo.GetDraft(ctx, "dr-2")
	if err != nil || got.Status != office.DraftStatusPending {
		t.Fatalf("conflict draft must stay pending: (%v,%+v)", err, got)
	}
	if _, err := repo.GetEvent(ctx, "ev-c1", "u-1"); !errors.Is(err, office.ErrEventNotFound) {
		t.Fatalf("conflict event must not land, got %v", err)
	}

	// 拒绝：状态翻转 + 决议元数据；重复拒绝 409；不存在 404。
	if err := repo.RejectDraft(ctx, "dr-2", "boss-1", "时间冲突，改期再报"); err != nil {
		t.Fatalf("RejectDraft: %v", err)
	}
	got, err = repo.GetDraft(ctx, "dr-2")
	if err != nil || got.Status != office.DraftStatusRejected || got.DecideNote != "时间冲突，改期再报" || got.DecidedAt == nil {
		t.Fatalf("rejected draft mismatch: (%v,%+v)", err, got)
	}
	if err := repo.RejectDraft(ctx, "dr-2", "boss-1", "again"); !errors.Is(err, office.ErrDraftNotPending) {
		t.Fatalf("re-reject must be NotPending, got %v", err)
	}
	if err := repo.RejectDraft(ctx, "dr-nonexistent", "boss-1", "x"); !errors.Is(err, office.ErrDraftNotFound) {
		t.Fatalf("missing draft reject must be NotFound, got %v", err)
	}
}

// TestOfficeRepo_RecurringAndSearch 重复系列：范围查询带回系列本身、不参与重叠计数、不进单次提醒扫描；搜索。
func TestOfficeRepo_RecurringAndSearch(t *testing.T) {
	t.Parallel()
	repo := openOfficeRepo(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 5, 1, 0, 0, 0, time.UTC)
	series := &office.Event{ID: "ev_rr", UserID: "u-r", Title: "站会", Location: "三楼", StartAt: start, EndAt: start.Add(15 * time.Minute),
		Timezone: "Asia/Shanghai", ReminderMinutes: 10, Source: office.SourceManual, RRule: "FREQ=DAILY", ExDates: "2026-01-06"}
	if err := repo.CreateEvent(ctx, series); err != nil {
		t.Fatal(err)
	}
	later := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	rows, err := repo.ListEventsRange(ctx, "u-r", later, later.Add(24*time.Hour))
	if err != nil || len(rows) != 1 || rows[0].RRule != "FREQ=DAILY" || rows[0].Location != "三楼" || rows[0].ExDates != "2026-01-06" {
		t.Fatalf("series in range: %+v err=%v", rows, err)
	}
	if n, _ := repo.CountOverlap(ctx, "u-r", start, start.Add(time.Hour), ""); n != 0 {
		t.Fatalf("recurring counted as overlap: %d", n)
	}
	if due, _ := repo.ListDueReminders(ctx, later, 10); len(due) != 0 {
		t.Fatalf("recurring in one-off reminder scan: %d", len(due))
	}
	if rec, _ := repo.ListRecurringReminders(ctx, 10); len(rec) == 0 {
		t.Fatal("recurring reminder scan empty")
	}
	if hits, _ := repo.SearchEvents(ctx, "u-r", "三楼", 10); len(hits) != 1 {
		t.Fatalf("search: %d", len(hits))
	}
}
