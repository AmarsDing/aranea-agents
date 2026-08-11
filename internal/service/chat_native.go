package service

import (
	"context"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// nativeDialogModeChatOptions returns the dialog mode options for native chat.
func nativeDialogModeChatOptions() []*chatv1.ChatOption {
	return []*chatv1.ChatOption{
		{Type: "dialog_mode", Key: "default", Label: "标准对话", Enabled: true, SortOrder: 1},
		{Type: "dialog_mode", Key: "plan", Label: "深度思考", Enabled: true, SortOrder: 2},
		{Type: "dialog_mode", Key: "code", Label: "仅代码", Enabled: true, SortOrder: 3},
	}
}

// RunNativeTurn implements biz.ChannelTurnGateway — the biz-level turn entry point
// that avoids proto dependency. All internal callers (Channel, Cron, A2A) should
// use this instead of proto-based methods.
func (s *ChatService) RunNativeTurn(ctx context.Context, input biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	result, err := s.ExecuteTurn(ctx, input)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	return result.UserMsg, result.AssistantMsg, nil
}

// RunNativeTurnWithOutcome implements biz.ChannelTurnGateway with explicit turn classification.
func (s *ChatService) RunNativeTurnWithOutcome(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error) {
	return s.ExecuteTurn(ctx, input)
}

// RunAgentTurn implements a2a.AgentTurnRunner for call_agent and HTTP Invoke dispatch (EP-A2A-01).
func (s *ChatService) RunAgentTurn(ctx context.Context, agentID, input string, timeoutSec int) (string, error) {
	return s.orch.RunAgentTurn(ctx, agentID, input, timeoutSec)
}

// RunCronTurn dispatches a cron-triggered turn through the in-process agent runner.
func (s *ChatService) RunCronTurn(ctx context.Context, sessionID, content, teamID string) (userMsgID, agentMsgID string, err error) {
	return s.orch.RunCronTurn(ctx, sessionID, content, teamID)
}

// hydratedAgent loads and returns an Agent by ID.
func (s *ChatService) hydratedAgent(ctx context.Context, agentID string) (biz.Agent, error) {
	return s.orch.hydratedAgent(ctx, agentID)
}

// notifyNativeTurnHooks runs post-turn side effects.
func notifyNativeTurnHooks(ctx context.Context, o *ChatOrchestrator, sessionID string, ag biz.Agent, userInput, assistantOutput string) {
	o.notifyNativeTurnHooks(ctx, sessionID, ag, userInput, assistantOutput)
}

// chatMessageToMap converts a ChatMessage to a map for proto serialization.
func chatMessageToMap(m biz.ChatMessage) map[string]any {
	return map[string]any{
		"id":                m.ID,
		"session_id":        m.SessionID,
		"parent_message_id": m.ParentMessageID,
		"turn_id":           m.TurnID,
		"turn_number":       m.TurnNumber,
		"seq_in_turn":       m.SeqInTurn,
		"role":              m.Role,
		"content_markdown":  m.ContentMarkdown,
		"model_name":        m.ModelName,
		"token_in":          m.TokenIn,
		"token_out":         m.TokenOut,
		"latency_ms":        m.LatencyMS,
		"status":            m.Status,
		"attachments_count": m.AttachmentsCount,
		"options_json":      m.OptionsJSON,
		"error_message":     m.ErrorMessage,
		"created_at":        m.CreatedAt,
	}
}

func chatNowRFC3339() string {
	return strings.TrimSpace(chatagent.RFC3339Now())
}

// ExecuteTurn implements biz.TurnGateway — delegates to ChatOrchestrator.Execute.
func (s *ChatService) ExecuteTurn(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error) {
	s.lg.With(loggateway.SessionID(input.SessionID)).Info("ChatService.ExecuteTurn: 入口",
		loggateway.StepID("chat.execute_turn_start"),
		loggateway.Any("agent_key", input.AgentKey),
		loggateway.Any("has_pipeline", s.turnPipeline != nil))
	start := time.Now()

	if s != nil && s.turnPipeline != nil {
		if result, err, handled := s.tryAdmissionBeforePersistence(ctx, input); handled {
			s.lg.With(loggateway.SessionID(input.SessionID)).Info("ChatService.ExecuteTurn: admission handled",
				loggateway.StepID("chat.execute_turn_admission"),
				loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()),
				loggateway.Any("handled", true))
			if isTurnMessageQueued(err) {
				return result, ErrTurnMessageQueued
			}
			return result, err
		}
		_, result, err := s.turnPipeline.Run(ctx, turnIntentFromInput(input))
		s.lg.With(loggateway.SessionID(input.SessionID)).Info("ChatService.ExecuteTurn: pipeline 完成",
			loggateway.StepID("chat.execute_turn_pipeline_done"),
			loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()),
			loggateway.Any("has_error", err != nil),
			loggateway.Any("outcome", string(result.Outcome)))
		if err != nil {
			return result, err
		}
		if result.Outcome == biz.TurnOutcomeQueued {
			return result, ErrTurnMessageQueued
		}
		return result, nil
	}
	result, err := s.orch.Execute(ctx, input)
	s.lg.With(loggateway.SessionID(input.SessionID)).Info("ChatService.ExecuteTurn: orch.Execute 完成",
		loggateway.StepID("chat.execute_turn_orch_done"),
		loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()),
		loggateway.Any("has_error", err != nil))
	return result, err
}

