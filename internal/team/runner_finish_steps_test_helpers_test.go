package team

import (
	"context"

	"aranea-agents/pkg/loggateway"
)

// persistGraphMemberStepsFromResultTestOnly is a test-harness helper used by parity_run_e2e_test.go
// to simulate the per-member step writes that the coordinator watch performs via PersistGraphRunStep
// in production. It is intentionally NOT called from production code paths.
func (r *Runner) persistGraphMemberStepsFromResultTestOnly(ctx context.Context, in TeamRunFinishInput, def Definition) {
	if r == nil || in.GraphExecID == "" {
		return
	}
	stepCtx := buildGraphRunStepContext(in.DefinitionJSON, in.Content, in.Run.ID, in.TeamID, in.Run.SessionID, in.Run.SessionID, loggateway.NewNoop())
	if stepCtx == nil {
		return
	}
	members := EnabledMembers(def)
	for i, m := range members {
		nodeID := memberNodeID(m, i)
		if stepCtx.AlreadyPersisted(nodeID) {
			continue
		}
		ag, err := r.lookupAgent(ctx, m.AgentID)
		if err != nil {
			continue
		}
		stepMsg := in.AssistantMsg
		var cachedTok int
		stepMsg.TokenIn, stepMsg.TokenOut, cachedTok = stepTokensForMember(ag.AgentKey, i, in.Result, in.PromptTok, in.CompletionTok, in.Result.CachedTok)
		toolCalls := 0
		if in.Result.MemberToolCalls != nil {
			toolCalls = in.Result.MemberToolCalls[ag.AgentKey]
		}
		r.persistStep(ctx, in.Run, in.TeamID, stepCtx.SortIndex(nodeID), m, ag, in.Content, stepMsg, in.Prov, in.Mod, in.DialogMode, toolCalls, cachedTok)
		stepCtx.MarkPersisted(nodeID)
	}
}
