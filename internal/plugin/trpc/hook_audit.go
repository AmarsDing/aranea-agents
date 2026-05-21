package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// recordHookAudit persists blocked/error hook invocations to plugin_runs for admin audit.
func recordHookAudit(stats StatsRecorder, ctx context.Context, rh biz.ResolvedHook, point, status, agentID string, durationMS int64) {
	st := normalizeRunStatus(status)
	if st != "blocked" && st != "error" {
		return
	}
	if stats == nil {
		return
	}
	key := strings.TrimSpace(rh.Hook.Key)
	if key == "" {
		return
	}
	stats.RecordEvent(ctx, CallbackEvent{
		PluginKey:  "hook:" + key,
		Point:      strings.TrimSpace(point),
		Status:     st,
		Action:     strings.TrimSpace(rh.Rule.Action.Type),
		AgentID:    strings.TrimSpace(agentID),
		DurationMS: int(durationMS),
		Summary:    strings.TrimSpace(rh.Hook.Name),
	})
}
