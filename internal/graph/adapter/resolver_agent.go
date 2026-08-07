package adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// CatalogAgentResolver builds catalog agents for graph agent nodes.
type CatalogAgentResolver struct {
	Deps chatagent.TRPCBuilderDeps
	lg   loggateway.Logger
	// MemberCustomTools injects per-member custom tools (e.g. cli_admin_* for
	// __system_admin__) into graph agent node builds — the same hook as
	// team.RunnerConfig.MemberCustomTools on the non-graph path. Without it,
	// agent-specific dep-backed tools are only assembled on the direct chat
	// path and a system_admin graph node would not see cli_admin_* in its LLM
	// tool list. Optional; when nil, nodes get only registry + static custom tools.
	MemberCustomTools func(ctx context.Context, ag biz.Agent) []trpctool.Tool
}

var _ graphtrpc.AgentResolver = (*CatalogAgentResolver)(nil)

func NewCatalogAgentResolver(deps chatagent.TRPCBuilderDeps, lg loggateway.Logger, memberCustomTools ...func(ctx context.Context, ag biz.Agent) []trpctool.Tool) *CatalogAgentResolver {
	r := &CatalogAgentResolver{Deps: deps, lg: lg}
	if len(memberCustomTools) > 0 {
		r.MemberCustomTools = memberCustomTools[0]
	}
	return r
}

// WithExtraCustomTools returns a clone of the resolver whose builder deps
// carry the extra CustomTools appended to the existing ones. The original
// resolver is left unmodified, so tool injection stays scoped to the graph
// that requested it (e.g. deliverable tools for EnableStateDeliverable teams).
func (r *CatalogAgentResolver) WithExtraCustomTools(tools ...trpctool.Tool) *CatalogAgentResolver {
	clone := *r
	clone.Deps.CustomTools = append(append([]trpctool.Tool{}, r.Deps.CustomTools...), tools...)
	return &clone
}

func (r *CatalogAgentResolver) ResolveAgent(ctx context.Context, agentRef string) (trpcagent.Agent, error) {
	if r == nil {
		return nil, apierror.Internal(apierror.DomainGraph, "graph: agent catalog not configured")
	}
	ag, err := r.resolveBizAgent(ctx, agentRef)
	if err != nil {
		return nil, err
	}

	deps := r.Deps
	// Per-member custom tools (cli_admin_* for __system_admin__, etc.) so
	// agent-specific dep-backed tools are available inside graph agent nodes
	// just as they are on the direct chat path. Appended before the build so
	// the cache key (customToolNames) reflects them.
	if r.MemberCustomTools != nil {
		deps.CustomTools = append(append([]trpctool.Tool{}, deps.CustomTools...), r.MemberCustomTools(ctx, ag)...)
	}
	if eff, err := r.fetchEffectiveTools(ctx, ag.ID); err == nil {
		deps.CachedEffectiveTools = eff
		deps.ToolVersionHash = chatagent.ComputeToolVersionHash(eff)
	}
	deps.SkillVersionHash = chatagent.ComputeSkillVersionHash(r.fetchEnabledSkillRefs(ctx))
	deps.MCPVersionHash = chatagent.ComputeMCPVersionHash(r.fetchEffectiveMCPServers(ctx, ag.ID))

	return chatagent.BuildTRPCAgentCached(ctx, ag, deps, r.lg)
}

func (r *CatalogAgentResolver) fetchEffectiveTools(ctx context.Context, agentID string) (*biz.AgentEffectiveTools, error) {
	if r.Deps.AgentUC == nil || strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("agentUC not available")
	}
	eff, err := r.Deps.AgentUC.GetEffectiveTools(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return &eff, nil
}

func (r *CatalogAgentResolver) fetchEnabledSkillRefs(ctx context.Context) []biz.SkillEnabledRef {
	if r.Deps.SkillUC == nil {
		return nil
	}
	refs, err := r.Deps.SkillUC.ListEnabledPublishedSkillRefs(ctx)
	if err != nil {
		return nil
	}
	return refs
}

func (r *CatalogAgentResolver) fetchEffectiveMCPServers(ctx context.Context, agentID string) []biz.EffectiveMCPServer {
	if r.Deps.MCPTooling == nil || strings.TrimSpace(agentID) == "" {
		return nil
	}
	servers, err := r.Deps.MCPTooling.EffectiveServersForAgent(ctx, agentID)
	if err != nil {
		return nil
	}
	return servers
}

func (r *CatalogAgentResolver) resolveBizAgent(ctx context.Context, agentRef string) (biz.Agent, error) {
	ref := strings.TrimSpace(agentRef)
	if ref == "" {
		return biz.Agent{}, apierror.BadRequest(apierror.DomainGraph, "graph: agent ref is required")
	}
	if r.Deps.AgentUC != nil {
		if ag, err := r.Deps.AgentUC.Get(ctx, ref); err == nil {
			return ag, nil
		} else if !errors.Is(err, shared.ErrNotFound) {
			return biz.Agent{}, err
		}
	}
	if r.Deps.Agents != nil {
		if ag, err := r.Deps.Agents.GetAgentByAgentKey(ctx, ref); err == nil {
			if r.Deps.AgentUC != nil {
				return r.Deps.AgentUC.Get(ctx, ag.ID)
			}
			return ag, nil
		} else if !errors.Is(err, shared.ErrNotFound) {
			return biz.Agent{}, err
		}
		if ag, err := r.Deps.Agents.GetAgentByID(ctx, ref); err == nil {
			if r.Deps.AgentUC != nil {
				return r.Deps.AgentUC.Get(ctx, ag.ID)
			}
			return ag, nil
		} else if !errors.Is(err, shared.ErrNotFound) {
			return biz.Agent{}, err
		}
	}
	return biz.Agent{}, apierror.NotFound(apierror.DomainGraph, fmt.Sprintf("graph: agent %q not found", ref))
}
