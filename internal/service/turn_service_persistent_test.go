package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

type testPersistentTurnStore struct {
	created biz.SessionTurn
	updated biz.SessionTurnUpdateFields
}

func (s *testPersistentTurnStore) CreateTurn(_ context.Context, turn biz.SessionTurn) (biz.SessionTurn, error) {
	s.created = turn
	return turn, nil
}

func (s *testPersistentTurnStore) UpdateTurn(_ context.Context, id string, fields biz.SessionTurnUpdateFields) (biz.SessionTurn, error) {
	s.updated = fields
	row := s.created
	row.ID = id
	if fields.Status != nil {
		row.Status = *fields.Status
	}
	if fields.EndedAt != nil {
		row.EndedAt = *fields.EndedAt
	}
	if fields.ErrorMessage != nil {
		row.ErrorMessage = *fields.ErrorMessage
	}
	return row, nil
}

func TestPersistentTurnServiceLifecycle(t *testing.T) {
	store := &testPersistentTurnStore{}
	now := time.Date(2026, 5, 27, 1, 2, 3, 0, time.UTC)
	svc := &PersistentTurnService{Sessions: store, Now: func() time.Time { return now }}

	turn, err := svc.AdmitTurn(context.Background(), biz.TurnIntent{
		Source:    biz.TurnSourceWS,
		SessionID: "sess-1",
		AgentID:   "agent-1",
		Content:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != biz.TurnStatusQueued || store.created.Status != string(biz.TurnStatusQueued) {
		t.Fatalf("unexpected admitted turn: %+v row=%+v", turn, store.created)
	}

	completed, err := svc.CompleteTurn(context.Background(), turn, biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeCompleted})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != biz.TurnStatusCompleted || store.updated.Status == nil || *store.updated.Status != string(biz.TurnStatusCompleted) {
		t.Fatalf("unexpected completed turn: %+v update=%+v", completed, store.updated)
	}

	failed, err := svc.FailTurn(context.Background(), turn, errors.New("boom"))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != biz.TurnStatusFailed || store.updated.ErrorMessage == nil || *store.updated.ErrorMessage != "boom" {
		t.Fatalf("unexpected failed turn: %+v update=%+v", failed, store.updated)
	}
}
