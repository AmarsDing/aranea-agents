package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func (o *ChatOrchestrator) sessionContextPressure(ctx context.Context, input biz.TurnInput) bool {
	if o == nil || o.admission() == nil {
		return false
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return false
	}
	sess, err := o.td().Sessions.Get(ctx, sessionID)
	if err != nil {
		o.lg().Warn("session lookup failed in context pressure check, skipping",
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		return false
	}
	result := o.admission().EvaluateContextPressure(ctx, sess, input.EntryConfig.EntryPoint)
	return result.Pressure
}

// resolveContextAdmissionThresholdForSession implements
// biz.ContextThresholdResolver for non-channel sessions. It resolves the
// threshold from the agent's L0SummaryThreshold, falling back to the default.
func (o *ChatOrchestrator) resolveContextAdmissionThresholdForSession(ctx context.Context, sess biz.Session, entryPoint biz.TurnEntryPoint) float64 {
	threshold := biz.DefaultContextAdmissionThreshold
	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		return threshold
	}
	ag, err := o.hydratedAgent(ctx, agentID)
	if err != nil || ag.Settings == nil || ag.Settings.L0SummaryThreshold <= 0 {
		return threshold
	}
	return ag.Settings.L0SummaryThreshold
}
