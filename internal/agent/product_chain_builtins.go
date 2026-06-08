package agent

import (
	"context"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/metrics"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// productChainLifecycleMetrics registers lightweight Prometheus hooks on every
// Agent/Model lifecycle point so empty hook chains still emit observability.
func productChainLifecycleMetrics() []callbacks.Callback {
	return []callbacks.Callback{
		callbacks.NewBeforeAgentHook(0, func(ctx context.Context, _ *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
			start := time.Now()
			metrics.PluginInvokeTotal.WithLabelValues("product_chain", "before_agent", "ok").Inc()
			metrics.ObserveCallback("product_chain", "before_agent", start, nil)
			return &trpcagent.BeforeAgentResult{Context: ctx}, nil
		}),
		callbacks.NewAfterAgentHook(0, func(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error) {
			start := time.Now()
			status := "ok"
			var cbErr error
			if args != nil && args.Error != nil {
				status = "error"
				cbErr = args.Error
			}
			metrics.PluginInvokeTotal.WithLabelValues("product_chain", "after_agent", status).Inc()
			metrics.ObserveCallback("product_chain", "after_agent", start, cbErr)
			return &trpcagent.AfterAgentResult{Context: ctx}, nil
		}),
		callbacks.NewBeforeModelHook(0, callbacks.LayerDynamic, func(ctx context.Context, _ *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
			start := time.Now()
			metrics.PluginInvokeTotal.WithLabelValues("product_chain", "before_model", "ok").Inc()
			metrics.ObserveCallback("product_chain", "before_model", start, nil)
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}),
		callbacks.NewAfterModelHook(0, func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
			start := time.Now()
			status := "ok"
			var cbErr error
			if args != nil && args.Error != nil {
				status = "error"
				cbErr = args.Error
			}
			metrics.PluginInvokeTotal.WithLabelValues("product_chain", "after_model", status).Inc()
			metrics.ObserveCallback("product_chain", "after_model", start, cbErr)
			return &trpcmodel.AfterModelResult{Context: ctx}, nil
		}),
	}
}
