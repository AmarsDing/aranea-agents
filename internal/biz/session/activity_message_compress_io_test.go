package session

import (
	"context"
	"testing"
	"time"
)

type captureLister struct {
	fullScans int
	byIDs     int
	entries   map[string]ActivityEntry
}

func (c *captureLister) ListBySessionTurn(context.Context, string, string) ([]ActivityEntry, error) {
	return nil, nil
}
func (c *captureLister) ListBySession(context.Context, string) ([]ActivityEntry, error) {
	c.fullScans++
	out := make([]ActivityEntry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, e)
	}
	return out, nil
}
func (c *captureLister) ListByIDs(_ context.Context, _ string, ids []string) ([]ActivityEntry, error) {
	c.byIDs++
	out := make([]ActivityEntry, 0, len(ids))
	for _, id := range ids {
		if e, ok := c.entries[id]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

func TestListMessagesByIDs_UsesActivityIDLister(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	l := &captureLister{entries: map[string]ActivityEntry{
		"a1": {ID: "a1", Kind: "task", SessionID: "s1", Content: "hi", Timestamp: now},
		"a2": {ID: "a2", Kind: "reply", SessionID: "s1", Content: "yo", Timestamp: now.Add(time.Second)},
	}}
	r := NewActivityMessageReader(l)
	msgs, err := r.ListMessagesByIDs(context.Background(), "s1", []string{"a1"})
	if err != nil {
		t.Fatal(err)
	}
	if l.byIDs != 1 || l.fullScans != 0 {
		t.Fatalf("byIDs=%d fullScans=%d want byIDs=1 fullScans=0", l.byIDs, l.fullScans)
	}
	if len(msgs) != 1 || msgs[0].ID != "a1" {
		t.Fatalf("msgs=%+v", msgs)
	}
}

func TestListMessagesAfterTurn_FiltersOrdinals(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	l := &captureLister{entries: map[string]ActivityEntry{
		"u1": {ID: "u1", Kind: "task", SessionID: "s1", TurnID: "t1", Content: "q1", Timestamp: now},
		"a1": {ID: "a1", Kind: "reply", SessionID: "s1", TurnID: "t1", Content: "a1", Timestamp: now.Add(time.Second)},
		"u2": {ID: "u2", Kind: "task", SessionID: "s1", TurnID: "t2", Content: "q2", Timestamp: now.Add(2 * time.Second)},
	}}
	r := NewActivityMessageReader(l)
	msgs, err := r.ListMessagesAfterTurn(context.Background(), "s1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected messages after turn 1")
	}
	for _, m := range msgs {
		if m.TurnNumber <= 1 {
			t.Fatalf("got turn %d for %s, want >1", m.TurnNumber, m.ID)
		}
	}
}
