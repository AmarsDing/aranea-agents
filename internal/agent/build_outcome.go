package agent

import (
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/tools/deferred"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// LLMAgentBuildOutcome is the result of building a TRPC LLM agent.
//
// The function is intentionally side-effect free: it does not mutate the
// caller's TRPCBuilderDeps. Any dependency that the build derives at runtime
// (deferred tool manager, circuit breaker registry, …) is captured here so
// downstream callers can install, reset, or otherwise manage it.
//
// This replaces the historical "deps.DeferredManager = …" mutations inside
// BuildTRPCLLMAgent, which were both invisible to the caller (deps is
// passed by value) and a footgun for future signature changes.
type LLMAgentBuildOutcome struct {
	// Agent is the fully assembled trpc-agent-go agent, ready to be wrapped
	// in a Runner.
	Agent trpcagent.Agent

	// DeferredManager is the deferred-tool visibility manager produced by
	// buildToolsetsForAgent. Nil when the agent has no deferred tools.
	// Callers can install reset hooks / admin access on this manager.
	DeferredManager *deferred.DeferredToolManager

	// CircuitBreakerRegistry exposes the per-tool circuit breakers wired by
	// buildCallbackChainOptions. Nil when no circuit breakers are configured.
	// Service-layer admin endpoints can use this to reset state.
	CircuitBreakerRegistry *biztool.CircuitBreakerRegistry
}
