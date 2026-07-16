package service

import (
	"context"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// ConfirmActivity handles user approval/rejection of a tool-blocked confirm Step.
// It loads from steps_v2, validates kind=confirm + status=tool_blocked,
// updates status via StepV2Writer, publishes system.notice, and resumes the
// awaiting run via the await channel.
func (s *ChatService) ConfirmActivity(ctx context.Context, req *chatv1.ConfirmActivityRequest) (*chatv1.ConfirmActivityResponse, error) {
	if s == nil || s.orch == nil {
		return nil, apierror.Internal(apierror.DomainChat, "service unavailable")
	}

	sessionID := strings.TrimSpace(req.GetSessionId())
	activityID := strings.TrimSpace(req.GetActivityId())
	if sessionID == "" || activityID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id and activity_id are required")
	}

	stepReader := s.orch.stepReader()
	stepWriter := s.orch.stepWriter()
	if stepReader == nil || stepWriter == nil {
		return nil, apierror.Internal(apierror.DomainChat, "step store unavailable")
	}
	step, err := stepReader.GetStep(ctx, activityID)
	if err != nil {
		return nil, err
	}

	if step.Kind != biz.StepKindConfirm {
		return nil, apierror.BadRequest(apierror.DomainChat, "expected confirm kind, got %s", step.Kind)
	}
	if step.Status != biz.StepStatusToolBlocked {
		return nil, apierror.BadRequest(apierror.DomainChat, "activity is not in tool_blocked state (current: %s)", step.Status)
	}
	if step.SessionID != sessionID {
		return nil, apierror.BadRequest(apierror.DomainChat, "activity does not belong to session %s", sessionID)
	}

	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized(apierror.DomainChat, "user authentication required for activity confirmation")
	}

	sessions := s.orch.td().Sessions
	if sessions != nil {
		session, err := sessions.Get(ctx, sessionID)
		if err != nil {
			return nil, apierror.NotFound(apierror.DomainChat, "session not found")
		}
		if session.UserID != userID {
			s.lg.Warn("confirm activity ownership denied",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("activity_id", activityID),
				loggateway.Str("user_id", userID),
				loggateway.Str("owner_id", session.UserID),
			)
			return nil, apierror.Forbidden(apierror.DomainChat, "only the session owner can confirm activities")
		}
	} else {
		return nil, apierror.Internal(apierror.DomainChat, "session store unavailable, cannot verify ownership")
	}

	transitionEvent := biz.ActivityTransitionDone
	if !req.GetApproved() {
		transitionEvent = biz.ActivityTransitionCancel
	}
	newStatus, err := biz.TransitionActivityStatus(biz.ActivityStatus(step.Status), transitionEvent)
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainChat,
			"illegal activity transition from %s via %s: %v",
			step.Status, transitionEvent, err)
	}
	step.Status = biz.StepStatus(newStatus)
	now := time.Now().UTC()
	step.CompletedAt = &now
	if _, err := stepWriter.UpdateStep(ctx, step); err != nil {
		return nil, err
	}

	if bus := s.orch.td().Pipeline.EventBus; bus != nil {
		decision := "approved"
		noticeType := "tool_confirm_approved"
		if !req.GetApproved() {
			decision = "rejected"
			noticeType = "tool_confirm_rejected"
		}
		meta := map[string]any{
			"activity_id": step.ID,
			"step_id":     step.ID,
			"decision":    decision,
			"status":      string(step.Status),
			"kind":        string(step.Kind),
		}
		bus.Publish(ctx, biz.NewSystemNoticeEvent(step.SessionID, noticeType, "", meta))
	}

	replyMsg := "approved"
	if !req.GetApproved() {
		replyMsg = "rejected"
	}
	runID := ""
	if _, requestID, active := s.orch.ActiveRunner(sessionID); active {
		runID = requestID
	}
	sent := s.orch.TrySendAwaitChannel(sessionID, biz.AwaitReplyMsg{
		RunID: runID,
		Reply: replyMsg,
	})
	if !sent {
		return &chatv1.ConfirmActivityResponse{
			Accepted: false,
			Status:   string(newStatus),
		}, nil
	}

	return &chatv1.ConfirmActivityResponse{
		Accepted: true,
		Status:   string(newStatus),
	}, nil
}
