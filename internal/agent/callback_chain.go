package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// buildCallbackChainOptions wires product-layer Callback Chain into LLMAgent.
// Runner-level plugins (WithPlugins) handle DB builtins and OnEvent; see plugintrpc/orchestration.go.
func buildCallbackChainOptions(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) ([]trpcllmagent.Option, *biztool.CircuitBreakerRegistry) {
	chain, cbRegistry := productCallbackChainWithRegistry(ctx, ag, deps)
	if chain == nil {
		return nil, nil
	}
	if deps.PluginManager != nil {
		chain = deps.PluginManager.MergeChain(ctx, ag.ID, ag.AgentKey, chain)
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
	return opts, cbRegistry
}

func productCallbackChain(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) *callbacks.Chain {
	chain, _ := productCallbackChainWithRegistry(ctx, ag, deps)
	return chain
}

func productCallbackChainWithRegistry(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (*callbacks.Chain, *biztool.CircuitBreakerRegistry) {
	var entries []callbacks.Callback
	lg := deps.Logger()
	entries = append(entries, productChainLifecycleMetrics()...)

	if hook := newStaticRuntimeCueBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newDynamicRuntimeCueBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newContextCompressionBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newSkillGuidanceBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newMemoryInjectBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newWorkingMemoryContextBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newMemoryEditContextBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newCompactContextBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newToolResultGateBeforeHook(deps.ToolResultGate, ag, lg); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newKnowledgeCueBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	if hook := newPromptSnapshotBeforeHook(ag, deps); hook != nil {
		entries = append(entries, hook)
	}
	// Problem 6: inject a reply reminder after each tool call so the LLM
	// outputs a brief "已完成 + 下一步" reply before calling the next tool.
	// BeforeModel hook reads state set by the AfterTool hook.
	entries = append(entries, newReplyReminderBeforeHook())
	if hook := newL0SnapshotAfterModelHook(deps); hook != nil {
		entries = append(entries, hook)
	}
	entries = append(entries, newTokenUsageAccumulatorAfterHook())

	var cbRegistry *biztool.CircuitBreakerRegistry
	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		// Tool execution timeout: inject BeforeTool + AfterTool hooks that
		// enforce a per-tool timeout via context.WithTimeout. This is the
		// product-layer implementation since the framework lacks built-in timeout.
		if timeoutHooks := toolExecutionTimeoutHooks(buildToolExecutionTimeout(ag.Settings), lg); len(timeoutHooks) > 0 {
			entries = append(entries, timeoutHooks...)
		}
		entries = append(entries, newToolArgsRepairBeforeHook(lg))
		entries = append(entries, newTodoArgsGuardBeforeHook(lg))
		entries = append(entries, newToolArgsGuardBeforeHook(lg))
		entries = append(entries, newToolResultCacheBeforeHook(deps))
		entries = append(entries, newToolCallTimingBeforeHook())
		if gate := buildToolConfirmGate(ctx, ag, deps); gate != nil {
			entries = append(entries, newToolConfirmationBeforeHook(gate, ag, deps))
		}
		// Capture skill_load/skill_run slug into invocation state BEFORE the
		// tool recorder reads it (recorder runs at priority 50).
		entries = append(entries, newSkillLoadCaptureAfterHook())
		// Problem 6: set reply-reminder state after each tool call so the
		// BeforeModel hook can inject a reminder system message.
		entries = append(entries, newReplyReminderAfterHook())
		entries = append(entries, callbacks.NewToolRecorderCallback(50, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
			recordToolInvocationAfter(ctx, args, ag, deps)
			return &trpctool.AfterToolResult{}, nil
		}))
		entries = append(entries, newToolResultCacheAfterHook(deps))
		// Side-effect feedback: remind the LLM (via tool results) when files
		// were modified without a subsequent test run. The BeforeAgent hook
		// pre-creates a per-invocation ToolReminder so concurrent sessions
		// sharing a cached Agent never share reminder state.
		entries = append(entries, newToolReminderBeforeAgentHook())
		entries = append(entries, newToolReminderAfterHook())
		if ag.Settings.ToolsCircuitBreakerEnabled {
			cbRegistry = buildCircuitBreakerRegistry(ag.Settings, lg)
			entries = append(entries, newCircuitBreakerBeforeHook(cbRegistry, lg))
			entries = append(entries, newCircuitBreakerAfterHook(cbRegistry, lg))
		}
		if ag.Settings.ToolsCommandSafetyEnabled {
			entries = append(entries, newCommandSafetyBeforeHook(lg))
		}
		// Output size limiter: truncate oversized tool results to prevent
		// context window overflow. Runs after the tool recorder (priority 50)
		// so the original size is logged before truncation.
		entries = append(entries, newOutputSizeLimiterAfterHook(lg))
	}

	if len(entries) == 0 {
		return nil, nil
	}
	return callbacks.NewChain(entries...), cbRegistry
}
