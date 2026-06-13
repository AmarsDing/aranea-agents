package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// recordHookAudit persists hook invocations to plugin_runs for admin audit.
// The StatsRecorder decides whether to persist based on status and config.
func recordHookAudit(stats StatsRecorder, ctx context.Context, rh biz.ResolvedHook, point, status, agentID string, durationMS int64, hookErr error) {
	st := normalizeRunStatus(status)
	if stats == nil {
		return
	}
	key := strings.TrimSpace(rh.Hook.Key)
	if key == "" {
		return
	}
	summary := strings.TrimSpace(rh.Hook.Name)
	if summary == "" {
		summary = "hook:" + key
	}
	if hookErr != nil {
		summary += " — " + hookErr.Error()
	}
	stats.RecordEvent(ctx, CallbackEvent{
		PluginKey:  "hook:" + key,
		Point:      strings.TrimSpace(point),
		Status:     st,
		Action:     strings.TrimSpace(rh.Rule.Action.Type),
		AgentID:    strings.TrimSpace(agentID),
		DurationMS: int(durationMS),
		Summary:    summary,
	})
}
