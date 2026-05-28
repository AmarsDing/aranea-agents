package service

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
)

type admittedTurnContextKey struct{}

var errChatOrchestratorNil = errors.New("chat turn executor: orchestrator is nil")

func contextWithAdmittedTurnID(ctx context.Context, turnID string) context.Context {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return ctx
	}
	return context.WithValue(ctx, admittedTurnContextKey{}, turnID)
}

func admittedTurnIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(admittedTurnContextKey{}).(string)
	return strings.TrimSpace(v)
}

type chatTurnExecutor struct {
	orch *ChatOrchestrator
}

func (e chatTurnExecutor) ExecuteTurn(ctx context.Context, turn biz.Turn, input biz.TurnInput) (biz.NativeTurnResult, error) {
	if e.orch == nil {
		return biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed}, errChatOrchestratorNil
	}
	result, err := e.orch.RunNativeAgentTurnWithOutcome(contextWithAdmittedTurnID(ctx, turn.ID), input)
	if IsTurnMessageQueued(err) {
		return result, nil
	}
	return result, err
}

func turnResultFromNative(result biz.NativeTurnResult) biz.TurnResult {
	switch result.Outcome {
	case biz.NativeTurnOutcomeCompleted:
		return biz.TurnResult{
			Outcome:      biz.TurnOutcomeCompleted,
			UserMsg:      result.UserMsg,
			AssistantMsg: result.AssistantMsg,
			Reply:        result.AssistantMsg.ContentMarkdown,
		}
	case biz.NativeTurnOutcomeQueued:
		return biz.TurnResult{
			Outcome:   biz.TurnOutcomeQueued,
			PendingID: result.PendingID,
		}
	case biz.NativeTurnOutcomeFailed:
		return biz.TurnResult{
			Outcome: biz.TurnOutcomeFailed,
			UserMsg: result.UserMsg,
		}
	default:
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}
	}
}
