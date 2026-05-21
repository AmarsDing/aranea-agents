package biz

import (
	"testing"

	"aranea-agents/internal/event"
)

func TestShouldPersistEnvelope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  event.EnvelopeType
		want bool
	}{
		{event.EnvelopeTypeTextDelta, false},
		{event.EnvelopeTypeMemberDelta, false},
		{event.EnvelopeTypeTextDone, true},
		{event.EnvelopeTypeToolResult, true},
		{event.EnvelopeTypeLog, false},
		{event.EnvelopeTypeFlowLog, false},
	}
	for _, tc := range cases {
		env := event.NewEnvelope(tc.typ, "agent", "sess-1")
		if got := shouldPersistEnvelope(env); got != tc.want {
			t.Fatalf("type %s: got %v want %v", tc.typ, got, tc.want)
		}
	}
	if shouldPersistEnvelope(event.Envelope{Type: event.EnvelopeTypeError}) {
		t.Fatal("empty id should not persist")
	}
}

func TestEnvelopeToStoreRecord(t *testing.T) {
	t.Parallel()
	env := event.NewEnvelope(event.EnvelopeTypeToolCall, "agent", "sess-1")
	rec, ok := envelopeToStoreRecord(env)
	if !ok {
		t.Fatal("expected ok")
	}
	if rec.ID != env.ID || rec.SessionID != "sess-1" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if rec.EnvelopeJSON == "" {
		t.Fatal("expected envelope json")
	}
}
