package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

const (
	flowStepRunEscalate       = "run.escalate.durable"
	channelBackgroundReplyDenied = "无权操作该任务。"
)

func firstNonEmptyString(parts ...string) string {
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			return v
		}
	}
	return ""
}

// EscalateActiveSessionRun moves the active session run to durable phase (CC-R-02 /background).
func (s *ChatService) EscalateActiveSessionRun(ctx context.Context, sessionID string) (escalated bool, reply string, err error) {
	if s == nil || s.orch == nil || s.orch.chJobs.SessionRuns == nil {
		return false, channelBackgroundReplyNoActiveRun, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, channelBackgroundReplyNoActiveRun, nil
	}
	run, err := s.orch.chJobs.SessionRuns.GetActiveForSession(ctx, sessionID)
	if err != nil || run.ID == "" {
		return false, channelBackgroundReplyNoActiveRun, nil
	}
	if run.Phase == biz.SessionRunPhaseDurable {
		return true, channelBackgroundReplyAlready, nil
	}
	s.orch.escalateSessionRunToDurable(ctx, sessionID, run.ID)
	return true, channelBackgroundReplyOK, nil
}

// EscalateSessionRun moves a specific session run to durable phase (Feishu card callback / CC-F-02).
// When expectedSessionID is non-empty, run must belong to that session (CC-R-OPT-02).
func (s *ChatService) EscalateSessionRun(ctx context.Context, sessionRunID, expectedSessionID string) (reply string, err error) {
	if s == nil || s.orch == nil || s.orch.chJobs.SessionRuns == nil {
		return channelBackgroundReplyNoActiveRun, nil
	}
	sessionRunID = strings.TrimSpace(sessionRunID)
	expectedSessionID = strings.TrimSpace(expectedSessionID)
	if sessionRunID == "" {
		return channelBackgroundReplyNoActiveRun, nil
	}
	run, err := s.orch.chJobs.SessionRuns.Get(ctx, sessionRunID)
	if err != nil || run.ID == "" {
		return channelBackgroundReplyNoActiveRun, nil
	}
	if expectedSessionID != "" && run.SessionID != expectedSessionID {
		s.lg.Warn("session run ownership denied",
			loggateway.StepID(flowStepRunEscalate),
			loggateway.Str("session_run_id", sessionRunID),
			loggateway.Str("expected_session_id", expectedSessionID),
			loggateway.Str("run_session_id", run.SessionID),
		)
		return channelBackgroundReplyDenied, nil
	}
	if run.Phase == biz.SessionRunPhaseDurable {
		return channelBackgroundReplyAlready, nil
	}
	if run.Phase == biz.SessionRunPhaseCompleted || run.Phase == biz.SessionRunPhaseFailed {
		return channelBackgroundReplyNoActiveRun, nil
	}
	s.orch.escalateSessionRunToDurable(ctx, run.SessionID, run.ID)
	return channelBackgroundReplyOK, nil
}
