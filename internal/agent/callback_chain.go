package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// buildCallbackChainOptions wires product-layer Callback Chain into LLMAgent.
// Runner-level plugins (WithPlugins) handle DB builtins and OnEvent; see plugintrpc/orchestration.go.
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
	entries = append(entries, productChainLifecycleMetrics()...)

	if hook := newMemoryInjectBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}

	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		entries = append(entries, newToolArgsGuardBeforeHook())
		entries = append(entries, newToolResultCacheBeforeHook(deps))
		entries = append(entries, newToolCallTimingBeforeHook())
		if gate := buildToolConfirmGate(ctx, ag, deps); gate != nil {
			entries = append(entries, newToolConfirmationBeforeHook(gate, ag, deps))
		}
		entries = append(entries, callbacks.NewToolRecorderCallback(50, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
			recordToolInvocationAfter(ctx, args, ag, deps)
			return &trpctool.AfterToolResult{}, nil
		}))
		entries = append(entries, newToolResultCacheAfterHook(deps))
	}

	if len(entries) == 0 {
		return nil
	}
	return callbacks.NewChain(entries...)
}
