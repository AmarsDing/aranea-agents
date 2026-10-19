package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// contextAdmissionSettingsWarnThrottle gates the settings-read failure warn on
// the per-turn admission fast path: the first occurrence per window logs (with
// the suppressed count since the last one), the rest are counted and dropped —
// a broken settings store must not spam one warn per turn, but the failure
// must stay distinguishable from "agent has no threshold configured".
var contextAdmissionSettingsWarnThrottle = loggateway.NewThrottle(time.Minute)

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
//
// P3-1 (2026-10-19): settings-only read — the resolver only needs
// Settings.L0SummaryThreshold; the previous full hydrate ran 5 extra DB
// queries per web turn. Narrow test stubs lacking the method fall back to the
// original (hydrated) resolution.
func (o *ChatOrchestrator) resolveContextAdmissionThresholdForSession(ctx context.Context, sess biz.Session, entryPoint biz.TurnEntryPoint) float64 {
	threshold := biz.DefaultContextAdmissionThreshold
	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		return threshold
	}
	if settingsReader, ok := o.td().ReadDeps.AgentsUC.(interface {
		GetRuntimeSettingsForAdmission(ctx context.Context, id string) (biz.AgentRuntimeSettings, error)
	}); ok {
		settings, err := settingsReader.GetRuntimeSettingsForAdmission(ctx, agentID)
		if err != nil {
			// Distinguish "settings store broken" from "no threshold
			// configured": the former silently degrades every turn's
			// admission threshold to the default, so log it (throttled).
			if ok, suppressed := contextAdmissionSettingsWarnThrottle.Allow(); ok {
				o.lg().Warn("context admission settings read failed, using default threshold",
					loggateway.Str("agent_id", agentID),
					loggateway.Int("suppressed", suppressed),
					loggateway.Err(err))
			}
			return threshold
		}
		if settings.L0SummaryThreshold <= 0 {
			return threshold
		}
		return settings.L0SummaryThreshold
	}
	ag, err := o.hydratedAgent(ctx, agentID)
	if err != nil || ag.Settings == nil || ag.Settings.L0SummaryThreshold <= 0 {
		return threshold
	}
	return ag.Settings.L0SummaryThreshold
}
