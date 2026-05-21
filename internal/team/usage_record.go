package team

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	"github.com/google/uuid"
)

// recordMemberUsage writes model_token_usage_events for a team member step when tokens were consumed.
func (r *Runner) recordMemberUsage(
	ctx context.Context,
	run biz.TeamRun,
	teamID string,
	ag biz.Agent,
	asst biz.ChatMessage,
	prov, mod, dialogMode string,
	stepID string,
) {
	if r == nil || r.usage == nil {
		return
	}
	tin, tout := asst.TokenIn, asst.TokenOut
	if tin <= 0 && tout <= 0 {
		return
	}
	now := time.Now().UTC()
	status := asst.Status
	if status == "" || status == "ok" {
		status = "success"
	}
	latency := asst.LatencyMS
	tps := 0.0
	if latency > 0 && tout > 0 {
		tps = float64(tout) / (float64(latency) / 1000.0)
	}
	meta := "{}"
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		meta = em.MetadataJSON()
	} else {
		b, _ := json.Marshal(map[string]any{
			"source":  "team_member_step",
			"run_id":  run.ID,
			"step_id": stepID,
		})
		meta = string(b)
	}
	ev := biz.TokenUsageEvent{
		ID:               uuid.NewString(),
		TeamID:           teamID,
		SessionID:        run.SessionID,
		AgentID:          ag.ID,
		AgentKey:         ag.AgentKey,
		MessageID:        stepID,
		ProviderCode:     prov,
		ModelAPIID:       mod,
		ModelDisplayName: mod,
		InputTokens:      tin,
		OutputTokens:     tout,
		TotalTokens:      tin + tout,
		LatencyMS:        latency,
		TokensPerSecond:  tps,
		Status:           status,
		ErrorMessage:     asst.ErrorMessage,
		UsageKind:        biz.UsageKindTeamMember,
		PromptMode:       dialogMode,
		MetadataJSON:     meta,
		OccurredAt: now.Format(time.RFC3339),
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	if _, err := r.usage.RecordTokenUsageEvent(recCtx, ev); err != nil {
		event.CtxFlowLogWarn(ctx, "team.usage_record_fail", "团队成员用量落库失败",
			event.P("error", err.Error()),
			event.P("agent_id", ag.ID),
			event.P("team_id", teamID),
			event.P("step_id", stepID),
			event.P("usage_kind", ev.UsageKind),
		)
	}
}

// recordTeamRunUsage writes one aggregated team turn row (workflow-level tokens).
func (r *Runner) recordTeamRunUsage(
	ctx context.Context,
	run biz.TeamRun,
	teamID string,
	anchor biz.Agent,
	promptTok, completionTok int,
	prov, mod, dialogMode string,
) {
	if r == nil || r.usage == nil || promptTok <= 0 && completionTok <= 0 {
		return
	}
	now := time.Now().UTC()
	meta := "{}"
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		meta = em.MetadataJSON()
	} else {
		b, _ := json.Marshal(map[string]any{
			"source": "team_run",
			"run_id": run.ID,
		})
		meta = string(b)
	}
	ev := biz.TokenUsageEvent{
		ID:               uuid.NewString(),
		TeamID:           teamID,
		SessionID:        run.SessionID,
		AgentID:          anchor.ID,
		AgentKey:         anchor.AgentKey,
		MessageID:        run.ID,
		ProviderCode:     prov,
		ModelAPIID:       mod,
		ModelDisplayName: mod,
		InputTokens:      promptTok,
		OutputTokens:     completionTok,
		TotalTokens:      promptTok + completionTok,
		UsageKind:        biz.UsageKindTeamTurn,
		PromptMode:       dialogMode,
		Status:           "success",
		MetadataJSON:     meta,
		OccurredAt:       now.Format(time.RFC3339),
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	if _, err := r.usage.RecordTokenUsageEvent(recCtx, ev); err != nil {
		event.CtxFlowLogWarn(ctx, "team.usage_record_fail", "团队轮次用量落库失败",
			event.P("error", err.Error()),
			event.P("team_id", teamID),
			event.P("run_id", run.ID),
			event.P("usage_kind", biz.UsageKindTeamTurn),
		)
	}
}