func (s *ChatService) tryAdmissionBeforePersistence(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error, bool) {
	if s == nil || s.orch == nil {
		return biz.TurnResult{}, nil, false
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return biz.TurnResult{}, nil, false
	}
	hasActive := s.orch.runs.HasActive(sessionID)
	if !hasActive {
		return biz.TurnResult{}, nil, false
	}
	contextPressure := s.orch.sessionContextPressure(ctx, input)
	verdict, handled := s.orch.checkTurnAdmission(input, hasActive, contextPressure)
	if !handled {
		return biz.TurnResult{}, nil, false
	}
	result, err := turnResultFromAdmissionVerdict(verdict)
	return result, err, true
}

func turnIntentFromInput(input biz.TurnInput) biz.TurnIntent {
	return biz.TurnIntent{
		SessionID:     input.SessionID,
		AgentKey:      input.AgentKey,
		TeamID:        input.TeamID,
		Content:       input.Content,
		AttachmentIDs: input.Options.AttachmentIDs,
		Options:       input.Options,
		Timeouts:      input.Timeouts,
		EntryConfig:   input.EntryConfig,
		ParentTaskID:  input.ParentTaskID,
		Synthesis:     input.Synthesis,
		// Voice 必须透传：pipeline 出口 TurnIntent.TurnInput() 重建后，
		// orchestrator 依此打 voice fast-path（关思考）与跳过主动召回。
		Voice: input.Voice,
	}
}

// chatServiceGatewayAdapter adapts ChatService to implement biz-level narrow
// interfaces without method name collisions with proto-based methods.
type chatServiceGatewayAdapter struct {
	svc *ChatService
}

// EnqueueUserMessage implements biz.PendingMessageGateway.
func (a chatServiceGatewayAdapter) EnqueueUserMessage(ctx context.Context, sessionID, content string) (accepted bool, pendingID string, rejectReason string, err error) {
	acc, _, pid, reason, err := a.svc.orch.EnqueueUserMessage(sessionID, content)
	return acc, pid, reason, err
}

// CancelPendingMessage implements biz.PendingMessageGateway.
func (a chatServiceGatewayAdapter) CancelPendingMessage(ctx context.Context, sessionID, pendingID string) error {
	if a.svc.orch.CancelPendingMessage(sessionID, pendingID) {
		return nil
	}
	return apierror.NotFound(apierror.DomainChat, "pending message not found")
}

// UpdatePendingMessage implements biz.PendingMessageGateway.
func (a chatServiceGatewayAdapter) UpdatePendingMessage(ctx context.Context, sessionID, pendingID, content string) error {
	if a.svc.orch.UpdatePendingMessage(sessionID, pendingID, content) {
		return nil
	}
	return apierror.NotFound(apierror.DomainChat, "pending message not found")
}

// GetPendingMessages implements biz.PendingMessageGateway.
func (a chatServiceGatewayAdapter) GetPendingMessages(ctx context.Context, sessionID string) ([]biz.PendingMessage, error) {
	entries := a.svc.orch.GetPendingMessages(sessionID)
	result := make([]biz.PendingMessage, len(entries))
	for i, e := range entries {
		result[i] = biz.PendingMessage{
			ID:        e.ID,
			SessionID: sessionID,
			Content:   e.Content,
			CreatedAt: e.CreatedAt,
		}
	}
	return result, nil
}

// NewPendingMessageGatewayAdapter creates a biz.PendingMessageGateway from ChatService.
func NewPendingMessageGatewayAdapter(svc *ChatService) biz.PendingMessageGateway {
	return chatServiceGatewayAdapter{svc: svc}
}
