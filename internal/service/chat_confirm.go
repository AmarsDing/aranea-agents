package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// resolveConfirmReply maps the ConfirmActivity request to the reply token
// sent through the await channel. A structured reply token (grant-scoped
// approve/deny) takes precedence over the legacy approved flag so the
// runtime confirmation gate can record the grant scope.
func resolveConfirmReply(reqApproved bool, reqReply string) (token string, approved bool, err error) {
	reqReply = strings.TrimSpace(reqReply)
	if reqReply == "" {
		if reqApproved {
			return "approved", true, nil
		}
		return "rejected", false, nil
	}
	outcome, structured := serviceawaitreply.ParseToolConfirmOutcome(reqReply)
	if !structured {
		return "", false, apierror.BadRequest(apierror.DomainChat, "unknown confirm reply token")
	}
	return reqReply, outcome.Approved(), nil
}

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

	// Phase 3b-D Task 5: load via v2 StepV2Reader (reads from steps_v2 table)
	// and convert back to v1 Activity shape. The legacy activityReader is
	// retained for fallback; if StepReader is nil (v1-only deployments) we
	// fall back to the v1 reader so existing tests/CLI continue to work.
	stepReader := s.orch.stepReader()
	var activity biz.Activity
	if stepReader != nil {
		step, err := stepReader.GetStep(ctx, activityID)
		if err != nil {
			return nil, err
		}
		activity = biz.StepToActivity(step)
	} else {
		reader := s.orch.activityReader()
		if reader == nil {
			return nil, apierror.Internal(apierror.DomainChat, "activity reader unavailable")
		}
		var err error
		activity, err = reader.GetActivity(ctx, activityID)
		if err != nil {
			return nil, err
		}
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

	// Update status via state machine (AS-FSM-01).
	// ToolBlocked → Completed (approved) or Cancelled (rejected).
	// Using TransitionActivityStatus enforces legal transitions and prevents
	// illegal direct assignments that bypass the state machine.
	transitionEvent := biz.ActivityTransitionDone
	if !req.GetApproved() {
		transitionEvent = biz.ActivityTransitionCancel
	}
	newStatus, err := biz.TransitionActivityStatus(activity.Status, transitionEvent)
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainChat,
			"illegal activity transition from %s via %s: %v",
			activity.Status, transitionEvent, err)
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
	//
	// Phase 3b-D: migrated to v2 EventBus via ActivityBridgeEvent.
	if bus := s.orch.td().Pipeline.EventBus; bus != nil {
		eventType := biz.ActivityEventCompleted
		if !req.GetApproved() {
			eventType = biz.ActivityEventCancelled
		}
		bus.Publish(ctx, biz.NewActivityBridgeEvent(biz.ActivityEvent{
			Event:    eventType,
			Activity: activity,
		}))
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
