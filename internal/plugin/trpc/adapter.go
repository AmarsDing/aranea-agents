package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

func builtin(p biz.Plugin, stats StatsRecorder, bus event.Bus) trpcplugin.Plugin {
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "audit_log", "audit-log", "auditlog", "runtime_audit":
		return NewAuditLogPlugin(p, stats, bus)
	case "skill_usage_tracker":
		return NewSkillUsageTrackerPlugin(p, stats, bus)
	case "retry_and_reflect":
		return NewRetryAndReflectPlugin(p, stats, bus)
	case "sensitive_data_mask":
		return NewSensitiveDataMaskPlugin(p, stats, bus)
	case "confirmation_guard":
		return NewConfirmationGuardPlugin(p, stats, bus)
	case "cost_guard":
		return NewCostGuardPlugin(p, stats, bus)
	case "model_router":
		return NewModelRouterPlugin(p, stats, bus)
	case "permission_guard":
		return NewPermissionGuardPlugin(p, stats, bus)
	case "output_policy":
		return NewOutputPolicyPlugin(p, stats, bus)
	default:
		return nil
	}
}

func adapt(p biz.Plugin, stats StatsRecorder, bus event.Bus) trpcplugin.Plugin {
	if !p.Enabled {
		return nil
	}
	ValidatePluginCallbackPoints(p)
	return builtin(p, stats, bus)
}
