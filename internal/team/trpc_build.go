package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

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
	UseCache    bool
}

// BuildTeamMemberAgents builds member trpc agents and a runner lookup map keyed by agent_key.
func BuildTeamMemberAgents(
	ctx context.Context,
	def Definition,
	deps TRPCTeamBuilderDeps,
	catalogAgent func(ctx context.Context, id string) (biz.Agent, error),
	lg loggateway.Logger,
) ([]trpcagent.Agent, map[string]trpcagent.Agent, error) {
	members := EnabledMembers(def)
	memberAgents := make([]trpcagent.Agent, 0, len(members))
	lookup := make(map[string]trpcagent.Agent, len(members))
	for _, m := range members {
		ag, err := catalogAgent(ctx, strings.TrimSpace(m.AgentID))
		if err != nil {
			return nil, nil, kerrors.BadRequest("TEAM", fmt.Sprintf("member %s: %v", m.AgentID, err))
		}
		var trpcAg trpcagent.Agent
		if deps.UseCache {
			trpcAg, err = chatagent.BuildTRPCAgentCached(ctx, ag, deps.BuilderDeps, lg)
		} else {
			trpcAg, err = chatagent.BuildTRPCAgent(ctx, ag, deps.BuilderDeps, lg)
		}
		if err != nil {
			return nil, nil, kerrors.InternalServer("TEAM", fmt.Sprintf("build member %s: %v", m.AgentID, err))
		}
		memberAgents = append(memberAgents, trpcAg)
		if key := strings.TrimSpace(ag.AgentKey); key != "" {
			lookup[key] = trpcAg
		}
	}
	return memberAgents, lookup, nil
}

// Deprecated: Team runs use the GraphAgent compile path by default (M53 Phase 7).
// BuildTRPCTeam is retained only for emergency fallback when ARANEA_TEAM_NATIVE=1.
func BuildTRPCTeam(ctx context.Context, def Definition, deps TRPCTeamBuilderDeps, catalogAgent func(ctx context.Context, id string) (biz.Agent, error), lg loggateway.Logger) (trpcagent.Agent, map[string]trpcagent.Agent, error) {
	members := EnabledMembers(def)
	if len(members) == 0 {
		return nil, nil, kerrors.BadRequest("TEAM", "no enabled members")
	}

	mode := strings.ToLower(strings.TrimSpace(def.Mode))

	memberAgents, lookup, err := BuildTeamMemberAgents(ctx, def, deps, catalogAgent, lg)
	if err != nil {
		return nil, nil, err
	}

	switch mode {
	case "sequential":
		return chainagent.New("team-sequential",
			chainagent.WithSubAgents(memberAgents),
		), lookup, nil

	case "parallel":
		return parallelagent.New("team-parallel",
			parallelagent.WithSubAgents(memberAgents),
		), lookup, nil

	case "critic_loop":
		maxIter := 3
		if def.CriticLoop != nil && def.CriticLoop.MaxIterations > 0 {
			maxIter = def.CriticLoop.MaxIterations
		}
		escFn := buildEscalationFunc(def.CriticLoop)
		return cycleagent.New("team-critic-loop",
			cycleagent.WithSubAgents(memberAgents),
			cycleagent.WithMaxIterations(maxIter),
			cycleagent.WithEscalationFunc(escFn),
		), lookup, nil

	case "swarm":
		root, err := buildSwarmTeam(def, memberAgents)
		return root, lookup, err

	case "adaptive":
		if len(memberAgents) < 2 {
			return memberAgents[0], lookup, nil
		}
		root, err := buildAdaptiveSwarm(def, memberAgents)
		return root, lookup, err

	default:
		return nil, nil, kerrors.BadRequest("TEAM", fmt.Sprintf("unsupported team mode %q for native fallback path", mode))
	}
}

func buildSwarmTeam(def Definition, memberAgents []trpcagent.Agent) (trpcagent.Agent, error) {
	entryName := memberAgents[0].Info().Name
	opts := buildSwarmOptions(def)
	t, err := trpcteam.NewSwarm("team", entryName, memberAgents, opts...)
	if err != nil {
		return nil, kerrors.InternalServer("TEAM", fmt.Sprintf("new swarm: %v", err))
	}
	return t, nil
}

func buildAdaptiveSwarm(def Definition, memberAgents []trpcagent.Agent) (trpcagent.Agent, error) {
	entryName := memberAgents[0].Info().Name
	opts := buildSwarmOptions(def)
	t, err := trpcteam.NewSwarm("team-adaptive", entryName, memberAgents, opts...)
	if err != nil {
		return nil, kerrors.InternalServer("TEAM", fmt.Sprintf("new adaptive swarm: %v", err))
	}
	return t, nil
}

