package team

import (
	"context"
	"strings"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// cascadeLeaderAgentKeys resolves the leader/planner tier agent keys for model
// cascade (P2-1): the synthesizer (final aggregation) and the intent anchor
// (coordinator/planner) keep their configured high-tier model. Both are
// resolved from agent IDs to keys via lookupAgent; failures are skipped with a
// warn — a cascade misconfiguration must never fail the run.
func cascadeLeaderAgentKeys(
	ctx context.Context,
	def Definition,
	lookupAgent func(ctx context.Context, id string) (biz.Agent, error),
	lg loggateway.Logger,
) []string {
	ids := make([]string, 0, 2)
	if id := strings.TrimSpace(SynthesizerAgentID(def)); id != "" {
		ids = append(ids, id)
	}
	if id := strings.TrimSpace(def.IntentAnchorAgentID); id != "" {
		ids = append(ids, id)
	}
	keys := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ag, err := lookupAgent(ctx, id)
		if err != nil {
			lg.Warn("级联 leader 解析失败，已跳过",
				loggateway.StepID("team.model_cascade.route"),
				loggateway.Str("agent_id", id), loggateway.Err(err))
			continue
		}
		if key := strings.TrimSpace(ag.AgentKey); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// cascadeRunOption builds the run-level cascade ModelSelector option for a
// team turn. Returns nil when the definition has no model_cascade config.
// The option rides the root RunOptions; agent-node invocations clone the
// parent's RunOptions, so every member invocation applies the selector with
// its own AgentName while leaders (synthesizer/intent anchor) keep base.
func (r *Runner) cascadeRunOption(ctx context.Context, def Definition, sessID string) trpcagent.RunOption {
	mc := def.ModelCascade
	if mc == nil {
		return nil
	}
	leaderKeys := cascadeLeaderAgentKeys(ctx, def, r.lookupAgent, r.lg)
	sel := agent.CascadeModelSelector(
		leaderKeys,
		mc.MemberProvider, mc.MemberModel,
		r.td.ReadDeps.LLM,
		r.td.RoundTripForSession(sessID),
		r.lg,
	)
	return trpcagent.WithModelSelector(sel)
}
