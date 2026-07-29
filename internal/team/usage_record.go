package team

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// recordMemberUsage writes model_token_usage_events for a team member step when tokens were consumed.
func (r *Runner) recordMemberUsage(
	ctx context.Context,
	run biz.TeamRunRecord,
	teamID string,
	ag biz.Agent,
	asst biz.ChatMessage,
	prov, mod, dialogMode string,
	stepID string,
	cachedTok int,
) {
	if r == nil || r.usage == nil {
		return
	}
	tin, tout := asst.TokenIn, asst.TokenOut
	if tin <= 0 && tout <= 0 {
		return
	}
	now := time.Now().UTC()
	status := biz.NormalizeTokenUsageStatus(asst.Status)
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
		ID:                uuid.NewString(),
		TeamID:            teamID,
		SessionID:         run.SessionID,
		AgentID:           ag.ID,
		AgentKey:          ag.AgentKey,
		MessageID:         stepID,
		ProviderCode:      prov,
		ModelAPIID:        mod,
		ModelDisplayName:  mod,
		InputTokens:       tin,
		OutputTokens:      tout,
		CachedInputTokens: cachedTok,
		TotalTokens:       tin + tout,
		LatencyMS:         latency,
		TokensPerSecond:   tps,
		Status:            status,
		ErrorMessage:      asst.ErrorMessage,
		UsageKind:         biz.UsageKindTeamMember,
		PromptMode:        dialogMode,
		MetadataJSON:      meta,
		OccurredAt:        now.Format(time.RFC3339),
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	if _, err := r.usage.RecordTokenUsageEvent(recCtx, ev); err != nil {
		r.lg.Warn("团队成员用量落库失败",
			loggateway.StepID("team.usage_record_fail"),
			loggateway.Err(err),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Str("team_id", teamID),
			loggateway.Str("step_id", stepID),
			loggateway.Str("usage_kind", ev.UsageKind),
		)
		return
	}
	if r.td.Sessions != nil && strings.TrimSpace(run.SessionID) != "" {
		r.td.Sessions.AccumulateMetricsDelta(session.SessionMetricsDelta{
			SessionID:         run.SessionID,
			ModelCallCount:    ev.CallCount,
			InputTokens:       int64(ev.InputTokens),
			OutputTokens:      int64(ev.OutputTokens),
			TotalTokens:       int64(ev.TotalTokens),
			TotalCostMicroUsd: ev.TotalCostMicroUSD,
		})
	}
	biz.PublishTokenUsageEnvelope(ctx, r.td.Pipeline.EventBus, ev)
}

// recordTeamRunUsage writes one aggregated team turn row (workflow-level tokens).
func (r *Runner) recordTeamRunUsage(
	ctx context.Context,
	run biz.TeamRunRecord,
	teamID string,
	anchor biz.Agent,
	promptTok, completionTok, cachedTok int,
	prov, mod, dialogMode string,
) {
	if (r == nil || r.usage == nil) || (promptTok <= 0 && completionTok <= 0) {
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
		ID:                uuid.NewString(),
		TeamID:            teamID,
		SessionID:         run.SessionID,
		AgentID:           anchor.ID,
		AgentKey:          anchor.AgentKey,
		MessageID:         run.ID,
		ProviderCode:      prov,
		ModelAPIID:        mod,
		ModelDisplayName:  mod,
		InputTokens:       promptTok,
		OutputTokens:      completionTok,
		CachedInputTokens: cachedTok,
		TotalTokens:       promptTok + completionTok,
		UsageKind:         biz.UsageKindTeamTurn,
		PromptMode:        dialogMode,
		Status:            biz.TokenUsageStatusSuccess,
		MetadataJSON:      meta,
		OccurredAt:        now.Format(time.RFC3339),
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	if _, err := r.usage.RecordTokenUsageEvent(recCtx, ev); err != nil {
		r.lg.Warn("团队轮次用量落库失败",
			loggateway.StepID("team.usage_record_fail"),
			loggateway.Err(err),
			loggateway.Str("team_id", teamID),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("usage_kind", biz.UsageKindTeamTurn),
		)
		return
	}
	if r.td.Sessions != nil && strings.TrimSpace(run.SessionID) != "" {
		r.td.Sessions.AccumulateMetricsDelta(session.SessionMetricsDelta{
			SessionID:         run.SessionID,
			ModelCallCount:    ev.CallCount,
			InputTokens:       int64(ev.InputTokens),
			OutputTokens:      int64(ev.OutputTokens),
			TotalTokens:       int64(ev.TotalTokens),
			TotalCostMicroUsd: ev.TotalCostMicroUSD,
		})
	}
	biz.PublishTokenUsageEnvelope(ctx, r.td.Pipeline.EventBus, ev)
}
