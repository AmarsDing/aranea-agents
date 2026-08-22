package knowledge

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
)

func TestDocumentStateMachine_ValidTransitions(t *testing.T) {
	sm := NewDocumentStateMachine()
	cases := []struct {
		from  DocumentState
		event DocumentEvent
		want  DocumentState
	}{
		{DocumentStatePending, DocumentEventStart, DocumentStateIndexing},
		{DocumentStatePending, DocumentEventFail, DocumentStateError},
		{DocumentStateIndexing, DocumentEventComplete, DocumentStateIndexed},
		{DocumentStateIndexing, DocumentEventFail, DocumentStateError},
		{DocumentStateIndexing, DocumentEventReset, DocumentStatePending},
		{DocumentStateIndexed, DocumentEventStart, DocumentStateIndexing},
		{DocumentStateIndexed, DocumentEventFail, DocumentStateError},
		{DocumentStateIndexed, DocumentEventReset, DocumentStatePending},
		{DocumentStateError, DocumentEventStart, DocumentStateIndexing},
		{DocumentStateError, DocumentEventReset, DocumentStatePending},
	}
	for _, tc := range cases {
		got, err := sm.Transition(tc.from, tc.event)
		if err != nil {
			t.Errorf("Transition(%q, %q): unexpected error: %v", tc.from, tc.event, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%q, %q) = %q, want %q", tc.from, tc.event, got, tc.want)
		}
	}
}

func TestDocumentStateMachine_InvalidTransitions(t *testing.T) {
	sm := NewDocumentStateMachine()
	cases := []struct {
		from  DocumentState
		event DocumentEvent
	}{
		{DocumentStatePending, DocumentEventComplete},
		{DocumentStateIndexed, DocumentEventComplete},
		{DocumentStateError, DocumentEventComplete},
		{DocumentStateError, DocumentEventFail},
		{DocumentStatePending, DocumentEventReset},
		{DocumentStateNone, DocumentEventStart},
		{DocumentStateNone, DocumentEventComplete},
	}
	for _, tc := range cases {
		_, err := sm.Transition(tc.from, tc.event)
		if !errors.Is(err, shared.ErrInvalidTransition) {
			t.Errorf("Transition(%q, %q) err = %v, want ErrInvalidTransition", tc.from, tc.event, err)
		}
	}
}

func TestDocumentEventFor(t *testing.T) {
	ev, ok := documentEventFor(DocumentStatePending, DocumentStateIndexing)
	if !ok || ev != DocumentEventStart {
		t.Fatalf("pending→indexing = (%q, %v), want start", ev, ok)
	}
	if _, ok := documentEventFor(DocumentStatePending, DocumentStateIndexed); ok {
		t.Fatal("pending→indexed must not infer an event")
	}
	if _, ok := documentEventFor(DocumentStatePending, DocumentState("ready")); ok {
		t.Fatal("unknown target must not infer an event")
	}
	if _, ok := documentEventFor(DocumentStateIndexing, DocumentStateIndexing); ok {
		t.Fatal("same-state must not infer an event")
	}
}

func TestNormalizeDocumentState(t *testing.T) {
	if got := NormalizeDocumentState(""); got != DocumentStatePending {
		t.Fatalf("empty status = %q, want pending", got)
	}
	if got := NormalizeDocumentState("indexing"); got != DocumentStateIndexing {
		t.Fatalf("indexing = %q", got)
	}
}
