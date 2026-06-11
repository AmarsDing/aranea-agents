package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

func (o *ChatOrchestrator) sessionContextPressure(ctx context.Context, input biz.TurnInput) bool {
	if o == nil || o.td.Sessions == nil {
		return false
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return false
	}
	sess, err := o.td.Sessions.Get(ctx, sessionID)
	if err != nil {
		return false
	}
	threshold := o.resolveContextAdmissionThreshold(ctx, sess, input)
	return biz.ContextPressureActive(sess.ContextUsedRatio, threshold)
}

func (o *ChatOrchestrator) resolveContextAdmissionThreshold(ctx context.Context, sess biz.Session, input biz.TurnInput) float64 {
	if input.EntryConfig.EntryPoint == biz.EntryPointChannel {
		lt := o.sessionRunLC.ResolveChannelLongTaskConfig(ctx, sess)
		return lt.ContextAdmissionThreshold
	}
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
