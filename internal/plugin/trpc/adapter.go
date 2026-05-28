package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type adaptedPlugin struct {
	plugin              trpcplugin.Plugin
	modelRouter         *ModelRouterConfig
	costGuard           *CostGuardConfig
	confirmationGuard   *ConfirmationGuardConfig
}

func builtin(p biz.Plugin, stats StatsRecorder, bus event.Bus, rt *Runtime) trpcplugin.Plugin {
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "audit_log":
		return NewAuditLogPlugin(p, stats, bus)
	case "skill_usage_tracker":
		return NewSkillUsageTrackerPlugin(p, stats, bus)
	case "retry_and_reflect":
		return NewRetryAndReflectPlugin(p, stats, bus, rt)
	case "sensitive_data_mask":
		return NewSensitiveDataMaskPlugin(p, stats, bus)
	case "confirmation_guard":
		return NewConfirmationGuardPlugin(p, stats, bus)
	case "cost_guard":
		return NewCostGuardPlugin(p, stats, bus, rt)
	case "model_router":
		return NewModelRouterPlugin(p, stats, bus)
	case "permission_guard":
		var resolve AgentKeyResolver
		if rt != nil {
			rt.mu.RLock()
			resolve = rt.resolveAgent
			rt.mu.RUnlock()
		}
		return NewPermissionGuardPlugin(p, stats, bus, resolve)
	case "output_policy":
		return NewOutputPolicyPlugin(p, stats, bus)
	default:
		return nil
	}
}

func adapt(p biz.Plugin, stats StatsRecorder, bus event.Bus, rt *Runtime) *adaptedPlugin {
	if !p.Enabled {
		return nil
	}
	ValidatePluginCallbackPoints(p)
	tp := builtin(p, stats, bus, rt)
	if tp == nil {
		return nil
	}
	ap := &adaptedPlugin{plugin: tp}
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "model_router":
		var cfg ModelRouterConfig
		parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
		ap.modelRouter = &cfg
	case "cost_guard":
		var cfg CostGuardConfig
		parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
		ap.costGuard = &cfg
	case "confirmation_guard":
		var cfg ConfirmationGuardConfig
		parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
		ap.confirmationGuard = &cfg
	}
	return ap
}
