package office

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// DraftUsecase agent 排期草案用例：提交（只产草案，不落事件）→ 审批中心
// 逐项审批 → 批准方在单事务内复检冲突并落库（需求概要「agent 规划草案
// 逐项审批」）。
type DraftUsecase struct {
	drafts DraftRepo
}

func NewDraftUsecase(drafts DraftRepo) *DraftUsecase {
	return &DraftUsecase{drafts: drafts}
}

// SubmitDraft 提交草案：仅做形状校验（title/end>start），不做冲突检测——
// 冲突是批准时刻的时点语义（提交到批准之间日历可能已变化），复检在
// ApproveDraft 事务内完成。
func (u *DraftUsecase) SubmitDraft(ctx context.Context, workspaceID, userID, agentID, sessionID string, items []DraftItem) (*ScheduleDraft, error) {
	if len(items) == 0 {
		return nil, ErrDraftEmpty
	}
	for i := range items {
		if err := validateWindow(items[i].Title, items[i].StartAt, items[i].EndAt); err != nil {
			return nil, err
		}
		if items[i].ReminderMinutes < 0 {
			items[i].ReminderMinutes = 0
		}
	}
	d := &ScheduleDraft{
		ID:          "sd_" + uuid.NewString(),
		WorkspaceID: workspaceID,
		UserID:      userID,
		AgentID:     agentID,
		SessionID:   sessionID,
		Status:      DraftStatusPending,
		Items:       items,
	}
	if err := u.drafts.CreateDraft(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ApproveDraft 批准草案：转换为属主事件（source=agent_draft）并交由仓储
// 单事务落库（逐项 409 复检 + CAS）。decidedBy 取 JWT 主体（service 层传入）。
func (u *DraftUsecase) ApproveDraft(ctx context.Context, draftID, decidedBy string) error {
	d, err := u.drafts.GetDraft(ctx, draftID)
	if err != nil {
		return err
	}
	if d.Status != DraftStatusPending {
		return ErrDraftNotPending
	}
	events := make([]*Event, 0, len(d.Items))
	for _, it := range d.Items {
		ev := &Event{
			ID:              "ev_" + uuid.NewString(),
			WorkspaceID:     d.WorkspaceID,
			UserID:          d.UserID,
			TaskRef:         it.TaskRef,
			Title:           it.Title,
			Note:            it.Note,
			StartAt:         it.StartAt,
			EndAt:           it.EndAt,
			ReminderMinutes: it.ReminderMinutes,
			Source:          SourceAgentDraft,
		}
		normalizeEvent(ev)
		events = append(events, ev)
	}
	return u.drafts.ApproveDraftTx(ctx, draftID, decidedBy, events)
}

// RejectDraft 驳回草案：仅留决议审计（decided_by/note/decided_at），不落事件。
func (u *DraftUsecase) RejectDraft(ctx context.Context, draftID, decidedBy, note string) error {
	if strings.TrimSpace(note) == "" {
		// 与 review.v1 收编同一条约：驳回必填理由（审计可读性）。
		return ErrRejectNoteRequired
	}
	return u.drafts.RejectDraft(ctx, draftID, decidedBy, note)
}
