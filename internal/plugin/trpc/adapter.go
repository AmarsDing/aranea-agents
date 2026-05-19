// Package plugintrpc bridges biz.Plugin DB rows to trpc-agent-go plugin.Plugin
// instances. Only plugins with known built-in keys are instantiated; unknown
// keys are silently skipped so that database experiments do not break the runner.
package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

// builtin returns a concrete plugin.Plugin for known built-in plugin keys.
// Returns nil if the key has no matching implementation.
func builtin(p biz.Plugin, stats StatsRecorder) trpcplugin.Plugin {
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "audit_log", "audit-log", "auditlog", "runtime_audit":
		return NewAuditLogPlugin(p, stats)
	case "skill_usage_tracker":
		return NewSkillUsageTrackerPlugin(p, stats)
	case "retry_and_reflect":
		return NewRetryAndReflectPlugin(p, stats)
	case "sensitive_data_mask":
		return NewSensitiveDataMaskPlugin(p, stats)
	case "confirmation_guard":
		return NewConfirmationGuardPlugin(p, stats)
	case "cost_guard":
		return NewCostGuardPlugin(p, stats)
	case "model_router":
		return NewModelRouterPlugin(p, stats)
	case "permission_guard":
		return NewPermissionGuardPlugin(p, stats)
	case "output_policy":
		return NewOutputPolicyPlugin(p, stats)
	default:
		return nil
	}
}

// adapt converts a biz.Plugin to a trpcplugin.Plugin.
// Returns nil when the plugin is disabled or has no built-in implementation.
func adapt(p biz.Plugin, stats StatsRecorder) trpcplugin.Plugin {
	if !p.Enabled {
		return nil
	}
	ValidatePluginCallbackPoints(p)
	return builtin(p, stats)
}
