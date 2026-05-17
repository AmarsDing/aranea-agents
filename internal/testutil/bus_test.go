package testutil_test

import (
	"context"
	"testing"

	"aranea-agents/internal/event"
	"aranea-agents/internal/testutil"
)

func TestRecordingBusCollectsEvents(t *testing.T) {
	bus := testutil.NewRecordingBus()

	env1 := event.NewEnvelope(event.EnvelopeTypeTextDelta, "agent-1", "sess-a")
	env2 := event.NewEnvelope(event.EnvelopeTypeError, "agent-1", "sess-a")

	bus.Publish(context.Background(), env1)
	bus.Publish(context.Background(), env2)

	events := bus.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestRecordingBusEventsOfType(t *testing.T) {
	bus := testutil.NewRecordingBus()

	bus.Publish(context.Background(), event.NewEnvelope(event.EnvelopeTypeTextDelta, "a", "s"))
	bus.Publish(context.Background(), event.NewEnvelope(event.EnvelopeTypeError, "a", "s"))
	bus.Publish(context.Background(), event.NewEnvelope(event.EnvelopeTypeTextDelta, "a", "s"))

	textEvents := bus.EventsOfType(event.EnvelopeTypeTextDelta)
	if len(textEvents) != 2 {
		t.Fatalf("expected 2 text events, got %d", len(textEvents))
	}

	errEvents := bus.EventsOfType(event.EnvelopeTypeError)
	if len(errEvents) != 1 {
		t.Fatalf("expected 1 error event, got %d", len(errEvents))
	}
}

func TestRecordingBusDropCountZero(t *testing.T) {
	bus := testutil.NewRecordingBus()
	if bus.DropCount() != 0 {
		t.Fatal("expected 0 drop count")
	}
}

func TestRecordingBusSubscribeReturnsChannel(t *testing.T) {
	bus := testutil.NewRecordingBus()
	ch, cancel := bus.Subscribe(event.SubscribeOptions{SessionID: "s"})
	defer cancel()
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}
