package trpcmem

import (
	"context"

	"aranea-agents/internal/biz"
)

type AgentRuntimeSettingsLoader interface {
	GetAgentRuntimeSettings(ctx context.Context, agentID string) (*biz.AgentRuntimeSettings, error)
}

type AgentSettingsGetter interface {
	Get(ctx context.Context, id string) (biz.Agent, error)
}

type agentRuntimeSettingsLoader struct {
	agents AgentSettingsGetter
}

func NewAgentRuntimeSettingsLoader(agents AgentSettingsGetter) AgentRuntimeSettingsLoader {
	if agents == nil {
		return nil
	}
	return &agentRuntimeSettingsLoader{agents: agents}
}

func (l *agentRuntimeSettingsLoader) GetAgentRuntimeSettings(ctx context.Context, agentID string) (*biz.AgentRuntimeSettings, error) {
	if l == nil || l.agents == nil {
		return nil, nil
	}
	ag, err := l.agents.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return ag.Settings, nil
}

func resolveMemoryToolSearchLimits(ctx context.Context, loader AgentRuntimeSettingsLoader, agentID string, optsMax int32) (topK int32, minScore float64) {
	policy := biz.ResolveMemoryRuntimePolicy(nil)
	topK = int32(policy.MemoryToolMaxResults)
	minScore = policy.MemoryToolMinScore
	if loader != nil {
		if settings, err := loader.GetAgentRuntimeSettings(ctx, agentID); err == nil && settings != nil {
			policy = biz.ResolveMemoryRuntimePolicy(settings)
		}
	}
	if !policy.MasterEnabled {
		// MasterEnabled=false: return sentinel values that callers must respect.
		// optsMax cannot override this — policy takes absolute precedence.
		return 0, 1
	}
	if policy.MemoryToolMaxResults > 0 {
		topK = int32(policy.MemoryToolMaxResults)
	}
	if policy.MemoryToolMinScore > 0 {
		minScore = policy.MemoryToolMinScore
	}
	// optsMax may further reduce topK but cannot exceed the policy ceiling.
	if optsMax > 0 && optsMax < topK {
		topK = optsMax
	}
	if topK <= 0 {
		topK = 10
	}
	return topK, minScore
}
