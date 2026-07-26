package service

import (
	"context"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	sessctx "aranea-agents/internal/session"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// recordSessionTurn records a completed agent turn.
func (o *ChatOrchestrator) recordSessionTurn(ctx context.Context, sessionID string, ag biz.Agent, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	o.turnMetrics().RecordSessionTurn(ctx, SessionTurnRecordParams{
		SessionID:      sessionID,
		OwnerType:      "agent",
		OwnerID:        ag.ID,
		UserMsgID:      userMsgID,
		AssistantMsgID: assistantMsgID,
		Provider:       prov,
		Model:          mod,
		PromptTok:      promptTok,
		CompletionTok:  completionTok,
		ContentPreview: contentPreview,
	})
}

// recordTeamSessionTurn records a completed team turn.
func (o *ChatOrchestrator) recordTeamSessionTurn(ctx context.Context, sessionID, teamID, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	o.turnMetrics().RecordSessionTurn(ctx, SessionTurnRecordParams{
		SessionID:      sessionID,
		OwnerType:      "team",
		OwnerID:        teamID,
		UserMsgID:      userMsgID,
		AssistantMsgID: assistantMsgID,
		Provider:       prov,
		Model:          mod,
		PromptTok:      promptTok,
		CompletionTok:  completionTok,
		ContentPreview: contentPreview,
	})
}

// recordTurnUsage records token usage for a turn.
func (o *ChatOrchestrator) recordTurnUsage(
	ctx context.Context,
	emitter *event.TraceEmitter,
	sessionID, runID, agentKey, agentID, prov, mod, status string,
	promptTok, completionTok, cachedTok int,
	latency time.Duration,
	errMsg string,
) {
	o.turnMetrics().RecordTurnUsage(ctx, TurnUsageParams{
		Emitter:       emitter,
		SessionID:     sessionID,
		RunID:         runID,
		AgentKey:      agentKey,
		AgentID:       agentID,
		Provider:      prov,
		Model:         mod,
		Status:        status,
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		CachedTok:     cachedTok,
		Latency:       latency,
		ErrMsg:        errMsg,
	})
}

// patchSessionContextUsage updates session context usage after a turn.
func (o *ChatOrchestrator) patchSessionContextUsage(ctx context.Context, sessionID string, sess biz.Session, ag biz.Agent, prov, mod string, promptTok, completionTok int) {
	sessctx.PatchContextFromLLMUsage(ctx, o.td().Sessions, o.td().Compress, o.llmContextCatalog(), sessionID, sess, ag, prov, mod, promptTok, completionTok, o.lg())
}

// notifyNativeTurnHooks runs post-turn side effects.
func (o *ChatOrchestrator) notifyNativeTurnHooks(ctx context.Context, sessionID string, ag biz.Agent, userInput, assistantOutput string) {
	if o == nil || o.td().AfterTurn == nil {
		return
	}
	o.td().AfterTurn.AfterNativeTurn(ctx, biz.NativeTurnEvent{
		AgentID:         ag.ID,
		AgentConfigJSON: ag.ConfigJSON,
		AgentSettings:   ag.Settings,
		SessionID:       sessionID,
		UserInput:       userInput,
		AssistantOutput: assistantOutput,
	})
}

// checkTeamMemberQuotas rejects the turn when any enabled team member exceeds agent scope quota.
func (o *ChatOrchestrator) checkTeamMemberQuotas(ctx context.Context, teamID string) error {
	return o.admission().EnforceTeamMemberQuotas(ctx, teamID)
}

// bumpSessionRevision bumps the session revision counter.
func (o *ChatOrchestrator) bumpSessionRevision(ctx context.Context, sessionID string) {
	o.eventPublisher().BumpSessionRevision(ctx, sessionID)
}

// buildUserMessage constructs a trpcmodel.Message from content and attachment IDs.
func (o *ChatOrchestrator) buildUserMessage(ctx context.Context, sessionID, content string, attachmentIDs []string) (trpcmodel.Message, error) {
	return chatagent.BuildUserMessageFromArtifacts(ctx, o.artifacts(), o.rt().ToolResultGate, sessionID, content, attachmentIDs)
}
