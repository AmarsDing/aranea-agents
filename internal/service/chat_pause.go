package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// PauseSession pauses a running chat session (running → paused).
//
// Implementation:
//  1. Validate session_id and active run status (running/streaming).
//  2. Cancel the active runner via orch.CancelRun.
//  3. Sync MemberSession v2 status → paused (when session is a team member).
//  4. Publish run_status on the spirit root session (WS subscribers watch spirit),
//     with Meta.chat_session_id so the frontend can patch MemberSession cards.
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

	publishRunID := runID
	if publishRunID == "" {
		publishRunID = sessionID
	}
	s.syncMemberSessionStatus(ctx, sessionID, biz.MemberSessionStatusPaused)
	s.publishPauseResumeStatus(ctx, sessionID, publishRunID, "paused")

	return &chatv1.PauseSessionResponse{Paused: true}, nil
}

// ResumeSession resumes a paused chat session (paused → running).
//
// MVP does NOT automatically re-trigger execution. The user must send a new
// message to actually restart the turn. This RPC flips the run-status marker
// and MemberSession status so the UI affordance switches back to running.
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
	s.syncMemberSessionStatus(ctx, sessionID, biz.MemberSessionStatusRunning)
	s.publishPauseResumeStatus(ctx, sessionID, publishRunID, "running")

	return &chatv1.ResumeSessionResponse{Resumed: true}, nil
}

// resolveSpiritSessionID returns RootSessionID when present, otherwise sessionID.
func (s *ChatService) resolveSpiritSessionID(ctx context.Context, sessionID string) string {
	if s.orch == nil {
		return sessionID
	}
	sess, err := s.orch.td().Sessions.Get(ctx, sessionID)
	if err != nil {
		return sessionID
	}
	if root := strings.TrimSpace(sess.RootSessionID); root != "" {
		return root
	}
	return sessionID
}

// publishPauseResumeStatus updates the run registry for chatSessionID and
// publishes run_status on the spirit root so Chat WS subscribers receive it.
// Meta.chat_session_id identifies the member/chat session that was paused/resumed.
func (s *ChatService) publishPauseResumeStatus(ctx context.Context, chatSessionID, runID, status string) {
	spiritID := s.resolveSpiritSessionID(ctx, chatSessionID)
	// Registry key must remain the chat session so ResumeSession GetRunStatus works.
	if s.orch.runs != nil {
		s.orch.runs.SetStatus(chatSessionID, runID, status, "")
	}
	_ = s.orch.runStatus().PersistRunStatus(ctx, chatSessionID, runID, status, "")
	meta := map[string]any{
		"run_id":          runID,
		"status":          status,
		"chat_session_id": chatSessionID,
		"notice_type":     "info",
	}
	if bus := s.orch.td().Pipeline.EventBus; bus != nil {
		bus.Publish(ctx, biz.NewRunStatusEvent(spiritID, runID, status, meta))
	}
}

// syncMemberSessionStatus publishes member_session.updated via Sequencer
// (persist + WS) when the paused/resumed session belongs to a team member.
//
// 版本带纪律（2026-07-29 哨兵化，见 biz.MemberSessionVersion*）：
//   - 目标为终态（Mode B finish：completed/failed/skipped）→ 携带 outcome
//     权威带（哨兵），保证终态恒赢、终态之后无写者；
//   - 生命周期（paused/running）→ Version++ 单调递增（永远低于哨兵带）；
//   - 已终态记录 → 直接跳过，生命周期事件不得覆盖终态裁决。
func (s *ChatService) syncMemberSessionStatus(ctx context.Context, chatSessionID string, status biz.MemberSessionStatus) {
	repo := s.memberSessions
	if repo == nil && s.orch != nil {
		repo = s.orch.memberSessions()
	}
	var seq rt.EventPublisher
	if s.orch != nil {
		seq = s.orch.v2Seq
	}
	if repo == nil || seq == nil {
		return
	}
	ms, err := repo.GetMemberSessionByChatSessionID(ctx, chatSessionID)
	if err != nil {
		return
	}
	if biz.IsMemberSessionTerminal(ms.Status) {
		return
	}
	ms.Status = status
	if biz.IsMemberSessionTerminal(status) {
		ms.Version = biz.MemberSessionVersionOutcome
	} else {
		ms.Version++
	}
	seq.Publish(ctx, biz.NewMemberSessionUpdatedEvent(ms))
}
