package team

import (
	"context"
	"fmt"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcteam "trpc.group/trpc-go/trpc-agent-go/team"
)

type TRPCTeamBuilderDeps struct {
	BuilderDeps chatagent.TRPCBuilderDeps
}

func BuildTRPCTeam(ctx context.Context, def Definition, deps TRPCTeamBuilderDeps, catalogAgent func(ctx context.Context, id string) (biz.Agent, error)) (trpcagent.Agent, error) {
	members := EnabledMembers(def)
	if len(members) == 0 {
		return nil, kerrors.BadRequest("TEAM", "no enabled members")
	}

	mode := strings.ToLower(strings.TrimSpace(def.Mode))

	memberAgents := make([]trpcagent.Agent, 0, len(members))
	for _, m := range members {
		ag, err := catalogAgent(ctx, strings.TrimSpace(m.AgentID))
		if err != nil {
			return nil, kerrors.BadRequest("TEAM", fmt.Sprintf("member %s: %v", m.AgentID, err))
		}
		trpcAg, err := chatagent.BuildTRPCLLMAgent(ctx, ag, deps.BuilderDeps)
		if err != nil {
			return nil, kerrors.InternalServer("TEAM", fmt.Sprintf("build member %s: %v", m.AgentID, err))
		}
		memberAgents = append(memberAgents, trpcAg)
	}

	switch mode {
	case "swarm":
		entryName := memberAgents[0].Info().Name
		t, err := trpcteam.NewSwarm(
			"team",
			entryName,
			memberAgents,
		)
		if err != nil {
			return nil, kerrors.InternalServer("TEAM", fmt.Sprintf("new swarm: %v", err))
		}
		return t, nil

	default:
		if len(memberAgents) < 2 {
			return memberAgents[0], nil
		}
		coordinator := memberAgents[0]
		rest := memberAgents[1:]
		t, err := trpcteam.New(
			coordinator,
			rest,
		)
		if err != nil {
			return nil, kerrors.InternalServer("TEAM", fmt.Sprintf("new coordinator: %v", err))
		}
		return t, nil
	}
}
