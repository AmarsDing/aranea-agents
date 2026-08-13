package service

import (
	"context"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
)

// turnRecorder is the interface for recording turn usage and session turn data.
type turnRecorder interface {
	RecordTurnUsage(ctx context.Context, p TurnUsageParams)
	RecordSessionTurn(ctx context.Context, p SessionTurnRecordParams)
}

// chatTurnMetrics implements turnRecorder.
//
// Part of the TECH-DEBT(BL8) resolution: separating metrics recording from
// the orchestrator's core turn logic.
type chatTurnMetrics struct {
	sessions biz.SessionTurnManager
	usage    *biz.UsageUsecase
	monitor  *biz.MonitorUsecase
	lg       loggateway.Logger
}

func newChatTurnMetrics(sessions biz.SessionTurnManager, usage *biz.UsageUsecase, monitor *biz.MonitorUsecase, lg loggateway.Logger) *chatTurnMetrics {
	return &chatTurnMetrics{sessions: sessions, usage: usage, monitor: monitor, lg: lg}
}

// Compile-time interface check.
var _ turnRecorder = (*chatTurnMetrics)(nil)

// TurnUsageParams groups parameters for recording turn token usage.
type TurnUsageParams struct {
	Emitter       *event.TraceEmitter
	SessionID     string
	RunID         string
	AgentKey      string
	AgentID       string
	Provider      string
	Model         string
	Status        string
	PromptTok     int
	CompletionTok int
	// CachedTok is the cache-hit portion of PromptTok (billed at cache-read price).
	CachedTok int
	Latency   time.Duration
	ErrMsg    string
}

// RecordTurnUsage records token usage for a turn.
func (m *chatTurnMetrics) RecordTurnUsage(ctx context.Context, p TurnUsageParams) {
	if m == nil || m.usage == nil {
		return
	}
	// E 预算表分解（P0-A，2026-08-11）：per-turn token/缓存命中入进程日志，
	// 前缀稳定化效果直接由 cache_hit_ratio 验证（此前只落库，排查需查表）。
	if p.PromptTok > 0 {
		m.lg.Info("turn token usage",
			loggateway.StepID("chat.turn_usage"),
			loggateway.SessionID(p.SessionID),
			loggateway.RunID(p.RunID),
			loggateway.AgentKey(p.AgentKey),
			loggateway.Str("model", p.Model),
			loggateway.Int("prompt_tokens", p.PromptTok),
			loggateway.Int("completion_tokens", p.CompletionTok),
			loggateway.Int("cached_tokens", p.CachedTok),
			loggateway.Float64("cache_hit_ratio", float64(p.CachedTok)/float64(p.PromptTok)),
			loggateway.Duration(p.Latency.Milliseconds()))
	}
	m.recordContextBudgetLog(ctx, p)
	meta := "{}"
	if p.Emitter != nil {
		meta = p.Emitter.MetadataJSON()
	}
	traceID := ""
	if p.Emitter != nil {
		traceID = p.Emitter.TraceID()
	}
	if err := m.usage.RecordTurnUsage(ctx, biz.TurnUsageInput{
		SessionID:     p.SessionID,
		RunID:         p.RunID,
		AgentKey:      p.AgentKey,
		AgentID:       p.AgentID,
		Provider:      p.Provider,
		Model:         p.Model,
		Status:        p.Status,
		PromptTok:     p.PromptTok,
		CompletionTok: p.CompletionTok,
		CachedTok:     p.CachedTok,
		Latency:       p.Latency,
		ErrMsg:        p.ErrMsg,
		MetadataJSON:  meta,
		TraceID:       traceID,
	}); err != nil && p.Emitter != nil {
		p.Emitter.LogError("chat.usage_record", "turn usage record failed",
			event.P("error", err.Error()),
			event.P("run_id", p.RunID),
			event.P("usage_kind", biz.UsageKindChatTurn),
			event.P("status", p.Status),
		)
	}
	m.recordRunnerCompletion(ctx, p, traceID)
}

