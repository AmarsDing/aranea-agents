package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// ConfirmActivity handles user approval/rejection of a tool-blocked confirm Activity.
// It loads the Activity from DB, validates kind=confirm + status=tool_blocked,
// updates the status, publishes an ActivityEvent (completed/cancelled), and resumes the
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

	reader := s.orch.activityReader()
	if reader == nil {
		return nil, apierror.Internal(apierror.DomainChat, "activity reader unavailable")
	}

	// Load activity from DB
	activity, err := reader.GetActivity(ctx, activityID)
	if err != nil {
		return nil, err
	}

	// Validate kind
	if activity.Kind != biz.ActivityKindConfirm {
		return nil, apierror.BadRequest(apierror.DomainChat, "expected confirm kind, got %s", activity.Kind)
	}

	// Validate status
	if activity.Status != biz.ActivityStatusToolBlocked {
		return nil, apierror.BadRequest(apierror.DomainChat, "activity is not in tool_blocked state (current: %s)", activity.Status)
	}

	// Validate session ownership
	if activity.SessionID != sessionID {
		return nil, apierror.BadRequest(apierror.DomainChat, "activity does not belong to session %s", sessionID)
	}

	// Validate user identity - reject anonymous (default_user) confirm requests
	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized(apierror.DomainChat, "user authentication required for activity confirmation")
	}

	// Validate session ownership - only the session owner can confirm/reject activities
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

	// Update status
	newStatus := biz.ActivityStatusCompleted
	if !req.GetApproved() {
		newStatus = biz.ActivityStatusCancelled
	}
	activity.Status = newStatus

	// Persist the status update
	writer := s.orch.activityWriter()
	if writer == nil {
		return nil, apierror.Internal(apierror.DomainChat, "activity writer unavailable")
	}
	if _, err := writer.UpdateActivity(ctx, activity); err != nil {
		return nil, err
	}

	// Publish ActivityEvent (completed/cancelled) via ActivityEventBus so the
	// frontend's unified rendering pipeline receives the lifecycle transition.
	// This replaces the legacy EnvelopeTypeActivityDone envelope.
	if bus := s.orch.td().Pipeline.ActivityBus; bus != nil {
		eventType := biz.ActivityEventCompleted
		if !req.GetApproved() {
			eventType = biz.ActivityEventCancelled
		}
		bus.Publish(ctx, biz.ActivityEvent{
			Event:    eventType,
			Activity: activity,
		})
	}

	// Resume the awaiting run by sending the approval/rejection through the await channel.
	// The tool_confirm gate in the runtime is waiting for this signal.
	replyMsg := "approved"
	if !req.GetApproved() {
		replyMsg = "rejected"
	}
	// Resolve the active RunID for this session to avoid signal misdelivery (B-02).
	runID := ""
	if _, requestID, active := s.orch.ActiveRunner(sessionID); active {
		runID = requestID
	}
	sent := s.orch.TrySendAwaitChannel(sessionID, biz.AwaitReplyMsg{
		RunID: runID,
		Reply: replyMsg,
	})
	if !sent {
		// Channel full or closed - the runtime may not have received the signal.
		// Return accepted=false so the frontend knows the confirm didn't reach the runner.
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
