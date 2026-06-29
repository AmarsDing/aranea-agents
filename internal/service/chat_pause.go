package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// PauseSession pauses a running chat session (running → paused).
//
// MVP implementation:
//  1. Validate session_id.
//  2. Look up the current run status; if not 'running' or 'streaming',
//     return Conflict.
//  3. Cancel the active runner via orch.CancelRun — this stops the in-flight
//     turn. (trpc-agent-go does not natively support framework-level pause.)
//  4. Publish run_status notice with status=paused so the frontend AgentCard
//     re-renders with the resume affordance.
//
// Note: true "resume from checkpoint" requires trpc-agent-go native pause
// support. ResumeSession therefore relies on the user sending a new message
// to re-trigger the turn.
func (s *ChatService) PauseSession(ctx context.Context, req *chatv1.PauseSessionRequest) (*chatv1.PauseSessionResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	if s.orch == nil {
		return nil, apierror.Internal(apierror.DomainChat, "orchestrator not configured")
	}

	// Validate there is an active run that can be paused.
	runID, status, _, _, ok := s.orch.GetRunStatus(ctx, sessionID)
	if !ok {
		return nil, apierror.Conflict(apierror.DomainChat, "session %s has no active run to pause", sessionID)
	}
	if status != "running" && status != "streaming" {
		return nil, apierror.Conflict(apierror.DomainChat, "session %s run status is %s; pause requires running or streaming", sessionID, status)
	}

	// Cancel the active runner. We intentionally do NOT call
	// chatactivity.CancelRunningActivityMessages here (unlike CancelRun) so
	// that the in-flight activity cards remain visible to the user — they
	// represent work that can be conceptually resumed.
	stopped := s.orch.CancelRun(ctx, sessionID)
	if !stopped {
		s.lg.Warn("pause session: no active runner cancelled",
			loggateway.StepID("chat.pause"),
			loggateway.Str("session_id", sessionID),
		)
	}

	// Publish run_status notice (paused) so the frontend re-renders the
	// AgentCard tail with the resume affordance. We use a fresh runID from
	// GetRunStatus (in case CancelRun cleared it).
	publishRunID := runID
	if publishRunID == "" {
		publishRunID = sessionID
	}
	s.setRunStatus(ctx, sessionID, publishRunID, "paused", "")

	return &chatv1.PauseSessionResponse{Paused: true}, nil
}

// ResumeSession resumes a paused chat session (paused → running).
//
// MVP implementation:
//  1. Validate session_id.
//  2. Look up the current run status; if not 'paused', return Conflict.
//  3. Publish run_status notice with status=running so the frontend
//     AgentCard re-renders with the pause affordance.
//
// Note: MVP does NOT automatically re-trigger execution. The user must send
// a new message to actually restart the turn. This RPC exists primarily to
// flip the run-status marker so the UI affordance switches back to "running"
// state (showing pause + dialog, hiding resume).
func (s *ChatService) ResumeSession(ctx context.Context, req *chatv1.ResumeSessionRequest) (*chatv1.ResumeSessionResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	if s.orch == nil {
		return nil, apierror.Internal(apierror.DomainChat, "orchestrator not configured")
	}

	runID, status, _, _, ok := s.orch.GetRunStatus(ctx, sessionID)
	if !ok {
		return nil, apierror.Conflict(apierror.DomainChat, "session %s has no run to resume", sessionID)
	}
	if status != "paused" {
		return nil, apierror.Conflict(apierror.DomainChat, "session %s run status is %s; resume requires paused state", sessionID, status)
	}

	publishRunID := runID
	if publishRunID == "" {
		publishRunID = sessionID
	}
	s.setRunStatus(ctx, sessionID, publishRunID, "running", "")

	return &chatv1.ResumeSessionResponse{Resumed: true}, nil
}
