package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/event"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/provider"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// PluginCostGuardSelector returns a ModelSelector that switches to fallback when the base model is blocked.
func PluginCostGuardSelector(
	baseProv, baseMod string,
	catalog *biz.LlmProviderModelUsecase,
	rt *provider.RoundTrip,
	cfg plugintrpc.CostGuardConfig,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		target := plugintrpc.ResolveCostGuardFallbackModel(baseMod, cfg)
		if target == "" || target == baseMod {
			return nil, nil
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, target)
		if err != nil {
			event.CtxFlowLogWarn(ctx, "plugin.cost_guard.block", "费用保护回退到基础模型",
				event.P("provider", baseProv), event.P("target", target), event.P("base", baseMod), event.P("error", err))
			metrics.ModelRouterFallbackTotal.WithLabelValues("cost_guard_catalog").Inc()
			return nil, nil
		}
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
	catalog *biz.LlmProviderModelUsecase,
	rt *provider.RoundTrip,
	cfg plugintrpc.ModelRouterConfig,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		prompt := invocationPrompt(inv)
		target := plugintrpc.ResolveModelAPI(prompt, cfg)
		if target == "" || target == baseMod {
			return nil, nil
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, target)
		if err != nil {
			event.CtxFlowLogWarn(ctx, "plugin.model_router.route", "模型路由回退到基础模型",
				event.P("provider", baseProv), event.P("target", target), event.P("base", baseMod), event.P("error", err))
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
