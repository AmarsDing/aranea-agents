package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// PersistentTurnService stores the canonical Turn lifecycle in session_turns.
type PersistentTurnService struct {
	Sessions persistentTurnStore
	Now      func() time.Time
}

type persistentTurnStore interface {
	CreateTurn(context.Context, biz.SessionTurn) (biz.SessionTurn, error)
	UpdateTurn(context.Context, string, biz.SessionTurnUpdateFields) (biz.SessionTurn, error)
}

func NewPersistentTurnService(sessions biz.SessionTurnWriterPort) *PersistentTurnService {
	return &PersistentTurnService{Sessions: sessions}
}

func (s *PersistentTurnService) AdmitTurn(ctx context.Context, intent biz.TurnIntent) (biz.Turn, error) {
	if s == nil || s.Sessions == nil {
		return biz.Turn{}, errors.New("turn service: sessions is nil")
	}
	intent = intent.Canonicalize()
	now := s.now()
	turn := biz.Turn{
		ID:              uuid.NewString(),
		SessionID:       intent.SessionID,
		Source:          intent.Source,
		TargetType:      intent.TargetType,
		AgentID:         intent.AgentID,
		TeamID:          intent.TeamID,
		Status:          biz.TurnStatusQueued,
		DeliveryTargets: intent.DeliveryTargets,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	meta, _ := json.Marshal(map[string]any{
		"source":           turn.Source,
		"target_type":      turn.TargetType,
		"idempotency_key":  intent.IdempotencyKey,
		"delivery_targets": intent.DeliveryTargets,
	})
	row, err := s.Sessions.CreateTurn(ctx, biz.SessionTurn{
		ID:           turn.ID,
		SessionID:    turn.SessionID,
		OwnerType:    string(turn.TargetType),
		AgentID:      turn.AgentID,
		TeamID:       turn.TeamID,
		Status:       string(turn.Status),
		StartedAt:    now.Format(time.RFC3339),
		MetadataJSON: string(meta),
		CreatedAt:    now.Format(time.RFC3339),
		UpdatedAt:    now.Format(time.RFC3339),
	})
	if err != nil {
		return biz.Turn{}, err
	}
	return turnFromSessionTurn(row, turn), nil
}

func (s *PersistentTurnService) CompleteTurn(ctx context.Context, turn biz.Turn, result biz.NativeTurnResult) (biz.Turn, error) {
	if s == nil || s.Sessions == nil {
		return biz.Turn{}, errors.New("turn service: sessions is nil")
	}
	status := biz.TurnStatusFromNativeOutcome(result.Outcome)
	if status == "" {
		status = biz.TurnStatusCompleted
	}
	now := s.now().Format(time.RFC3339)
	row, err := s.Sessions.UpdateTurn(ctx, turn.ID, biz.SessionTurnUpdateFields{
		Status:  ptrString(string(status)),
		EndedAt: ptrString(now),
	})
	if err != nil {
		return turn, err
	}
	turn.Status = status
	turn.UpdatedAt = s.now()
	return turnFromSessionTurn(row, turn), nil
}

func (s *PersistentTurnService) FailTurn(ctx context.Context, turn biz.Turn, err error) (biz.Turn, error) {
	if s == nil || s.Sessions == nil {
		return biz.Turn{}, errors.New("turn service: sessions is nil")
	}
	now := s.now().Format(time.RFC3339)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	row, updateErr := s.Sessions.UpdateTurn(ctx, turn.ID, biz.SessionTurnUpdateFields{
		Status:       ptrString(string(biz.TurnStatusFailed)),
		EndedAt:      ptrString(now),
		ErrorMessage: ptrString(errMsg),
	})
	if updateErr != nil {
		return turn, updateErr
	}
	turn.Status = biz.TurnStatusFailed
	turn.UpdatedAt = s.now()
	return turnFromSessionTurn(row, turn), nil
}

func (s *PersistentTurnService) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func turnFromSessionTurn(row biz.SessionTurn, base biz.Turn) biz.Turn {
	base.ID = row.ID
	base.SessionID = row.SessionID
	base.RunID = row.RunID
	base.AgentID = row.AgentID
	base.TeamID = row.TeamID
	if row.Status != "" {
		base.Status = biz.TurnStatus(row.Status)
	}
	return base
}

func ptrString(v string) *string {
	return &v
}

func ptrInt(v int) *int {
	return &v
}
