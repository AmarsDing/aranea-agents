package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
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
		return nil, kerrors.InternalServer("GRAPH", "graph: agent catalog not configured")
	}
	ag, err := r.resolveBizAgent(ctx, agentRef)
	if err != nil {
		return nil, err
	}
	return chatagent.BuildTRPCAgentCached(ctx, ag, r.Deps, r.lg)
}

func (r *CatalogAgentResolver) resolveBizAgent(ctx context.Context, agentRef string) (biz.Agent, error) {
	ref := strings.TrimSpace(agentRef)
	if ref == "" {
		return biz.Agent{}, kerrors.BadRequest("GRAPH", "graph: agent ref is required")
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
	return biz.Agent{}, kerrors.NotFound("GRAPH", fmt.Sprintf("graph: agent %q not found", ref))
}
