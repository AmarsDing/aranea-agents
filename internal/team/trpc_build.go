package team

import (
	"context"
	"fmt"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

type TRPCTeamBuilderDeps struct {
	BuilderDeps chatagent.TRPCBuilderDeps
	UseCache    bool
}

// BuildTeamMemberAgents builds member trpc agents and a runner lookup map keyed by agent_key.
func BuildTeamMemberAgents(
	ctx context.Context,
	def Definition,
	deps TRPCTeamBuilderDeps,
	lookupAgent func(ctx context.Context, id string) (biz.Agent, error),
	lg loggateway.Logger,
) ([]trpcagent.Agent, map[string]trpcagent.Agent, error) {
	members := EnabledMembers(def)
	memberAgents := make([]trpcagent.Agent, 0, len(members))
	lookup := make(map[string]trpcagent.Agent, len(members))
	for _, m := range members {
		ag, err := lookupAgent(ctx, strings.TrimSpace(m.AgentID))
		if err != nil {
			return nil, nil, apierror.BadRequest(apierror.DomainTeam, fmt.Sprintf("member %s: %v", m.AgentID, err))
		}
		var trpcAg trpcagent.Agent
		if deps.UseCache {
			trpcAg, err = chatagent.BuildTRPCAgentCached(ctx, ag, deps.BuilderDeps, lg)
		} else {
			trpcAg, err = chatagent.BuildTRPCAgent(ctx, ag, deps.BuilderDeps, lg)
		}
		if err != nil {
			return nil, nil, apierror.Internal(apierror.DomainTeam, fmt.Sprintf("build member %s: %v", m.AgentID, err))
		}
		memberAgents = append(memberAgents, trpcAg)
		if key := strings.TrimSpace(ag.AgentKey); key != "" {
			lookup[key] = trpcAg
		}
	}
	return memberAgents, lookup, nil
}