func buildSwarmOptions(def Definition) []trpcteam.Option {
	cfg := trpcteam.DefaultSwarmConfig()
	crossTransfer := true
	if sc := def.Swarm; sc != nil {
		if sc.MaxHandoffs > 0 {
			cfg.MaxHandoffs = sc.MaxHandoffs
		}
		if sc.NodeTimeoutSeconds > 0 {
			cfg.NodeTimeout = time.Duration(sc.NodeTimeoutSeconds) * time.Second
		}
		if sc.RepetitiveHandoffWindow > 0 {
			cfg.RepetitiveHandoffWindow = sc.RepetitiveHandoffWindow
		}
		if sc.RepetitiveHandoffMinUnique > 0 {
			cfg.RepetitiveHandoffMinUnique = sc.RepetitiveHandoffMinUnique
		}
		crossTransfer = sc.CrossRequestTransfer
	}
	return []trpcteam.Option{
		trpcteam.WithSwarmConfig(cfg),
		trpcteam.WithCrossRequestTransfer(crossTransfer),
		trpcteam.WithSwarmHandoffInputBuilder(defaultSwarmHandoffInput),
	}
}

func buildCoordinatorOptions(def Definition) []trpcteam.Option {
	var opts []trpcteam.Option
	if mt := def.MemberTool; mt != nil {
		cfg := trpcteam.MemberToolConfig{
			StreamInner:      mt.StreamInner,
			SkipSummarization: mt.SkipSummarization,
		}
		cfg.InnerTextMode = mapInnerTextMode(mt.InnerTextMode)
		cfg.HistoryScope = mapHistoryScope(mt.HistoryScope)
		opts = append(opts, trpcteam.WithMemberToolConfig(cfg))
		if mt.ToolSetName != "" {
			opts = append(opts, trpcteam.WithMemberToolSetName(mt.ToolSetName))
		}
	}
	return opts
}

func mapInnerTextMode(s string) trpcteam.InnerTextMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "include":
		return trpcteam.InnerTextModeInclude
	case "exclude":
		return trpcteam.InnerTextModeExclude
	default:
		return trpcteam.InnerTextModeDefault
	}
}

func mapHistoryScope(s string) trpcteam.HistoryScope {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "isolated":
		return trpcteam.HistoryScopeIsolated
	case "parent_branch":
		return trpcteam.HistoryScopeParentBranch
	default:
		return trpcteam.HistoryScopeDefault
	}
}

func buildEscalationFunc(clc *CriticLoopConfig, lg loggateway.Logger) func(ev *trpcevent.Event) bool {
	if clc == nil || clc.ScoreThreshold <= 0 {
		return defaultEscalationFunc(lg)
	}
	threshold := clc.ScoreThreshold
	return func(ev *trpcevent.Event) bool {
		if ev == nil || ev.Response == nil {
			return false
		}
		for _, ch := range ev.Choices {
			for _, tc := range ch.Message.ToolCalls {
				if tc.Function.Name == biz.OrchestrationControlToolName {
					d, err := biz.ParseOrchestrationDecision(tc.Function.Arguments, lg)
					if err == nil && biz.IsApprovedDecision(d, threshold) {
						return true
					}
				}
			}
			content := strings.ToLower(ch.Message.Content)
			if strings.Contains(content, "approved") {
				return true
			}
			score := biz.ExtractScore(content)
			if score > 0 && score >= threshold {
				return true
			}
		}
		return false
	}
}

const OrchestrationControlToolSchema = `{
  "name": "orchestration_control",
  "description": "Signal orchestration decisions (approve, retry, escalate) during multi-agent workflows. Use this tool instead of writing the decision in plain text.",
  "parameters": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "enum": ["approve", "retry", "escalate"],
        "description": "The orchestration decision: approve to accept and move forward, retry to request another iteration, escalate to flag for human review."
      },
      "score": {
        "type": "number",
        "description": "Optional quality score (0.0-1.0). If provided and above the configured threshold, the decision is treated as approved."
      },
      "reason": {
        "type": "string",
        "description": "Brief explanation for the decision."
      }
    },
    "required": ["action"]
  }
}`

func defaultEscalationFunc(lg loggateway.Logger) func(ev *trpcevent.Event) bool {
	return func(ev *trpcevent.Event) bool {
	if ev == nil || ev.Response == nil {
		return false
	}
	for _, ch := range ev.Choices {
		for _, tc := range ch.Message.ToolCalls {
			if tc.Function.Name == biz.OrchestrationControlToolName {
				d, err := biz.ParseOrchestrationDecision(tc.Function.Arguments)
				if err == nil && d.Action == "approve" {
					return true
				}
			}
		}
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
