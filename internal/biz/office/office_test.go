package office

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// 内存 fakes（窄端口测试桩；仅承载用例所需语义，不做完整并发正确性——
// 并发/CAS 正确性由 internal/data PG 集成测试钉死）。
// ============================================================================

type fakeEventRepo struct {
	byID      map[string]*Event
	countHook func(userID string, start, end time.Time, excludeID string) int
}

func newFakeEventRepo() *fakeEventRepo { return &fakeEventRepo{byID: map[string]*Event{}} }

func (f *fakeEventRepo) CreateEvent(_ context.Context, ev *Event) error {
	cp := *ev
	f.byID[ev.ID] = &cp
	return nil
}

func (f *fakeEventRepo) UpdateEvent(_ context.Context, ev *Event) error {
	if _, ok := f.byID[ev.ID]; !ok {
		return ErrEventNotFound
	}
	cp := *ev
	f.byID[ev.ID] = &cp
	return nil
}

func (f *fakeEventRepo) DeleteEvent(_ context.Context, id, userID string) error {
	ev, ok := f.byID[id]
	if !ok || ev.UserID != userID {
		return ErrEventNotFound
	}
	delete(f.byID, id)
	return nil
}

func (f *fakeEventRepo) GetEvent(_ context.Context, id, userID string) (*Event, error) {
	ev, ok := f.byID[id]
	if !ok || ev.UserID != userID {
		return nil, ErrEventNotFound
	}
	cp := *ev
	return &cp, nil
}

func overlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return !(aEnd.Compare(bStart) <= 0 || aStart.Compare(bEnd) >= 0)
}

