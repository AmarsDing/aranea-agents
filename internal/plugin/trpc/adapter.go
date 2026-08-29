package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type adaptedPlugin struct {
	plugin            trpcplugin.Plugin
	modelRouter       *ModelRouterConfig
	costGuard         *CostGuardConfig
	confirmationGuard *ConfirmationGuardConfig
}

func builtin(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, rt *Runtime, lg loggateway.Logger) trpcplugin.Plugin {
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "audit_log":
		return NewAuditLogPlugin(p, stats, monitorBus, lg)
	case "skill_usage_tracker":
		return NewSkillUsageTrackerPlugin(p, stats, monitorBus, lg)
	case "retry_and_reflect":
		return NewRetryAndReflectPlugin(p, stats, monitorBus, rt, lg)
	case "sensitive_data_mask":
		return NewSensitiveDataMaskPlugin(p, stats, monitorBus, lg)
	case "confirmation_guard":
		return NewConfirmationGuardPlugin(p, stats, monitorBus, lg)
	case "cost_guard":
		return NewCostGuardPlugin(p, stats, monitorBus, rt, lg)
	case "model_router":
		return NewModelRouterPlugin(p, stats, monitorBus, lg)
	case "permission_guard":
		var resolve AgentKeyResolver
		if rt != nil {
			rt.mu.RLock()
			resolve = rt.resolveAgent
			rt.mu.RUnlock()
		}
		return NewPermissionGuardPlugin(p, stats, monitorBus, resolve, lg)
	case "output_policy":
		// P1-④：决策 collector 经 getter 现取（Runtime.SetDecisionCollector
		// 由 newApp BeforeStart 后置注入，插件 Apply 可能更早）。
		var decisions func() decision.Collector
		if rt != nil {
			decisions = rt.decisionCollector
		}
		return NewOutputPolicyPlugin(p, stats, monitorBus, lg, decisions)
	default:
		return nil
	}
}

func adapt(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, rt *Runtime, lg loggateway.Logger) *adaptedPlugin {
	if !p.Enabled {
		return nil
	}
	ValidatePluginCallbackPoints(p)
	tp := builtin(p, stats, monitorBus, rt, lg)
	if tp == nil {
		return nil
	}
	ap := &adaptedPlugin{plugin: tp}
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "model_router":
		var cfg ModelRouterConfig
		parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
		// I-1: runtime 条目 config 供 PluginModelSelector 实际路由使用，
		// 必须编译 regex（遥测路径 NewModelRouterPlugin 已各自编译）。
		compileModelRouterRules(cfg.Rules, lg)
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
