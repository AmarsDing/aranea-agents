package team

import (
	"context"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
)

// TeamRunFinishInput captures post-stream persistence inputs shared by Native and Graph paths.
type TeamRunFinishInput struct {
	Run            biz.TeamRunRecord
	TeamID         string
	DefinitionJSON string
	Content        string
	AssistantMsg   biz.ChatMessage
	Result         agent.EventStreamResult
	PromptTok      int
	CompletionTok  int
	// UsageSource records how PromptTok/CompletionTok were obtained
	// ("streaming"/"runner_completion"/"estimated"); threaded into the
	// team_turn / anchor-fallback team_member usage event metadata.
	UsageSource    string
	Prov           string
	Mod            string
	DialogMode     string
	GraphExecID    string
	AnchorMem      MemberDef
	AnchorAg       biz.Agent
}

// finalizeGraphRunStepsFallback ensures at least one step exists when graph events produced none.
// suppressUsageRow is threaded from recordGraphMemberUsageFromResult: when
// stream-derived member usage rows already carry this run's billable usage,
// the fallback anchor row is persisted display-only (P2-1b 双计守卫).
func (r *Runner) finalizeGraphRunStepsFallback(ctx context.Context, in TeamRunFinishInput, suppressUsageRow bool) {
	if r == nil || in.GraphExecID == "" {
		return
	}
	r.ensureGraphRunStepsFallback(ctx, in.Run, in.TeamID, in.AnchorMem, in.AnchorAg, in.Content, in.AssistantMsg, in.PromptTok, in.CompletionTok, in.Result.CachedTok, in.Prov, in.Mod, in.UsageSource, suppressUsageRow)
}
