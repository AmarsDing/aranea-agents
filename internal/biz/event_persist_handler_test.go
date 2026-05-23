package biz

import (
	"testing"

	"aranea-agents/internal/event/contract"
)

func TestShouldPersistEnvelope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  contract.EnvelopeType
		want bool
	}{
		{contract.EnvelopeTypeTextDelta, false},
		{contract.EnvelopeTypeMemberDelta, false},
		{contract.EnvelopeTypeTextDone, true},
		{contract.EnvelopeTypeToolResult, true},
		{contract.EnvelopeTypeLog, false},
		{contract.EnvelopeTypeFlowLog, false},
	}
	for _, tc := range cases {
		env := contract.NewEnvelope(tc.typ, "agent", "sess-1")
		if got := shouldPersistEnvelope(env); got != tc.want {
			t.Fatalf("type %s: got %v want %v", tc.typ, got, tc.want)
		}
	}
	if shouldPersistEnvelope(contract.Envelope{Type: contract.EnvelopeTypeError}) {
		t.Fatal("empty id should not persist")
	}
}

func TestEnvelopeToStoreRecord(t *testing.T) {
	t.Parallel()
	env := contract.NewEnvelope(contract.EnvelopeTypeToolCall, "agent", "sess-1")
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
