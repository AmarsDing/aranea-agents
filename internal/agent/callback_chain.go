package agent

import (
	"context"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// buildCallbackChainOptions wires product-layer Callback Chain into LLMAgent.
// Runner-level plugins (WithPlugins) still handle invocation.Plugins hooks and OnEvent.
func buildCallbackChainOptions(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) []trpcllmagent.Option {
	chain := buildProductCallbackChain(ctx, ag, deps)
	if chain == nil {
		return nil
	}
	var opts []trpcllmagent.Option
	if chain.HasAgentHooks() {
		opts = append(opts, trpcllmagent.WithAgentCallbacks(chain.AdaptAgentCallbacks()))
	}
	if chain.HasModelHooks() {
		opts = append(opts, trpcllmagent.WithModelCallbacks(chain.AdaptModelCallbacks()))
	}
	if chain.HasToolHooks() {
		opts = append(opts, trpcllmagent.WithToolCallbacks(chain.AdaptToolCallbacks()))
	}
	return opts
}

// buildProductCallbackChain assembles the product chain plus optional hook rules from PluginManager.
func buildProductCallbackChain(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) *callbacks.Chain {
	chain := productCallbackChain(ctx, ag, deps)
	if deps.PluginManager != nil {
		chain = deps.PluginManager.MergeChain(ctx, ag.ID, ag.AgentKey, chain)
	}
	return chain
}

func productCallbackChain(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) *callbacks.Chain {
	var entries []callbacks.Callback

	entries = append(entries,
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
		callbacks.NewBeforeModelHook(0, func(ctx context.Context, _ *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
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
	)

	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		entries = append(entries, newToolCallTimingBeforeHook())
		confirmPolicy := buildToolConfirmationPolicy(ctx, ag, deps)
		if len(confirmPolicy) > 0 {
			entries = append(entries, newToolConfirmationBeforeHook(confirmPolicy, ag, deps))
		}
		entries = append(entries, callbacks.NewToolRecorderCallback(50, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
			recordToolInvocationAfter(ctx, args, ag, deps)
			return &trpctool.AfterToolResult{}, nil
		}))
	}

	if len(entries) == 0 {
		return nil
	}
	return callbacks.NewChain(entries...)
}