func (f *fakeEventRepo) ListEventsRange(_ context.Context, userID string, start, end time.Time) ([]*Event, error) {
	var out []*Event
	for _, ev := range f.byID {
		if ev.UserID == userID && overlap(ev.StartAt, ev.EndAt, start, end) {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (f *fakeEventRepo) ListTaskEvents(_ context.Context, userID, taskRef string) ([]*Event, error) {
	var out []*Event
	for _, ev := range f.byID {
		if ev.UserID == userID && ev.TaskRef == taskRef {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (f *fakeEventRepo) CountOverlap(_ context.Context, userID string, start, end time.Time, excludeID string) (int, error) {
	if f.countHook != nil {
		return f.countHook(userID, start, end, excludeID), nil
	}
	n := 0
	for _, ev := range f.byID {
		if ev.UserID != userID || ev.ID == excludeID {
			continue
		}
		if overlap(ev.StartAt, ev.EndAt, start, end) {
			n++
		}
	}
	return n, nil
}

func (f *fakeEventRepo) ListDueReminders(_ context.Context, now time.Time, _ int) ([]*Event, error) {
	var out []*Event
	for _, ev := range f.byID {
		if ev.ReminderMinutes > 0 && ev.RemindedAt == nil &&
			!ev.StartAt.Add(-time.Duration(ev.ReminderMinutes)*time.Minute).After(now) {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (f *fakeEventRepo) MarkReminded(_ context.Context, id string, at time.Time) error {
	if ev, ok := f.byID[id]; ok && ev.RemindedAt == nil {
		ev.RemindedAt = &at
	}
	return nil
}

type fakeDraftRepo struct {
	byID           map[string]*ScheduleDraft
	approveCalls   int
	approveErr     error
	approveEvents  []*Event
	rejectNoteSeen string
}

func newFakeDraftRepo() *fakeDraftRepo { return &fakeDraftRepo{byID: map[string]*ScheduleDraft{}} }

func (f *fakeDraftRepo) CreateDraft(_ context.Context, d *ScheduleDraft) error {
	f.byID[d.ID] = d
	return nil
}

func (f *fakeDraftRepo) GetDraft(_ context.Context, id string) (*ScheduleDraft, error) {
	d, ok := f.byID[id]
	if !ok {
		return nil, ErrDraftNotFound
	}
	return d, nil
}

func (f *fakeDraftRepo) ListPendingDrafts(_ context.Context, workspaceID string, _ int) ([]*ScheduleDraft, error) {
	var out []*ScheduleDraft
	for _, d := range f.byID {
		if d.WorkspaceID == workspaceID && d.Status == DraftStatusPending {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeDraftRepo) CountPendingDrafts(_ context.Context, workspaceID string) (int64, error) {
	var n int64
	for _, d := range f.byID {
		if d.WorkspaceID == workspaceID && d.Status == DraftStatusPending {
			n++
		}
	}
	return n, nil
}

func (f *fakeDraftRepo) ApproveDraftTx(_ context.Context, draftID, decidedBy string, events []*Event) error {
	f.approveCalls++
	f.approveEvents = events
	if f.approveErr != nil {
		return f.approveErr
	}
	d := f.byID[draftID]
	if d == nil {
		return ErrDraftNotFound
	}
	if d.Status != DraftStatusPending {
		return ErrDraftNotPending
	}
	d.Status = DraftStatusApproved
	d.DecidedBy = decidedBy
	return nil
}

func (f *fakeDraftRepo) RejectDraft(_ context.Context, draftID, decidedBy, note string) error {
	d := f.byID[draftID]
	if d == nil {
		return ErrDraftNotFound
	}
	if d.Status != DraftStatusPending {
		return ErrDraftNotPending
	}
	d.Status = DraftStatusRejected
	d.DecidedBy = decidedBy
	d.DecideNote = note
	f.rejectNoteSeen = note
	return nil
}

type fakeTokenRepo struct {
	byID    map[string]*ICalToken
	byHash  map[string]*ICalToken
	revoked map[string]bool
}

func newFakeTokenRepo() *fakeTokenRepo {
	return &fakeTokenRepo{byID: map[string]*ICalToken{}}
}

func (f *fakeTokenRepo) init() {
	if f.byHash == nil {
		f.byHash = map[string]*ICalToken{}
	}
	if f.revoked == nil {
		f.revoked = map[string]bool{}
	}
}

func (f *fakeTokenRepo) CreateICalToken(_ context.Context, t *ICalToken) error {
	f.init()
	f.byID[t.ID] = t
	f.byHash[t.TokenHash] = t
	return nil
}

func (f *fakeTokenRepo) RevokeICalToken(_ context.Context, id, userID string) error {
	f.init()
	t, ok := f.byID[id]
	if !ok || t.UserID != userID || f.revoked[id] {
		return ErrICalTokenNotFound
	}
	f.revoked[id] = true
	now := time.Now()
	t.RevokedAt = &now
	return nil
}

func (f *fakeTokenRepo) ListICalTokens(_ context.Context, userID string) ([]*ICalToken, error) {
	f.init()
	var out []*ICalToken
	for _, t := range f.byID {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeTokenRepo) ResolveICalToken(_ context.Context, tokenHash string) (string, error) {
	f.init()
	t, ok := f.byHash[tokenHash]
	if !ok || f.revoked[t.ID] {
		return "", ErrICalTokenInvalid
	}
	return t.UserID, nil
}

type fakeAppRepo struct{ apps map[string]*App }

func (f *fakeAppRepo) UpsertApp(_ context.Context, a *App) error {
	if f.apps == nil {
		f.apps = map[string]*App{}
	}
	f.apps[a.ID] = a
	return nil
}
func (f *fakeAppRepo) DeleteApp(_ context.Context, id, workspaceID string) error {
	if a, ok := f.apps[id]; ok && a.WorkspaceID == workspaceID {
		delete(f.apps, id)
		return nil
	}
	return ErrAppNotFound
}
func (f *fakeAppRepo) ListApps(_ context.Context, workspaceID string) ([]*App, error) {
	var out []*App
	for _, a := range f.apps {
		if a.WorkspaceID == workspaceID {
			out = append(out, a)
		}
	}
	return out, nil
}

type fakeReminderStore struct {
	deliveries []*Notification
	delivered  bool // 模拟并发 tick 已置标
}

func (f *fakeReminderStore) DeliverReminderTx(_ context.Context, _ *Event, n *Notification, _ time.Time) (bool, error) {
	if !f.delivered {
		return false, nil
	}
	f.deliveries = append(f.deliveries, n)
	return true, nil
}

type fakeDelivery struct {
	sinks    []ReminderSink
	enqueued []struct {
		sink ReminderSink
		text string
		key  string
	}
	listErr error
}

func (f *fakeDelivery) ListSinks(_ context.Context, _ string) ([]ReminderSink, error) {
	return f.sinks, f.listErr
}
func (f *fakeDelivery) Enqueue(_ context.Context, sink ReminderSink, text, idempotencyKey string) error {
	f.enqueued = append(f.enqueued, struct {
		sink ReminderSink
		text string
		key  string
	}{sink, text, idempotencyKey})
	return nil
}

func day(hour int) time.Time { return time.Date(2026, 11, 10, hour, 0, 0, 0, time.UTC) }

// ============================================================================
// ScheduleUsecase
// ============================================================================

// TestScheduleUsecase_CreateConflictAndValidation 平移 workbench 验收 S4：
// 手工创建 409（同人同时段重叠）+ 端点校验（空标题/end<=start）。
func TestScheduleUsecase_CreateConflictAndValidation(t *testing.T) {
	repo := newFakeEventRepo()
	uc := NewScheduleUsecase(repo)
	ctx := context.Background()

	first, err := uc.CreateEvent(ctx, &Event{
		UserID: "u-1", Title: "A", StartAt: day(9), EndAt: day(10),
	})
	if err != nil || first.Source != SourceManual || first.Timezone != DefaultTimezone || !strings.HasPrefix(first.ID, "ev_") {
		t.Fatalf("create first = (%+v,%v)", first, err)
	}
	// 重叠 → 409。
	_, err = uc.CreateEvent(ctx, &Event{UserID: "u-1", Title: "B", StartAt: day(9).Add(30 * time.Minute), EndAt: day(11)})
	if !errors.Is(err, ErrEventConflict) {
		t.Fatalf("overlap create must be 409, got %v", err)
	}
	// 贴边不冲突。
	if _, err := uc.CreateEvent(ctx, &Event{UserID: "u-1", Title: "C", StartAt: day(10), EndAt: day(11)}); err != nil {
		t.Fatalf("adjacent must pass, got %v", err)
	}
	// 他人同窗不冲突。
	if _, err := uc.CreateEvent(ctx, &Event{UserID: "u-2", Title: "D", StartAt: day(9), EndAt: day(10)}); err != nil {
		t.Fatalf("other user same window must pass, got %v", err)
	}
	// 校验：空标题 400、end<=start 400。
	if _, err := uc.CreateEvent(ctx, &Event{UserID: "u-3", Title: " ", StartAt: day(12), EndAt: day(13)}); !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("empty title must be 400, got %v", err)
	}
	if _, err := uc.CreateEvent(ctx, &Event{UserID: "u-3", Title: "x", StartAt: day(13), EndAt: day(13)}); !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("end<=start must be 400, got %v", err)
	}
}

// TestScheduleUsecase_UpdateSelfExclude 更新路径 409 排除自身：只改标题不动
// 时间不触发重叠；改到与他人事件重叠的窗口 → 409；改时间重置 reminded_at。
func TestScheduleUsecase_UpdateSelfExclude(t *testing.T) {
	repo := newFakeEventRepo()
	uc := NewScheduleUsecase(repo)
	ctx := context.Background()

	a, err := uc.CreateEvent(ctx, &Event{UserID: "u-1", Title: "A", StartAt: day(9), EndAt: day(10)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.CreateEvent(ctx, &Event{UserID: "u-1", Title: "B", StartAt: day(11), EndAt: day(12)}); err != nil {
		t.Fatal(err)
	}
	// 仅改标题（时间不动 → 不复检）。
	if _, err := uc.UpdateEvent(ctx, "u-1", &Event{ID: a.ID, Title: "A2"}); err != nil {
		t.Fatalf("title-only update must pass, got %v", err)
	}
	// 移到与 B 重叠 → 409。
	if _, err := uc.UpdateEvent(ctx, "u-1", &Event{ID: a.ID, StartAt: day(11).Add(30 * time.Minute), EndAt: day(12).Add(30 * time.Minute)}); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("overlapping move must be 409, got %v", err)
	}
	// 移到空窗成功；reminded_at 重置。
	reminded := day(8)
	a2, err := uc.GetEvent(ctx, a.ID, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	a2.RemindedAt = &reminded
	if err := repo.UpdateEvent(ctx, a2); err != nil {
		t.Fatal(err)
	}
	moved, err := uc.UpdateEvent(ctx, "u-1", &Event{ID: a.ID, StartAt: day(14), EndAt: day(15), ReminderMinutes: 10})
	if err != nil {
		t.Fatalf("move to free window: %v", err)
	}
	if moved.RemindedAt != nil {
		t.Fatal("time/reminder change must reset reminded_at")
	}
}

// TestScheduleUsecase_ScheduleTask 平移 workbench 任务排期：标题自动「执行：」
// 前缀、task_ref/task_title 落位、重叠 409、反查。
func TestScheduleUsecase_ScheduleTask(t *testing.T) {
	repo := newFakeEventRepo()
	uc := NewScheduleUsecase(repo)
	ctx := context.Background()

	ev, err := uc.ScheduleTask(ctx, "ws-1", "u-1", "task-1", "写周报", day(16), day(17), 30)
	if err != nil {
		t.Fatalf("ScheduleTask: %v", err)
	}
	if ev.Title != "执行：写周报" || ev.Source != SourceTaskSchedule || ev.TaskRef != "task-1" || ev.TaskTitle != "写周报" || ev.WorkspaceID != "ws-1" {
		t.Fatalf("schedule task event mismatch: %+v", ev)
	}
	if _, err := uc.ScheduleTask(ctx, "ws-1", "u-1", "task-2", "冲突任务", day(16).Add(30*time.Minute), day(17), 0); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("overlapping task schedule must be 409, got %v", err)
	}
	if _, err := uc.ScheduleTask(ctx, "ws-1", "u-1", "", "x", day(18), day(19), 0); !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("empty taskRef must be rejected, got %v", err)
	}
	back, err := uc.ListTaskEvents(ctx, "u-1", "task-1")
	if err != nil || len(back) != 1 || back[0].ID != ev.ID {
		t.Fatalf("ListTaskEvents mismatch: (%v,%+v)", err, back)
	}
	// 范围聚合端点校验。
	if _, err := uc.ListEventsRange(ctx, "u-1", day(10), day(10)); !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("invalid range must be 400, got %v", err)
	}
}

// ============================================================================
// DraftUsecase
// ============================================================================

// TestDraftUsecase_SubmitAndApprove 草案链路：提交仅形状校验（不做冲突检测）→
// 批准转换为 source=agent_draft 事件交仓储事务；拒绝必填理由（与 review.v1
// 同约）。
func TestDraftUsecase_SubmitAndApprove(t *testing.T) {
	drafts := newFakeDraftRepo()
	uc := NewDraftUsecase(drafts)
	ctx := context.Background()

	// 空草案 400。
	if _, err := uc.SubmitDraft(ctx, "ws-1", "u-1", "ag-1", "sess-1", nil); !errors.Is(err, ErrDraftEmpty) {
		t.Fatalf("empty draft must be 400, got %v", err)
	}
	// 条目形状校验（end<=start）。
	if _, err := uc.SubmitDraft(ctx, "ws-1", "u-1", "ag-1", "sess-1", []DraftItem{{Title: "x", StartAt: day(10), EndAt: day(10)}}); !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("bad item window must be 400, got %v", err)
	}
	// 提交不做冲突检测：与任何既有事件重叠也先落草案（批准时点复检）。
	d, err := uc.SubmitDraft(ctx, "ws-1", "u-1", "ag-1", "sess-1", []DraftItem{
		{Title: "排期A", StartAt: day(9), EndAt: day(10), ReminderMinutes: 5, TaskRef: "task-1"},
		{Title: "排期B", StartAt: day(11), EndAt: day(12)},
	})
	if err != nil {
		t.Fatalf("SubmitDraft: %v", err)
	}
	if d.Status != DraftStatusPending || !strings.HasPrefix(d.ID, "sd_") || d.AgentID != "ag-1" || d.SessionID != "sess-1" {
		t.Fatalf("draft shape mismatch: %+v", d)
	}
	if n, _ := drafts.CountPendingDrafts(ctx, "ws-1"); n != 1 {
		t.Fatalf("pending count = %d, want 1", n)
	}

	// 批准：事件转换语义钉死（属主=提交人、source=agent_draft、task_ref 透传）。
	if err := uc.ApproveDraft(ctx, d.ID, "boss-1"); err != nil {
		t.Fatalf("ApproveDraft: %v", err)
	}
	if drafts.approveCalls != 1 || len(drafts.approveEvents) != 2 {
		t.Fatalf("approve tx call mismatch: calls=%d events=%d", drafts.approveCalls, len(drafts.approveEvents))
	}
	ev := drafts.approveEvents[0]
	if ev.UserID != "u-1" || ev.WorkspaceID != "ws-1" || ev.Source != SourceAgentDraft || ev.TaskRef != "task-1" || ev.ReminderMinutes != 5 || !strings.HasPrefix(ev.ID, "ev_") {
		t.Fatalf("approved event conversion mismatch: %+v", ev)
	}
	if got := drafts.byID[d.ID]; got.Status != DraftStatusApproved || got.DecidedBy != "boss-1" {
		t.Fatalf("draft status after approve: %+v", got)
	}
	// 重复批准 409。
	if err := uc.ApproveDraft(ctx, d.ID, "boss-1"); !errors.Is(err, ErrDraftNotPending) {
		t.Fatalf("re-approve must be 409, got %v", err)
	}
	if drafts.approveCalls != 1 {
		t.Fatal("re-approve must not reach repo tx")
	}
	// 批准冲突回滚透传（仓储 409 原样上浮）。
	drafts.approveErr = ErrEventConflict
	d2, err := uc.SubmitDraft(ctx, "ws-1", "u-1", "ag-1", "sess-1", []DraftItem{{Title: "冲突", StartAt: day(9), EndAt: day(10)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := uc.ApproveDraft(ctx, d2.ID, "boss-1"); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflict approve must surface 409, got %v", err)
	}
	drafts.approveErr = nil

	// 拒绝：理由必填 400；正常拒绝留决议元数据、不落事件。
	if err := uc.RejectDraft(ctx, d2.ID, "boss-1", "  "); !errors.Is(err, ErrRejectNoteRequired) {
		t.Fatalf("empty reject note must be 400, got %v", err)
	}
	if err := uc.RejectDraft(ctx, d2.ID, "boss-1", "时段撞车"); err != nil {
		t.Fatalf("RejectDraft: %v", err)
	}
	if got := drafts.byID[d2.ID]; got.Status != DraftStatusRejected || got.DecideNote != "时段撞车" {
		t.Fatalf("rejected draft mismatch: %+v", got)
	}
	if drafts.approveCalls != 2 {
		t.Fatalf("reject must not reach approve tx (calls=%d, want 2)", drafts.approveCalls)
	}
}

// ============================================================================
// ReminderWorker
// ============================================================================

// TestReminderWorker_Tick 单 tick 端到端（fakes）：到期事件 → IM 幂等入队
// （键 sched-rem-<eventID>-<channelID>）→ 通知落表；并发 tick 已置标时
// (false,nil) 不报错且通知不翻倍；IM 无声明 sink 时仅通知中心。
func TestReminderWorker_Tick(t *testing.T) {
	events := newFakeEventRepo()
	store := &fakeReminderStore{delivered: true}
	im := &fakeDelivery{sinks: []ReminderSink{{ChannelID: "ch-1", Platform: "wecom", Recipient: "u-1"}}}
	w := NewReminderWorker(events, store, im, nil)
	now := day(8).Add(50 * time.Minute)
	w.now = func() time.Time { return now }
	ctx := context.Background()

	// 到期 + 未到期 + 无提醒配置 各一。
	events.byID["ev-due"] = &Event{ID: "ev-due", WorkspaceID: "ws-1", UserID: "u-1", Title: "评审会", StartAt: day(9), EndAt: day(10), Timezone: "Asia/Shanghai", ReminderMinutes: 15}
	events.byID["ev-later"] = &Event{ID: "ev-later", UserID: "u-1", Title: "未到期", StartAt: day(20), EndAt: day(21), ReminderMinutes: 15}
	events.byID["ev-norm"] = &Event{ID: "ev-norm", UserID: "u-1", Title: "无提醒", StartAt: day(9), EndAt: day(10)}

	w.tick(ctx)

	if len(im.enqueued) != 1 {
		t.Fatalf("im enqueues = %d, want 1", len(im.enqueued))
	}
	enq := im.enqueued[0]
	if enq.key != "sched-rem-ev-due-ch-1" {
		t.Fatalf("idempotency key mismatch: %q", enq.key)
	}
	if !strings.Contains(enq.text, "评审会") || !strings.Contains(enq.text, "时间：") {
		t.Fatalf("reminder text mismatch: %q", enq.text)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("inbox deliveries = %d, want 1", len(store.deliveries))
	}
	n := store.deliveries[0]
	if n.Kind != NotifyKindScheduleReminder || n.UserID != "u-1" || n.Link != "/schedule" || !strings.HasPrefix(n.ID, "ntf_") {
		t.Fatalf("notification shape mismatch: %+v", n)
	}

	// 并发 tick 已置标：(false,nil) → 不报错、通知不翻倍（IM 腿可能重入，
	// 幂等键去重——fake 不去重，故记录第二次入队也合法；收件箱腿是恰好一次
	// 的判定点）。
	store.delivered = false
	w.tick(ctx)
	if len(store.deliveries) != 1 {
		t.Fatalf("lost-CAS tick must not add inbox rows, got %d", len(store.deliveries))
	}

	// 无 IM sink 声明：仅通知中心。
	events2 := newFakeEventRepo()
	events2.byID["ev-x"] = &Event{ID: "ev-x", UserID: "u-9", Title: "x", StartAt: day(9), EndAt: day(10), ReminderMinutes: 15}
	store2 := &fakeReminderStore{delivered: true}
	w2 := NewReminderWorker(events2, store2, &fakeDelivery{}, nil)
	w2.now = func() time.Time { return now }
	w2.tick(ctx)
	if len(store2.deliveries) != 1 {
		t.Fatalf("no-sink tick deliveries = %d, want 1", len(store2.deliveries))
	}

	// sink 列表失败：单条不阻断、通知不落（下一 tick 重试）。
	events3 := newFakeEventRepo()
	events3.byID["ev-y"] = &Event{ID: "ev-y", UserID: "u-9", Title: "y", StartAt: day(9), EndAt: day(10), ReminderMinutes: 15}
	store3 := &fakeReminderStore{delivered: true}
	w3 := NewReminderWorker(events3, store3, &fakeDelivery{listErr: fmt.Errorf("channel svc down")}, nil)
	w3.now = func() time.Time { return now }
	w3.tick(ctx)
	if len(store3.deliveries) != 0 {
		t.Fatal("sink list failure must not deliver inbox row")
	}
}

// TestReminderWorker_TextTimezone 提醒正文按事件时区呈现。
func TestReminderWorker_TextTimezone(t *testing.T) {
	ev := &Event{Title: "站会", StartAt: day(1), EndAt: day(1).Add(30 * time.Minute), Timezone: "Asia/Shanghai"}
	text := reminderText(ev)
	if !strings.Contains(text, "站会") || !strings.Contains(text, "09:00") || !strings.Contains(text, "09:30") {
		t.Fatalf("reminder text timezone mismatch: %q", text)
	}
	// 非法时区回退不 panic。
	ev.Timezone = "Not/AZone"
	if reminderText(ev) == "" {
		t.Fatal("bad timezone must fall back, not panic")
	}
}

// ============================================================================
// ICalUsecase
// ============================================================================

// TestICalUsecase_TokenAndFeed K-3：明文令牌仅创建时出现（64 hex）、库内只存
// 哈希；feed 渲染 VEVENT/UTC 时间/转义；吊销即 401 语义（ErrICalTokenInvalid）。
func TestICalUsecase_TokenAndFeed(t *testing.T) {
	tokens := newFakeTokenRepo()
	events := newFakeEventRepo()
	uc := NewICalUsecase(tokens, events)
	ctx := context.Background()

	tok, plain, err := uc.CreateToken(ctx, "u-1", "手机")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if len(plain) != 64 {
		t.Fatalf("plain token len = %d, want 64 hex", len(plain))
	}
	if tok.TokenHash != HashICalToken(plain) {
		t.Fatal("stored hash must be SHA-256 of plain")
	}
	if strings.Contains(tok.TokenHash, plain) {
		t.Fatal("plain must not appear in stored hash")
	}

	events.byID["ev-1"] = &Event{ID: "ev-1", UserID: "u-1", Title: "逗号,分号;换行\n反斜\\", Note: "备注,;x", StartAt: day(9), EndAt: day(10), CreatedAt: day(0)}
	events.byID["ev-other"] = &Event{ID: "ev-other", UserID: "u-2", Title: "他人", StartAt: day(9), EndAt: day(10)}

	feed, err := uc.RenderFeed(ctx, plain, day(12))
	if err != nil {
		t.Fatalf("RenderFeed: %v", err)
	}
	for _, want := range []string{"BEGIN:VCALENDAR", "VERSION:2.0", "BEGIN:VEVENT", "UID:ev-1@aranea", "DTSTART:20261110T090000Z", "DTEND:20261110T100000Z", `SUMMARY:逗号\,分号\;换行\n反斜\\`, "END:VCALENDAR"} {
		if !strings.Contains(feed, want) {
			t.Fatalf("feed missing %q:\n%s", want, feed)
		}
	}
	if strings.Contains(feed, "ev-other") {
		t.Fatal("feed must not contain other user's events")
	}

	// 吊销 → feed 401 语义；管理端重复吊销 404。
	if err := uc.RevokeToken(ctx, tok.ID, "u-1"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if _, err := uc.RenderFeed(ctx, plain, day(12)); !errors.Is(err, ErrICalTokenInvalid) {
		t.Fatalf("revoked feed must be Invalid, got %v", err)
	}
	if err := uc.RevokeToken(ctx, tok.ID, "u-1"); !errors.Is(err, ErrICalTokenNotFound) {
		t.Fatalf("re-revoke must be NotFound, got %v", err)
	}
	// 乱码令牌 → Invalid。
	if _, err := uc.RenderFeed(ctx, "garbage", day(12)); !errors.Is(err, ErrICalTokenInvalid) {
		t.Fatalf("garbage token must be Invalid, got %v", err)
	}
}

// ============================================================================
// AppsUsecase
// ============================================================================

// TestAppsUsecase_URLValidation K-5：仅 http/https 绝对 URL（防 javascript:
// 伪协议经「打开」动作注入）；名称必填；upsert 规范化 URL。
func TestAppsUsecase_URLValidation(t *testing.T) {
	repo := &fakeAppRepo{}
	uc := NewAppsUsecase(repo)
	ctx := context.Background()

	for _, bad := range []string{"javascript:alert(1)", "ftp://x.com", "not-a-url", "https://", ""} {
		if _, err := uc.Upsert(ctx, &App{WorkspaceID: "ws-1", Name: "x", URL: bad}); !errors.Is(err, ErrAppInvalidURL) {
			t.Fatalf("url %q must be rejected, got %v", bad, err)
		}
	}
	if _, err := uc.Upsert(ctx, &App{WorkspaceID: "ws-1", Name: " ", URL: "https://x.com"}); !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("empty name must be rejected, got %v", err)
	}
	a, err := uc.Upsert(ctx, &App{WorkspaceID: "ws-1", Name: "Jira", URL: " https://jira.example.com/path ", GroupName: "dev"})
	if err != nil {
		t.Fatalf("valid upsert: %v", err)
	}
	if !strings.HasPrefix(a.ID, "app_") || a.URL != "https://jira.example.com/path" {
		t.Fatalf("upsert normalize mismatch: %+v", a)
	}
	if err := uc.Delete(ctx, a.ID, "ws-2"); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("cross-workspace delete must be NotFound, got %v", err)
	}
	if err := uc.Delete(ctx, a.ID, "ws-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
