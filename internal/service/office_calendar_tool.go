package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/office"
	"aranea-agents/internal/tools/calendar"
	"aranea-agents/pkg/apierror"
)

// ============================================================================
// officeCalendarTool —— calendar_list / calendar_create_draft 工具窄口
// （E 阶段 W1c；tools/calendar.Service ← office 用例 + session 解析）。
//
// 身份只从会话解析（session→user/workspace/agent），工具入参不含身份字段，
// agent 不可自报身份。
// ============================================================================

type officeCalendarTool struct {
	schedule *office.ScheduleUsecase
	drafts   *office.DraftUsecase
	sessions *biz.SessionUsecase
}

var _ calendar.Service = (*officeCalendarTool)(nil)

// ProvideOfficeCalendarTool 装配日程工具窄口（wire 注入 ToolBridges.Calendar）。
func ProvideOfficeCalendarTool(s *office.ScheduleUsecase, d *office.DraftUsecase, sessions *biz.SessionUsecase) calendar.Service {
	if s == nil || d == nil || sessions == nil {
		return nil
	}
	return &officeCalendarTool{schedule: s, drafts: d, sessions: sessions}
}

func (a *officeCalendarTool) resolveSession(ctx context.Context, sessionID string) (biz.Session, error) {
	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return biz.Session{}, apierror.NotFound(office.DomainOffice, "session not found")
	}
	if sess.UserID == "" {
		return biz.Session{}, apierror.Internal(office.DomainOffice, "session has no owner")
	}
	return sess, nil
}

func (a *officeCalendarTool) ListMyEvents(ctx context.Context, sessionID string, start, end time.Time) ([]calendar.EventView, error) {
	sess, err := a.resolveSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	events, err := a.schedule.ListEventsRange(ctx, sess.UserID, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]calendar.EventView, 0, len(events))
	for _, ev := range events {
		out = append(out, calendar.EventView{
			ID:              ev.ID,
			Title:           ev.Title,
			Note:            ev.Note,
			StartAt:         ev.StartAt,
			EndAt:           ev.EndAt,
			Timezone:        ev.Timezone,
			ReminderMinutes: ev.ReminderMinutes,
			Source:          ev.Source,
			TaskRef:         ev.TaskRef,
		})
	}
	return out, nil
}

func (a *officeCalendarTool) CreateDraft(ctx context.Context, sessionID string, items []calendar.DraftItemInput) (string, error) {
	sess, err := a.resolveSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	draftItems := make([]office.DraftItem, 0, len(items))
	for _, it := range items {
		draftItems = append(draftItems, office.DraftItem{
			Title:           it.Title,
			Note:            it.Note,
			StartAt:         it.StartAt,
			EndAt:           it.EndAt,
			ReminderMinutes: it.ReminderMinutes,
			TaskRef:         it.TaskRef,
		})
	}
	d, err := a.drafts.SubmitDraft(ctx, sess.WorkspaceID, sess.UserID, sess.AgentID, sessionID, draftItems)
	if err != nil {
		return "", err
	}
	return d.ID, nil
}
