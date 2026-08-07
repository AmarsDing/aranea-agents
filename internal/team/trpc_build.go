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
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type TRPCTeamBuilderDeps struct {
	BuilderDeps chatagent.TRPCBuilderDeps
	UseCache    bool
	// MemberCustomTools injects per-member custom tools (sourced from
	// RunnerConfig.MemberCustomTools). Called once per member during
	// BuildTeamMemberAgents; results are appended to memberDeps.CustomTools
	// before effective-tool hashing so the cache key reflects them.
	MemberCustomTools func(ctx context.Context, ag biz.Agent) []trpctool.Tool
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
		// inject set/get/ack deliverable tools into every member so they can
		// pass structured output via graph state. MDC: a declared
		// deliverable_contract is installed on set_deliverable.
		memberDeps.CustomTools = append(memberDeps.CustomTools, deliverableToolsForDef(def)...)
		// Per-member custom tools (cli_admin_* for __system_admin__, etc.) so
		// agent-specific dep-backed tools are available inside teams just as
		// they are on the direct chat path.
		if deps.MemberCustomTools != nil {
			memberDeps.CustomTools = append(memberDeps.CustomTools, deps.MemberCustomTools(ctx, ag)...)
		}
		if eff, err := fetchEffectiveTools(ctx, memberDeps, ag.ID); err == nil {
			memberDeps.CachedEffectiveTools = eff
			memberDeps.ToolVersionHash = chatagent.ComputeToolVersionHash(eff)
		}
		memberDeps.SkillVersionHash = chatagent.ComputeSkillVersionHash(fetchEnabledSkillRefs(ctx, memberDeps))
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

// deliverableToolsForDef resolves the deliverable tool set for member agents:
// nil when the state channel is disabled; otherwise set/get/ack with the
// optional member-level deliverable contract installed on set_deliverable.
func deliverableToolsForDef(def Definition) []trpctool.Tool {
	if !def.EnableStateDeliverable {
		return nil
	}
	return deliverabletools.ToolsWithContract(def.DeliverableContract)
}

// parallelDeliverableAdvisory returns the advisory warning for running a
// deliverable-enabled team in parallel mode ("" = no advisory). Topic-scoped
// writes are merge-safe under MergeReducer; whole-map (no-topic) writes in
// the same superstep remain last-writer-wins, so parallel members should
// always write via distinct topics.
func parallelDeliverableAdvisory(def Definition) string {
	if def.EnableStateDeliverable && strings.EqualFold(strings.TrimSpace(def.Mode), "parallel") {
		return "parallel mode with enable_state_deliverable: members must write via distinct topics " +
			"(set_deliverable topic); whole-map writes without topic are last-writer-wins in the same superstep"
	}
	return ""
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

func fetchEnabledSkillRefs(ctx context.Context, deps chatagent.TRPCBuilderDeps) []biz.SkillEnabledRef {
	if deps.SkillUC == nil {
		return nil
	}
	refs, err := deps.SkillUC.ListEnabledPublishedSkillRefs(ctx)
	if err != nil {
		return nil
	}
	return refs
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
