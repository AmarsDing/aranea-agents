package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type adaptedPlugin struct {
	plugin              trpcplugin.Plugin
	modelRouter         *ModelRouterConfig
	costGuard           *CostGuardConfig
	confirmationGuard   *ConfirmationGuardConfig
}

func builtin(p biz.Plugin, stats StatsRecorder, bus event.Bus, rt *Runtime, lg loggateway.Logger) trpcplugin.Plugin {
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "audit_log":
		return NewAuditLogPlugin(p, stats, bus, lg)
	case "skill_usage_tracker":
		return NewSkillUsageTrackerPlugin(p, stats, bus, lg)
	case "retry_and_reflect":
		return NewRetryAndReflectPlugin(p, stats, bus, rt, lg)
	case "sensitive_data_mask":
		return NewSensitiveDataMaskPlugin(p, stats, bus, lg)
	case "confirmation_guard":
		return NewConfirmationGuardPlugin(p, stats, bus, lg)
	case "cost_guard":
		return NewCostGuardPlugin(p, stats, bus, rt, lg)
	case "model_router":
		return NewModelRouterPlugin(p, stats, bus, lg)
	case "permission_guard":
		var resolve AgentKeyResolver
		if rt != nil {
			rt.mu.RLock()
			resolve = rt.resolveAgent
			rt.mu.RUnlock()
		}
		return NewPermissionGuardPlugin(p, stats, bus, resolve, lg)
	case "output_policy":
		return NewOutputPolicyPlugin(p, stats, bus, lg)
	default:
		return nil
	}
}

func adapt(p biz.Plugin, stats StatsRecorder, bus event.Bus, rt *Runtime, lg loggateway.Logger) *adaptedPlugin {
	if !p.Enabled {
		return nil
	}
	ValidatePluginCallbackPoints(p)
	tp := builtin(p, stats, bus, rt, lg)
	if tp == nil {
		return nil
	}
	ap := &adaptedPlugin{plugin: tp}
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "model_router":
		var cfg ModelRouterConfig
		parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
		ap.modelRouter = &cfg
	case "cost_guard":
		var cfg CostGuardConfig
		parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
		ap.costGuard = &cfg
	case "confirmation_guard":
		var cfg ConfirmationGuardConfig
		parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
		ap.confirmationGuard = &cfg
	}
	return ap
}
