package agent

import (
	"context"

	"aranea-agents/internal/a2a/trpc"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// BuildTRPCAgent selects llmagent or a2aagent based on catalog agent kind.
func BuildTRPCAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
	biz.HydrateAgentKind(&ag)
	switch biz.NormalizeAgentKind(ag.Kind) {
	case biz.AgentKindA2AProxy:
		cfg := ag.A2AProxy
		if cfg == nil {
			return nil, kerrors.BadRequest("AGENT", "a2a_proxy config is required")
		}
		return trpc.BuildTRPCA2AAgent(ctx, ag, *cfg)
	default:
		return BuildTRPCLLMAgent(ctx, ag, deps)
	}
}

// BuildTRPCAgentCached wraps BuildTRPCAgent with LRU cache for LLM agents only.
func BuildTRPCAgentCached(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
	biz.HydrateAgentKind(&ag)
	if biz.IsA2AProxyAgent(ag) {
		return BuildTRPCAgent(ctx, ag, deps)
	}
	return BuildTRPCLLMAgentCached(ctx, ag, deps)
}
