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

	completed, err := svc.CompleteTurn(context.Background(), turn, biz.TurnResult{Outcome: biz.TurnOutcomeCompleted})
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

func TestTurnFromSessionTurn(t *testing.T) {
	cases := []struct {
		name string
		row  biz.SessionTurn
		base biz.Turn
		want biz.Turn
	}{
		{
			name: "row_overrides_base_fields",
			row:  biz.SessionTurn{ID: "r1", SessionID: "s1", RunID: "run-1", AgentID: "a1", TeamID: "t1", Status: "completed"},
			base: biz.Turn{ID: "old", SessionID: "old", AgentID: "old"},
			want: biz.Turn{ID: "r1", SessionID: "s1", RunID: "run-1", AgentID: "a1", TeamID: "t1", Status: biz.TurnStatus("completed")},
		},
		{
			name: "empty_status_keeps_base",
			row:  biz.SessionTurn{ID: "r2", SessionID: "s2", Status: ""},
			base: biz.Turn{Status: biz.TurnStatusQueued},
			want: biz.Turn{ID: "r2", SessionID: "s2", Status: biz.TurnStatusQueued},
		},
		{
			name: "non_empty_status_overrides",
			row:  biz.SessionTurn{ID: "r3", Status: "failed"},
			base: biz.Turn{Status: biz.TurnStatusQueued},
			want: biz.Turn{ID: "r3", Status: biz.TurnStatus("failed")},
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
