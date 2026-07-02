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

func (e chatTurnExecutor) ExecuteTurn(ctx context.Context, turn biz.CanonicalTurn, input biz.TurnInput) (biz.TurnResult, error) {
	if e.orch == nil {
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, errChatOrchestratorNil
	}
	result, err := e.orch.RunNativeAgentTurnWithOutcome(contextWithAdmittedTurnID(ctx, turn.ID), input)
	if isTurnMessageQueued(err) {
		return result, nil
	}
	return result, err
}
