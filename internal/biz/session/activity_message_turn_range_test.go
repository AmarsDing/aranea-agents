package session

import (
	"context"
	"testing"
	"time"
)

type rangeLister struct {
	captureLister
	called bool
	res    ActivityTurnRangeResult
	err    error
}

func (r *rangeLister) ListBySessionAfterTurnNumber(context.Context, string, int) (ActivityTurnRangeResult, error) {
	r.called = true
	return r.res, r.err
}

func TestListMessagesAfterTurn_UsesTurnRangeLister(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rl := &rangeLister{
		res: ActivityTurnRangeResult{
			Entries: []ActivityEntry{
				{ID: "u2", Kind: "task", SessionID: "s1", TurnID: "t2", Content: "q2", Timestamp: now},
			},
			TurnSeq: map[string]int{"t2": 2},
		},
	}
	r := NewActivityMessageReader(rl)
	msgs, err := r.ListMessagesAfterTurn(context.Background(), "s1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !rl.called {
		t.Fatal("expected range lister to be called")
	}
	if rl.fullScans != 0 {
		t.Fatalf("fullScans=%d want 0", rl.fullScans)
	}
	if len(msgs) != 1 || msgs[0].TurnNumber != 2 {
		t.Fatalf("msgs=%+v", msgs)
	}
}

func TestListMessagesAfterTurn_FallsBackWhenRangeUnavailable(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rl := &rangeLister{
		captureLister: captureLister{entries: map[string]ActivityEntry{
			"u1": {ID: "u1", Kind: "task", SessionID: "s1", TurnID: "t1", Content: "q1", Timestamp: now},
			"u2": {ID: "u2", Kind: "task", SessionID: "s1", TurnID: "t2", Content: "q2", Timestamp: now.Add(time.Second)},
		}},
		err: ErrTurnRangeUnavailable,
	}
	r := NewActivityMessageReader(rl)
	msgs, err := r.ListMessagesAfterTurn(context.Background(), "s1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !rl.called || rl.fullScans != 1 {
		t.Fatalf("called=%v fullScans=%d", rl.called, rl.fullScans)
	}
	if len(msgs) == 0 {
		t.Fatal("expected fallback messages after turn 1")
	}
	for _, m := range msgs {
		if m.TurnNumber <= 1 {
			t.Fatalf("got turn %d for %s", m.TurnNumber, m.ID)
		}
	}
}
