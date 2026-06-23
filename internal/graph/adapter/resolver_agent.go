package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// CatalogAgentResolver builds catalog agents for graph agent nodes.
type CatalogAgentResolver struct {
	Deps chatagent.TRPCBuilderDeps
	lg   loggateway.Logger
}

var _ graphtrpc.AgentResolver = (*CatalogAgentResolver)(nil)

func NewCatalogAgentResolver(deps chatagent.TRPCBuilderDeps, lg loggateway.Logger) *CatalogAgentResolver {
	return &CatalogAgentResolver{Deps: deps, lg: lg}
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
	if eff, err := r.fetchEffectiveTools(ctx, ag.ID); err == nil {
		deps.CachedEffectiveTools = eff
		deps.ToolVersionHash = chatagent.ComputeToolVersionHash(eff)
	}
	deps.SkillVersionHash = chatagent.ComputeSkillVersionHash(r.fetchEnabledSkillSlugs(ctx))
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

func (r *CatalogAgentResolver) fetchEnabledSkillSlugs(ctx context.Context) []string {
	if r.Deps.SkillUC == nil {
		return nil
	}
	slugs, err := r.Deps.SkillUC.ListEnabledPublishedSkillKeys(ctx)
	if err != nil {
		return nil
	}
	return slugs
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
		} else if !errors.Is(err, sql.ErrNoRows) {
			return biz.Agent{}, err
		}
	}
	if r.Deps.Agents != nil {
		if ag, err := r.Deps.Agents.GetAgentByAgentKey(ctx, ref); err == nil {
			if r.Deps.AgentUC != nil {
				return r.Deps.AgentUC.Get(ctx, ag.ID)
			}
			return ag, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return biz.Agent{}, err
		}
		if ag, err := r.Deps.Agents.GetAgentByID(ctx, ref); err == nil {
			if r.Deps.AgentUC != nil {
				return r.Deps.AgentUC.Get(ctx, ag.ID)
			}
			return ag, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return biz.Agent{}, err
		}
	}
	return biz.Agent{}, apierror.NotFound(apierror.DomainGraph, fmt.Sprintf("graph: agent %q not found", ref))
}
