package office

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ScheduleUsecase 日程用例（CRUD + 范围聚合 + 任务双向联动）。
// 属主隔离：全部读写以 JWT 主体 userID 为界（构造期由 service 层传入，
// 不接受请求体自报）。
type ScheduleUsecase struct {
	events EventRepo
}

func NewScheduleUsecase(events EventRepo) *ScheduleUsecase {
	return &ScheduleUsecase{events: events}
}

// validateWindow 端点校验（平移 workbench：end>start 强制、title 必填）。
func validateWindow(title string, start, end time.Time) error {
	if strings.TrimSpace(title) == "" {
		return ErrTitleRequired
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return ErrInvalidTimeRange
	}
	return nil
}

// ensureNoOverlap 409 冲突检测（平移 workbench：同一用户时间段重叠即冲突）。
func (u *ScheduleUsecase) countOverlap(ctx context.Context, userID string, start, end time.Time, excludeID string) error {
	n, err := u.events.CountOverlap(ctx, userID, start, end, excludeID)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrEventConflict
	}
	return nil
}

func normalizeEvent(ev *Event) {
	if ev.Timezone == "" {
		ev.Timezone = DefaultTimezone
	}
	if ev.ReminderMinutes < 0 {
		ev.ReminderMinutes = 0
	}
	if ev.Kind == "" {
		ev.Kind = KindPlan
	}
	if ev.Status == "" {
		ev.Status = StatusTodo
	}
}

// validateKindStatus 类型、状态白名单（normalizeEvent 之后调用）。
func validateKindStatus(ev *Event) error {
	if !ValidKind(ev.Kind) {
		return ErrInvalidKind
	}
	if !ValidStatus(ev.Status) {
		return ErrInvalidStatus
	}
	return nil
}

// occupiesTime 只有「定了时间段的计划」占用时间：工作记录（log）和全天
// 计划都不参与 409 重叠检测（仓储侧计数同口径）。
func occupiesTime(ev *Event) bool { return ev.Kind == KindPlan && !ev.AllDay }

// CreateEvent 手工建事件（409 语义平移 verify-schedule S4 createEvent 路径）。
func (u *ScheduleUsecase) CreateEvent(ctx context.Context, ev *Event) (*Event, error) {
	if err := validateWindow(ev.Title, ev.StartAt, ev.EndAt); err != nil {
		return nil, err
	}
	normalizeEvent(ev)
	if err := validateKindStatus(ev); err != nil {
		return nil, err
	}
	if occupiesTime(ev) {
		if err := u.countOverlap(ctx, ev.UserID, ev.StartAt, ev.EndAt, ""); err != nil {
			return nil, err
		}
	}
	ev.ID = "ev_" + uuid.NewString()
	if ev.Source == "" {
		ev.Source = SourceManual
	}
	if err := u.events.CreateEvent(ctx, ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// UpdateEvent 部分更新：title/note 空串=保持原值；start/end 零值=保持；
// timezone 空串=保持；reminder_minutes 恒应用（proto3 标量无 presence，
// 0 语义=关闭提醒——见 proto UpdateEventRequest 注释）。改时间后复检 409
// （排除自身）；提醒窗口变化时重置提醒投递标记（reminded_at 清空重投）。
// extra 里的类型 / 状态 / 全天 / 完成情况 nil = 不改（可以清空完成情况、取消全天）；
// 变成「定了时间段的计划」时（改时间、log→plan、取消全天）复检 409。
func (u *ScheduleUsecase) UpdateEvent(ctx context.Context, userID string, patch *Event, extra EventPatch) (*Event, error) {
	cur, err := u.events.GetEvent(ctx, patch.ID, userID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(patch.Title) != "" {
		cur.Title = patch.Title
	}
	if extra.Note != nil {
		cur.Note = *extra.Note
	} else if patch.Note != "" {
		cur.Note = patch.Note
	}
	timeChanged := false
	if !patch.StartAt.IsZero() {
		cur.StartAt = patch.StartAt
		timeChanged = true
	}
	if !patch.EndAt.IsZero() {
		cur.EndAt = patch.EndAt
		timeChanged = true
	}
	if patch.Timezone != "" {
		cur.Timezone = patch.Timezone
	}
	reminderChanged := patch.ReminderMinutes != cur.ReminderMinutes
	cur.ReminderMinutes = patch.ReminderMinutes
	occupiedBefore := occupiesTime(cur)
	if extra.Kind != nil {
		cur.Kind = *extra.Kind
	}
	if extra.Status != nil {
		cur.Status = *extra.Status
	}
	if extra.AllDay != nil {
		cur.AllDay = *extra.AllDay
	}
	if extra.Outcome != nil {
		cur.Outcome = *extra.Outcome
	}
	normalizeEvent(cur)
	if err := validateKindStatus(cur); err != nil {
		return nil, err
	}
	if err := validateWindow(cur.Title, cur.StartAt, cur.EndAt); err != nil {
		return nil, err
	}
	if occupiesTime(cur) && (timeChanged || !occupiedBefore) {
		if err := u.countOverlap(ctx, userID, cur.StartAt, cur.EndAt, cur.ID); err != nil {
			return nil, err
		}
	}
	if timeChanged || reminderChanged {
		cur.RemindedAt = nil
	}
	if err := u.events.UpdateEvent(ctx, cur); err != nil {
		return nil, err
	}
	return cur, nil
}

func (u *ScheduleUsecase) DeleteEvent(ctx context.Context, id, userID string) error {
	return u.events.DeleteEvent(ctx, id, userID)
}

func (u *ScheduleUsecase) GetEvent(ctx context.Context, id, userID string) (*Event, error) {
	return u.events.GetEvent(ctx, id, userID)
}

// ListEventsRange K-1 范围聚合（月历一次请求）。区间合法性 400。
func (u *ScheduleUsecase) ListEventsRange(ctx context.Context, userID string, start, end time.Time) ([]*Event, error) {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, ErrInvalidTimeRange
	}
	return u.events.ListEventsRange(ctx, userID, start, end)
}

// ScheduleTask 任务→日程正向联动（409 语义同手工创建；标题缺省自动
// 「执行：」前缀，平移 workbench scheduleTask）。
func (u *ScheduleUsecase) ScheduleTask(ctx context.Context, workspaceID, userID, taskRef, taskTitle string, start, end time.Time, reminderMinutes int) (*Event, error) {
	if strings.TrimSpace(taskRef) == "" || strings.TrimSpace(taskTitle) == "" {
		return nil, ErrTitleRequired
	}
	if err := validateWindow("x", start, end); err != nil {
		return nil, err
	}
	if err := u.countOverlap(ctx, userID, start, end, ""); err != nil {
		return nil, err
	}
	ev := &Event{
		ID:              "ev_" + uuid.NewString(),
		WorkspaceID:     workspaceID,
		UserID:          userID,
		TaskRef:         taskRef,
		TaskTitle:       taskTitle,
		Title:           "执行：" + taskTitle,
		StartAt:         start,
		EndAt:           end,
		ReminderMinutes: reminderMinutes,
		Source:          SourceTaskSchedule,
	}
	normalizeEvent(ev)
	if err := u.events.CreateEvent(ctx, ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// ListTaskEvents 时间块反查（平移 workbench taskEvents：本人 + task 限定）。
func (u *ScheduleUsecase) ListTaskEvents(ctx context.Context, userID, taskRef string) ([]*Event, error) {
	if strings.TrimSpace(taskRef) == "" {
		return nil, ErrTitleRequired
	}
	return u.events.ListTaskEvents(ctx, userID, taskRef)
}
