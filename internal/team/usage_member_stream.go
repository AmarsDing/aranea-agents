package team

import (
	"context"
	"strings"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
)

// recordGraphMemberUsageFromResult persists genuine per-member usage rows from
// the stream's MemberUsage map (P2-1b, 2026-08-19).
//
// Background: graph watch persists per-member TeamRunSteps with ZERO tokens
// (node_end notices carry no usage), so recordMemberUsage skips them and the
// only usage row was the team_turn row — which billable aggregates exclude by
// design (sqlUsageBillableKind). Result: a watch-healthy team run was
// INVISIBLE to hourly/daily/breakdown/quota aggregates. The stream consumer
// already collects per-member usage (turn_helpers accumulateStreamUsage);
// this finish-path pass finally persists it.
//
// Rows written here are completion-path rows: the team_turn row remains the
// single session-metrics accumulator, so every row carries a usage_attribution
// marker and skips session accumulation (same anti-double-count rule as the
// anchor-fallback row). Per-member provider/model is resolved from each member
// agent so pricing stays correct when members use different models.
//
// Returns true when at least one row was written — the caller uses it to
// suppress the anchor-fallback usage row (which would double-count the same
// run totals in billable aggregates).
func (r *Runner) recordGraphMemberUsageFromResult(ctx context.Context, in TeamRunFinishInput) bool {
	if r == nil || r.usage == nil || len(in.Result.MemberUsage) == 0 {
		return false
	}
	def, err := ParseDefinition(in.DefinitionJSON)
	if err != nil {
		return false
	}
	// Best-effort step-ID resolution so usage rows link their TeamRunStep via
	// MessageID. Failure degrades to run-scoped rows (MessageID = run.ID),
	// matching the team_turn row's convention.
	stepIDByAgentKey := map[string]string{}
	if r.runReader != nil {
		if steps, serr := r.runReader.ListTeamRunSteps(ctx, in.Run.ID); serr == nil {
			for _, s := range steps {
				key := strings.TrimSpace(s.AgentKey)
				if key == "" {
					continue
				}
				if _, ok := stepIDByAgentKey[key]; !ok {
					stepIDByAgentKey[key] = s.ID
				}
			}
		}
	}
	sumIn, sumOut, sumCached := 0, 0, 0
	wrote := false
	consumed := map[string]bool{} // duplicate agent keys across members share one MemberUsage entry
	for _, m := range EnabledMembers(def) {
		ag, aerr := r.lookupAgent(ctx, m.AgentID)
		if aerr != nil {
			continue
		}
		key := strings.TrimSpace(ag.AgentKey)
		if key == "" || consumed[key] {
			continue
		}
		u, ok := in.Result.MemberUsage[key]
		if !ok || (u.PromptTokens <= 0 && u.CompletionTokens <= 0) {
			continue
		}
		consumed[key] = true
		asst := in.AssistantMsg
		asst.TokenIn, asst.TokenOut = u.PromptTokens, u.CompletionTokens
		prov := strutil.FirstNonEmpty(strings.TrimSpace(ag.Provider), strings.TrimSpace(in.Prov))
		mod := strutil.FirstNonEmpty(strings.TrimSpace(ag.Model), strings.TrimSpace(in.Mod))
		stepID := stepIDByAgentKey[key]
		if stepID == "" {
			stepID = in.Run.ID
		}
		r.recordMemberUsage(ctx, in.Run, in.TeamID, ag, asst, prov, mod, in.DialogMode, stepID, u.CachedTokens, agent.UsageSourceStreaming, biz.UsageAttributionMemberLevelStream)
		// Backfill the graph-watch step row with real tokens (T3 fix).
		if r.runWriter != nil && stepID != in.Run.ID {
			cost := int64(0)
			if r.usage != nil {
				cost = r.usage.QuoteTokenUsageCostMicroUSD(ctx, prov, mod, u.PromptTokens, u.CompletionTokens, u.CachedTokens)
			}
			if uerr := r.runWriter.UpdateTeamRunStepTokens(ctx, stepID, u.PromptTokens, u.CompletionTokens, cost); uerr != nil {
				r.lg.Warn("回填 team_run_steps token 失败",
					loggateway.StepID("team.step_token_backfill_fail"),
					loggateway.Err(uerr),
					loggateway.Str("step_id", stepID),
					loggateway.Str("agent_key", key),
				)
			}
		}
		sumIn += u.PromptTokens
		sumOut += u.CompletionTokens
		sumCached += u.CachedTokens
		wrote = true
	}
	// Anchor remainder: consumption authored by the team root (non-member
	// author) is inside the run totals but outside every MemberUsage entry.
	// Attribute the remainder to the anchor member so member rows sum to the
	// team_turn row's totals. The remainder inherits the aggregate's
	// usageSource (possibly "estimated").
	remIn, remOut := in.PromptTok-sumIn, in.CompletionTok-sumOut
	remCached := in.Result.CachedTok - sumCached
	if remIn > 0 || remOut > 0 {
		if remCached < 0 {
			remCached = 0
		}
		asst := in.AssistantMsg
		asst.TokenIn, asst.TokenOut = remIn, remOut
		stepID := stepIDByAgentKey[strings.TrimSpace(in.AnchorAg.AgentKey)]
		if stepID == "" {
			stepID = in.Run.ID
		}
		r.recordMemberUsage(ctx, in.Run, in.TeamID, in.AnchorAg, asst,
			strutil.FirstNonEmpty(strings.TrimSpace(in.AnchorAg.Provider), strings.TrimSpace(in.Prov)),
			strutil.FirstNonEmpty(strings.TrimSpace(in.AnchorAg.Model), strings.TrimSpace(in.Mod)),
			in.DialogMode, stepID, remCached, in.UsageSource, biz.UsageAttributionStreamAnchorRemainder)
		wrote = true
	}
	return wrote
}
