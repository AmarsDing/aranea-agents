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
	if turn.Status != biz.CanonicalTurnStatusQueued || store.created.Status != string(biz.CanonicalTurnStatusQueued) {
		t.Fatalf("unexpected admitted turn: %+v row=%+v", turn, store.created)
	}

	completed, err := svc.CompleteTurn(context.Background(), turn, biz.TurnResult{Outcome: biz.TurnOutcomeCompleted})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != biz.CanonicalTurnStatusCompleted || store.updated.Status == nil || *store.updated.Status != string(biz.CanonicalTurnStatusCompleted) {
		t.Fatalf("unexpected completed turn: %+v update=%+v", completed, store.updated)
	}

	failed, err := svc.FailTurn(context.Background(), turn, errors.New("boom"))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != biz.CanonicalTurnStatusFailed || store.updated.ErrorMessage == nil || *store.updated.ErrorMessage != "boom" {
		t.Fatalf("unexpected failed turn: %+v update=%+v", failed, store.updated)
	}
}

// TestAdmitTurn_IdempotencyKeyScoped verifies C-13: client idempotency keys are
// stored as "source:key" on the SessionTurn row so CreateTurn can dedupe.
func TestAdmitTurn_IdempotencyKeyScoped(t *testing.T) {
	store := &testPersistentTurnStore{}
	svc := &PersistentTurnService{Sessions: store, Now: func() time.Time { return time.Now().UTC() }}

	_, err := svc.AdmitTurn(context.Background(), biz.TurnIntent{
		Source:         biz.TurnSourceWS,
		SessionID:      "sess-1",
		AgentID:        "agent-1",
		Content:        "hello",
		IdempotencyKey: "client-key-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.created.IdempotencyKey != "ws:client-key-1" {
		t.Fatalf("IdempotencyKey = %q, want %q", store.created.IdempotencyKey, "ws:client-key-1")
	}
}

// idempotentTurnStore returns the first created turn on matching idempotency key.
type idempotentTurnStore struct {
	byKey map[string]biz.SessionTurn
}

func (s *idempotentTurnStore) CreateTurn(_ context.Context, turn biz.SessionTurn) (biz.SessionTurn, error) {
	if s.byKey == nil {
		s.byKey = make(map[string]biz.SessionTurn)
	}
	if turn.IdempotencyKey != "" {
		if existing, ok := s.byKey[turn.SessionID+"|"+turn.IdempotencyKey]; ok {
			return existing, nil
		}
		s.byKey[turn.SessionID+"|"+turn.IdempotencyKey] = turn
	}
	return turn, nil
}

func (s *idempotentTurnStore) UpdateTurn(_ context.Context, id string, fields biz.SessionTurnUpdateFields) (biz.SessionTurn, error) {
	return biz.SessionTurn{ID: id}, nil
}

// TestAdmitTurn_SameIdempotencyKeyReturnsCanonical verifies retries with the
// same key return the original turn ID (C-13).
func TestAdmitTurn_SameIdempotencyKeyReturnsCanonical(t *testing.T) {
	store := &idempotentTurnStore{}
	svc := &PersistentTurnService{Sessions: store, Now: func() time.Time { return time.Now().UTC() }}
	intent := biz.TurnIntent{
		Source:         biz.TurnSourceWeb,
		SessionID:      "sess-dup",
		AgentID:        "agent-1",
		Content:        "hello",
		IdempotencyKey: "retry-me",
	}
	first, err := svc.AdmitTurn(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.AdmitTurn(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same canonical turn ID, got %q then %q", first.ID, second.ID)
	}
}

func TestTurnFromSessionTurn(t *testing.T) {
	cases := []struct {
		name string
		row  biz.SessionTurn
		base biz.CanonicalTurn
		want biz.CanonicalTurn
	}{
		{
			name: "row_overrides_base_fields",
			row:  biz.SessionTurn{ID: "r1", SessionID: "s1", RunID: "run-1", AgentID: "a1", TeamID: "t1", Status: "completed"},
			base: biz.CanonicalTurn{ID: "old", SessionID: "old", AgentID: "old"},
			want: biz.CanonicalTurn{ID: "r1", SessionID: "s1", RunID: "run-1", AgentID: "a1", TeamID: "t1", Status: biz.CanonicalTurnStatus("completed")},
		},
		{
			name: "empty_status_keeps_base",
			row:  biz.SessionTurn{ID: "r2", SessionID: "s2", Status: ""},
			base: biz.CanonicalTurn{Status: biz.CanonicalTurnStatusQueued},
			want: biz.CanonicalTurn{ID: "r2", SessionID: "s2", Status: biz.CanonicalTurnStatusQueued},
		},
		{
			name: "non_empty_status_overrides",
			row:  biz.SessionTurn{ID: "r3", Status: "failed"},
			base: biz.CanonicalTurn{Status: biz.CanonicalTurnStatusQueued},
			want: biz.CanonicalTurn{ID: "r3", Status: biz.CanonicalTurnStatus("failed")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := turnFromSessionTurn(tc.row, tc.base)
			if got.ID != tc.want.ID || got.SessionID != tc.want.SessionID || got.RunID != tc.want.RunID || got.AgentID != tc.want.AgentID || got.TeamID != tc.want.TeamID || got.Status != tc.want.Status {
				t.Fatalf("got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestPtrString(t *testing.T) {
	v := "hello"
	p := ptrString(v)
	if p == nil || *p != v {
		t.Fatalf("ptrString(%q) = %v, want *%q", v, p, v)
	}
	s := ""
	ps := ptrString(s)
	if ps == nil || *ps != s {
		t.Fatal("ptrString empty should return non-nil pointer to empty string")
	}
}

func TestPtrInt(t *testing.T) {
	v := 42
	p := ptrInt(v)
	if p == nil || *p != v {
		t.Fatalf("ptrInt(%d) = %v, want *%d", v, p, v)
	}
	z := 0
	pz := ptrInt(z)
	if pz == nil || *pz != z {
		t.Fatal("ptrInt(0) should return non-nil pointer to 0")
	}
}