// recordContextBudgetLog emits the per-turn context budget ledger
// (29-token.design.md §9.6, 任务 0.1). Process log only — no flow log, no
// stepTitleRegistry entry. No-op when the turn ctx carries no ContextBudget
// (non-chat paths) or nothing was recorded (LLM never reached, e.g. early
// admission failure).
func (m *chatTurnMetrics) recordContextBudgetLog(ctx context.Context, p TurnUsageParams) {
	if m == nil {
		return
	}
	budget := chatagent.ContextBudgetFromContext(ctx)
	if budget == nil {
		return
	}
	snap := budget.Snapshot()
	if snap.EstTotalInput == 0 && snap.ToolsCount == 0 {
		return
	}
	staticRatio := 0.0
	if snap.EstTotalInput > 0 {
		staticRatio = float64(snap.EstTokens[chatagent.ContextBudgetCategoryStaticPrefix]) / float64(snap.EstTotalInput)
	}
	fields := []loggateway.Field{
		loggateway.StepID("chat.context_budget"),
		loggateway.SessionID(p.SessionID),
		loggateway.RunID(p.RunID),
		loggateway.AgentKey(p.AgentKey),
		loggateway.Int("static_prefix_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryStaticPrefix]),
		loggateway.Int("tools_schema_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryToolsSchema]),
		loggateway.Int("tools_count", snap.ToolsCount),
		loggateway.Int("history_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryHistory]),
		loggateway.Int("memory_l1_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryMemoryL1]),
		loggateway.Int("memory_l4_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryMemoryL4]),
		loggateway.Int("memory_composite_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryMemoryComposite]),
		loggateway.Int("knowledge_cue_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryKnowledgeCue]),
		loggateway.Int("skill_guidance_tokens", snap.EstTokens[chatagent.ContextBudgetCategorySkillGuidance]),
		loggateway.Int("skill_overview_tokens", snap.EstTokens[chatagent.ContextBudgetCategorySkillOverview]),
		loggateway.Int("other_dynamic_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryOtherDynamic]),
		loggateway.Int("est_total_input", snap.EstTotalInput),
		loggateway.Float64("static_ratio", staticRatio),
	}
	if p.PromptTok > 0 {
		fields = append(fields, loggateway.Float64("cache_hit_ratio", float64(p.CachedTok)/float64(p.PromptTok)))
	}
	m.lg.Info("context budget ledger", fields...)
}

// recordRunnerCompletion writes the runner.completion monitor event for a
// terminal chat turn. This restores the data stream behind the Runner
// metrics panel and the runner.error_rate alert rule — the legacy writer
// (EventBus runner-completion handler) was removed in the Activity-First
// migration (ab9ee9e07) without a replacement, leaving the stream dark
// since 2026-06-13.
func (m *chatTurnMetrics) recordRunnerCompletion(ctx context.Context, p TurnUsageParams, traceID string) {
	if m == nil || m.monitor == nil {
		return
	}
	de := biz.DomainEvent{
		Type:       biz.DomainEventRunnerCompletion,
		SessionID:  p.SessionID,
		RunID:      p.RunID,
		Author:     p.AgentKey,
		AgentID:    p.AgentID,
		TraceID:    traceID,
		DurationMS: p.Latency.Milliseconds(),
		Timestamp:  time.Now().UTC(),
		RunKind:    "chat",
	}
	if p.Status == "error" {
		de.Error = &biz.DomainError{Message: p.ErrMsg}
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := biz.RecordRunnerCompletion(recCtx, m.monitor, de); err != nil {
		m.lg.Warn("runner.completion 监控事件落库失败",
			loggateway.StepID("chat.runner_completion_fail"),
			loggateway.Str("session_id", p.SessionID),
			loggateway.Str("run_id", p.RunID),
			loggateway.Err(err))
	}
}

// SessionTurnRecordParams groups parameters for recording a completed session turn.
type SessionTurnRecordParams struct {
	SessionID      string
	OwnerType      string // "agent" or "team"
	OwnerID        string // AgentID when OwnerType="agent", TeamID when OwnerType="team"
	UserMsgID      string
	AssistantMsgID string
	Provider       string
	Model          string
	PromptTok      int
	CompletionTok  int
	ContentPreview string
}

// RecordSessionTurn records a completed agent or team turn.
func (m *chatTurnMetrics) RecordSessionTurn(ctx context.Context, p SessionTurnRecordParams) {
	if m == nil || m.sessions == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	preview := strutil.ProtoPreview(p.ContentPreview, 200)
	if turnID := admittedTurnIDFromContext(ctx); turnID != "" {
		updates := biz.SessionTurnUpdateFields{
			Status:              ptrString("completed"),
			EndedAt:             ptrString(now),
			UserMessageID:       ptrString(p.UserMsgID),
			AssistantMessageID:  ptrString(p.AssistantMsgID),
			OwnerType:           ptrString(p.OwnerType),
			InputTokens:         ptrInt(p.PromptTok),
			OutputTokens:        ptrInt(p.CompletionTok),
			TotalTokens:         ptrInt(p.PromptTok + p.CompletionTok),
			ModelCallCount:      ptrInt(1),
			FinalProvider:       ptrString(p.Provider),
			FinalModel:          ptrString(p.Model),
			FinalContentPreview: ptrString(preview),
		}
		switch p.OwnerType {
		case "agent":
			updates.AgentID = ptrString(p.OwnerID)
		case "team":
			updates.TeamID = ptrString(p.OwnerID)
		}
		if _, err := m.sessions.UpdateTurn(ctx, turnID, updates); err != nil {
			m.lg.Warn("session turn update failed",
				loggateway.StepID("chat.usage_record_fail"),
				loggateway.Str("session_id", p.SessionID),
				loggateway.Str("turn_id", turnID),
				loggateway.Err(err))
		}
		return
	}
	turn := biz.SessionTurn{
		SessionID:           p.SessionID,
		UserMessageID:       p.UserMsgID,
		AssistantMessageID:  p.AssistantMsgID,
		OwnerType:           p.OwnerType,
		Status:              "completed",
		StartedAt:           now,
		EndedAt:             now,
		InputTokens:         p.PromptTok,
		OutputTokens:        p.CompletionTok,
		TotalTokens:         p.PromptTok + p.CompletionTok,
		ModelCallCount:      1,
		FinalProvider:       p.Provider,
		FinalModel:          p.Model,
		FinalContentPreview: preview,
	}
	switch p.OwnerType {
	case "agent":
		turn.AgentID = p.OwnerID
	case "team":
		turn.TeamID = p.OwnerID
	}
	if _, err := m.sessions.CreateTurn(ctx, turn); err != nil {
		m.lg.Warn("session turn record failed",
			loggateway.StepID("chat.usage_record_fail"),
			loggateway.Str("session_id", p.SessionID),
			loggateway.Err(err))
	}
}
