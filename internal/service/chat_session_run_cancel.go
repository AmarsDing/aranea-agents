package service

import (
	"context"
	"strings"

	"aranea-agents/internal/event"
)

// CancelSessionRunForCard stops a run after validating session_run ownership (Feishu card cancel).
func (s *ChatService) CancelSessionRunForCard(ctx context.Context, sessionRunID, expectedSessionID string) (cancelled bool, reply string) {
	if s == nil || s.orch == nil {
		return false, "当前没有进行中的任务"
	}
	sessionRunID = strings.TrimSpace(sessionRunID)
	expectedSessionID = strings.TrimSpace(expectedSessionID)
	if sessionRunID != "" && s.orch.chTurn.SessionRuns != nil {
		run, err := s.orch.chTurn.SessionRuns.Get(ctx, sessionRunID)
		if err != nil || run.ID == "" {
			return false, channelBackgroundReplyNoActiveRun
		}
		if expectedSessionID != "" && run.SessionID != expectedSessionID {
			event.SysLogWarn(flowStepRunEscalate, "session run cancel ownership denied",
				event.P("session_run_id", sessionRunID),
				event.P("expected_session_id", expectedSessionID),
				event.P("run_session_id", run.SessionID),
			)
			return false, channelBackgroundReplyDenied
		}
		expectedSessionID = run.SessionID
	}
	if expectedSessionID == "" {
		return false, channelBackgroundReplyNoActiveRun
	}
	if !s.CancelRun(ctx, expectedSessionID) {
		return false, "当前没有进行中的任务"
	}
	return true, "已取消当前任务"
}
