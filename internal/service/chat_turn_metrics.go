package service

import (
	"context"
	"time"

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
	lg       loggateway.Logger
}

func newChatTurnMetrics(sessions biz.SessionTurnManager, usage *biz.UsageUsecase, lg loggateway.Logger) *chatTurnMetrics {
	return &chatTurnMetrics{sessions: sessions, usage: usage, lg: lg}
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
	Latency       time.Duration
	ErrMsg        string
}

// RecordTurnUsage records token usage for a turn.
func (m *chatTurnMetrics) RecordTurnUsage(ctx context.Context, p TurnUsageParams) {
	if m == nil || m.usage == nil {
		return
	}
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
