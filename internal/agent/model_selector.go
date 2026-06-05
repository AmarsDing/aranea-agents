package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// PluginCostGuardSelector switches to fallback when blocked_models or budget limits require routing.
func PluginCostGuardSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	cfg plugintrpc.CostGuardConfig,
	tracker *plugintrpc.CostGuardBudgetTracker,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		est := plugintrpc.EstimateInvocationTokens(inv)
		target := plugintrpc.ResolveCostGuardTarget(baseMod, cfg, est, tracker)
		if target == "" || target == baseMod {
			return nil, nil
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, target, lg)
		if err != nil {
			lg.Warn("费用保护回退到基础模型",
				loggateway.StepID("agent.cost_guard.fallback"),
				loggateway.Str("provider", baseProv), loggateway.Str("target", target), loggateway.Str("base", baseMod), loggateway.Err(err))
			metrics.ModelRouterFallbackTotal.WithLabelValues("cost_guard_catalog").Inc()
			return nil, nil
		}
		lg.Info("费用保护切换模型", loggateway.StepID("agent.cost_guard.switch"), loggateway.Phase("done"),
			loggateway.Str("provider", baseProv), loggateway.Str("target", target), loggateway.Str("base", baseMod), loggateway.Int("est_tokens", est))
		return m, nil
	}
}

// ChainedModelSelector tries selectors in order; first non-nil model wins.
func ChainedModelSelector(selectors ...trpcagent.ModelSelector) trpcagent.ModelSelector {
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		for _, sel := range selectors {
			if sel == nil {
				continue
			}
			m, err := sel(ctx, inv)
			if err != nil {
				return nil, err
			}
			if m != nil {
				return m, nil
			}
		}
		return nil, nil
	}
}

// PluginModelSelector returns a ModelSelector that routes to another catalog model when model_router is enabled.
func PluginModelSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	cfg plugintrpc.ModelRouterConfig,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		prompt := invocationPrompt(inv)
		target := plugintrpc.ResolveModelAPI(prompt, cfg)
		if target == "" || target == baseMod {
			return nil, nil
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, target, lg)
		if err != nil {
			lg.Warn("模型路由回退到基础模型",
				loggateway.StepID("agent.model_router.route"),
				loggateway.Str("provider", baseProv), loggateway.Str("target", target), loggateway.Str("base", baseMod), loggateway.Err(err))
			metrics.ModelRouterFallbackTotal.WithLabelValues("catalog_lookup").Inc()
			return nil, nil
		}
		return m, nil
	}
}

func invocationPrompt(inv *trpcagent.Invocation) string {
	if inv == nil {
		return ""
	}
	if c := strings.TrimSpace(inv.Message.Content); c != "" {
		return c
	}
	return lastUserPromptFromSession(inv)
}

func lastUserPromptFromSession(inv *trpcagent.Invocation) string {
	if inv == nil || inv.Session == nil {
		return ""
	}
	inv.Session.EventMu.RLock()
	defer inv.Session.EventMu.RUnlock()
	for i := len(inv.Session.Events) - 1; i >= 0; i-- {
		ev := inv.Session.Events[i]
		if strings.TrimSpace(ev.Author) != "user" {
			continue
		}
		if ev.Response != nil && len(ev.Response.Choices) > 0 {
			if t := strings.TrimSpace(ev.Response.Choices[0].Message.Content); t != "" {
				return t
			}
		}
	}
	return ""
}
