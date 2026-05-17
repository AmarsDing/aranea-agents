package team

import (
	"context"
	"fmt"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/chainagent"
	"trpc.group/trpc-go/trpc-agent-go/agent/cycleagent"
	"trpc.group/trpc-go/trpc-agent-go/agent/parallelagent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcteam "trpc.group/trpc-go/trpc-agent-go/team"
)

type TRPCTeamBuilderDeps struct {
	BuilderDeps chatagent.TRPCBuilderDeps
	UseCache    bool // enable agent build cache for team members
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
		var trpcAg trpcagent.Agent
		if deps.UseCache {
			trpcAg, err = chatagent.BuildTRPCLLMAgentCached(ctx, ag, deps.BuilderDeps)
		} else {
			trpcAg, err = chatagent.BuildTRPCLLMAgent(ctx, ag, deps.BuilderDeps)
		}
		if err != nil {
			return nil, kerrors.InternalServer("TEAM", fmt.Sprintf("build member %s: %v", m.AgentID, err))
		}
		memberAgents = append(memberAgents, trpcAg)
	}

	switch mode {
	case "sequential":
		return chainagent.New("team-sequential",
			chainagent.WithSubAgents(memberAgents),
		), nil

	case "parallel":
		return parallelagent.New("team-parallel",
			parallelagent.WithSubAgents(memberAgents),
		), nil

	case "critic_loop":
		maxIter := 3
		if def.CriticLoop != nil && def.CriticLoop.MaxIterations > 0 {
			maxIter = def.CriticLoop.MaxIterations
		}
		return cycleagent.New("team-critic-loop",
			cycleagent.WithSubAgents(memberAgents),
			cycleagent.WithMaxIterations(maxIter),
			cycleagent.WithEscalationFunc(defaultEscalationFunc),
		), nil

	case "swarm":
		entryName := memberAgents[0].Info().Name
		opts := []trpcteam.Option{
			trpcteam.WithSwarmConfig(trpcteam.DefaultSwarmConfig()),
			trpcteam.WithCrossRequestTransfer(true),
			trpcteam.WithSwarmHandoffInputBuilder(defaultSwarmHandoffInput),
		}
		t, err := trpcteam.NewSwarm(
			"team",
			entryName,
			memberAgents,
			opts...,
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

func defaultEscalationFunc(ev *trpcevent.Event) bool {
	if ev == nil || ev.Response == nil {
		return false
	}
	for _, ch := range ev.Choices {
		if strings.Contains(strings.ToLower(ch.Message.Content), "approved") {
			return true
		}
	}
	return false
}

func defaultSwarmHandoffInput(ctx context.Context, args trpcteam.SwarmHandoffInputArgs) (trpcmodel.Message, error) {
	transferMsg := strings.TrimSpace(args.TransferMessage)
	if transferMsg == "" {
		return args.RootInput, nil
	}
	rootContent := strings.TrimSpace(args.RootInput.Content)
	if rootContent != "" {
		combined := fmt.Sprintf("[Original request]: %s\n\n[Handoff from %s]: %s", rootContent, args.FromAgentName, transferMsg)
		return trpcmodel.NewUserMessage(combined), nil
	}
	return trpcmodel.NewUserMessage(transferMsg), nil
}
