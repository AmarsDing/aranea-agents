package session

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (uc *SessionUsecase) CreateTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error) {
	if strings.TrimSpace(turn.SessionID) == "" {
		return SessionTurn{}, validationErr("session_id is required")
	}
	if strings.TrimSpace(turn.ID) == "" {
		turn.ID = uuid.NewString()
	}
	if turn.CreatedAt == "" {
		turn.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if turn.UpdatedAt == "" {
		turn.UpdatedAt = turn.CreatedAt
	}
	return uc.turnRepo.CreateSessionTurn(ctx, turn)
}

func (uc *SessionUsecase) UpdateTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error) {
	if strings.TrimSpace(id) == "" {
		return SessionTurn{}, validationErr("turn id is required")
	}
	return uc.turnRepo.UpdateSessionTurn(ctx, id, fields)
}

// IncrementInvocationCounts bumps session.tool_call_count / mcp_call_count / skill_call_count.
func (uc *SessionUsecase) IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error {
	if toolDelta == 0 && mcpDelta == 0 && skillDelta == 0 {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return validationErr("session id is required")
	}
	return uc.contextUpdater.IncrementInvocationCounts(ctx, sessionID, toolDelta, mcpDelta, skillDelta)
}

func (uc *SessionUsecase) ListTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionTurnListResult{}, validationErr("session id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return uc.turnRepo.ListSessionTurns(ctx, sessionID, limit, offset)
}
