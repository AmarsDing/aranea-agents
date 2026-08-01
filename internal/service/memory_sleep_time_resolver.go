package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/memory"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// SleepTimeAgentGetter is the narrow subset of biz.AgentUsecase needed by
// SleepTimeLLMResolver. Defined as an interface for testability.
type SleepTimeAgentGetter interface {
	Get(ctx context.Context, id string) (biz.Agent, error)
}

// SleepTimeLLMResolver resolves per-target LLM models for the Sleep-time
// consolidation worker (P1-1). It implements memory.LLMResolver.
//
// Model selection follows the same precedence as MemoryLLMExtractor:
// agent MemoryWorker settings → L0Compress settings → agent default
// provider/model. Session defaults are NOT used because sleep-time
// consolidation spans all of the user's sessions for an agent.
//
// All failures degrade gracefully to a nil model (the caller treats a nil
// LLM as a no-op), so a misconfigured agent never breaks the worker loop.
type SleepTimeLLMResolver struct {
	agents  SleepTimeAgentGetter
	catalog biz.TeamModelCatalog
	rt      *provider.RoundTrip
	lg      loggateway.Logger
}

// Compile-time check: SleepTimeLLMResolver satisfies the memory.LLMResolver port.
var _ memory.LLMResolver = (*SleepTimeLLMResolver)(nil)

// NewSleepTimeLLMResolver creates a SleepTimeLLMResolver.
//
// Parameters:
//   - agents:  agent lookup (typically *biz.AgentUsecase). May be nil.
//   - catalog: the model catalog used to build trpc models. May be nil.
//   - rt:      the HTTP transport for LLM calls. May be nil.
//   - lg:      the logger. Falls back to a no-op logger if nil.
func NewSleepTimeLLMResolver(agents SleepTimeAgentGetter, catalog biz.TeamModelCatalog, rt *provider.RoundTrip, lg loggateway.Logger) *SleepTimeLLMResolver {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SleepTimeLLMResolver{
		agents:  agents,
		catalog: catalog,
		rt:      rt,
		lg:      lg.With(loggateway.Domain("sleep_time_llm_resolver")),
	}
}

// ResolveProviderModel selects (provider, model) for the target's agent.
// Returns an error when the agent cannot be loaded; returns empty strings
// when the agent has no usable model configuration.
func (r *SleepTimeLLMResolver) ResolveProviderModel(ctx context.Context, uk trpcmemory.UserKey) (prov, mod string, err error) {
	ag, err := r.agents.Get(ctx, uk.AppName)
	if err != nil {
		return "", "", err
	}
	// Zero session: sleep-time consolidation spans all sessions of the
	// (agent, user) pair, so session-level defaults do not apply. The
	// agent-level precedence (MemoryWorker → L0Compress → agent default)
	// is identical to MemoryLLMExtractor.
	prov, mod = memoryWorkerProviderModel(biz.Session{}, ag)
	return prov, mod, nil
}

// ResolveLLM implements memory.LLMResolver. Returns nil on any failure
// (agent lookup error, no model configured, catalog/build failure) —
// graceful degradation so a single misconfigured target never breaks the
// consolidation worker.
func (r *SleepTimeLLMResolver) ResolveLLM(ctx context.Context, uk trpcmemory.UserKey) trpcmodel.Model {
	if r == nil || r.agents == nil || r.catalog == nil || r.rt == nil {
		return nil
	}
	prov, mod, err := r.ResolveProviderModel(ctx, uk)
	if err != nil {
		r.lg.Warn("sleep-time LLM resolve: agent lookup failed, consolidation will be no-op",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
		return nil
	}
	if prov == "" || mod == "" {
		return nil
	}
	m, err := provider.TRPCModelForProviderModel(ctx, r.catalog, r.rt, prov, mod, r.lg)
	if err != nil {
		r.lg.Warn("sleep-time LLM resolve: model build failed, consolidation will be no-op",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("provider", prov),
			loggateway.Str("model", mod),
			loggateway.Err(err))
		return nil
	}
	return m
}
