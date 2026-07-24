package team

import (
	"context"
	"fmt"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	deliverabletools "aranea-agents/internal/tools/deliverable"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

type TRPCTeamBuilderDeps struct {
	BuilderDeps chatagent.TRPCBuilderDeps
	UseCache    bool
}

// BuildTeamMemberAgents builds member trpc agents and a runner lookup map keyed by agent_key.
// Per-member effective-tool, skill, and MCP version hashes are computed so that cache entries
// invalidate correctly when a member's tooling configuration changes.
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

		memberDeps := deps.BuilderDeps
		// C1/C3: when the team definition enables the deliverable state channel,
		// inject set_deliverable/get_deliverable tools into every member so
		// they can pass structured output via graph state.
		if def.EnableStateDeliverable {
			memberDeps.CustomTools = append(memberDeps.CustomTools, deliverabletools.Tools()...)
		}
		if eff, err := fetchEffectiveTools(ctx, memberDeps, ag.ID); err == nil {
			memberDeps.CachedEffectiveTools = eff
			memberDeps.ToolVersionHash = chatagent.ComputeToolVersionHash(eff)
		}
		memberDeps.SkillVersionHash = chatagent.ComputeSkillVersionHash(fetchEnabledSkillSlugs(ctx, memberDeps))
		memberDeps.MCPVersionHash = chatagent.ComputeMCPVersionHash(fetchEffectiveMCPServers(ctx, memberDeps, ag.ID))

		var trpcAg trpcagent.Agent
		if deps.UseCache {
			trpcAg, err = chatagent.BuildTRPCAgentCached(ctx, ag, memberDeps, lg)
		} else {
			trpcAg, err = chatagent.BuildTRPCAgent(ctx, ag, memberDeps, lg)
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

func fetchEffectiveTools(ctx context.Context, deps chatagent.TRPCBuilderDeps, agentID string) (*biz.AgentEffectiveTools, error) {
	if deps.AgentUC == nil || strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("agentUC not available")
	}
	eff, err := deps.AgentUC.GetEffectiveTools(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return &eff, nil
}

func fetchEnabledSkillSlugs(ctx context.Context, deps chatagent.TRPCBuilderDeps) []string {
	if deps.SkillUC == nil {
		return nil
	}
	slugs, err := deps.SkillUC.ListEnabledPublishedSkillKeys(ctx)
	if err != nil {
		return nil
	}
	return slugs
}

func fetchEffectiveMCPServers(ctx context.Context, deps chatagent.TRPCBuilderDeps, agentID string) []biz.EffectiveMCPServer {
	if deps.MCPTooling == nil || strings.TrimSpace(agentID) == "" {
		return nil
	}
	servers, err := deps.MCPTooling.EffectiveServersForAgent(ctx, agentID)
	if err != nil {
		return nil
	}
	return servers
}
