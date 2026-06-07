package team

import (
	"context"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
)

// TeamRunFinishInput captures post-stream persistence inputs shared by Native and Graph paths.
type TeamRunFinishInput struct {
	Run            biz.TeamRun
	TeamID         string
	DefinitionJSON string
	Content        string
	AssistantMsg   biz.ChatMessage
	Result         agent.EventStreamResult
	PromptTok      int
	CompletionTok  int
	Prov           string
	Mod            string
	DialogMode     string
	GraphExecID    string
	AnchorMem      MemberDef
	AnchorAg       biz.Agent
}

// persistNativeBulkMemberSteps writes one team_run_step per enabled member (Native bulk path).
// Deprecated: Native path is emergency-only (ARANEA_TEAM_NATIVE=1). Will be removed when Native is fully retired (BL-05).
func (r *Runner) persistNativeBulkMemberSteps(ctx context.Context, in TeamRunFinishInput, members []MemberDef) {
	if r == nil || in.GraphExecID != "" {
		return
	}
	for i, m := range members {
		ag, err := r.lookupAgent(ctx, m.AgentID)
		if err != nil {
			continue
		}
		stepMsg := in.AssistantMsg
		stepMsg.TokenIn, stepMsg.TokenOut = stepTokensForMember(ag.AgentKey, i, in.Result, in.PromptTok, in.CompletionTok)
		toolCalls := 0
		if in.Result.MemberToolCalls != nil {
			toolCalls = in.Result.MemberToolCalls[ag.AgentKey]
		}
		r.persistStep(ctx, in.Run, in.TeamID, i, m, ag, in.Content, stepMsg, in.Prov, in.Mod, in.DialogMode, toolCalls)
	}
}

// finalizeGraphRunStepsFallback ensures at least one step exists when graph events produced none.
func (r *Runner) finalizeGraphRunStepsFallback(ctx context.Context, in TeamRunFinishInput) {
	if r == nil || in.GraphExecID == "" {
		return
	}
	r.ensureGraphRunStepsFallback(ctx, in.Run, in.TeamID, in.AnchorMem, in.AnchorAg, in.Content, in.AssistantMsg, in.PromptTok, in.CompletionTok)
}
