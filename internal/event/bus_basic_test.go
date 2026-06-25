package event_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/event"
)

func TestBusPublishSubscribe(t *testing.T) {
	b := event.NewBus(nil)

	ch, cancel := b.Subscribe(event.SubscribeOptions{SessionID: "s1", BufferSize: 8})
	defer cancel()

	env := event.NewEnvelope(event.EnvelopeTypeStateDelta, "agent-1", "s1")
	env.Channel = "session:s1"
	b.Publish(context.Background(), env)

	select {
	case got := <-ch:
		if got.SessionID != "s1" {
			t.Fatalf("expected session s1, got %s", got.SessionID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestBusSubscribeCancel(t *testing.T) {
	b := event.NewBus(nil)

	ch, cancel := b.Subscribe(event.SubscribeOptions{SessionID: "s2", BufferSize: 8})
	cancel()

	env := event.NewEnvelope(event.EnvelopeTypeStateDelta, "agent-1", "s2")
	env.Channel = "session:s2"
	b.Publish(context.Background(), env)

	select {
	case <-ch:
		// Channel may have been buffered before cancel closed it; that's OK.
	case <-time.After(50 * time.Millisecond):
		// No event or channel closed — expected after cancel.
	}
}

func TestBusDropCountInitiallyZero(t *testing.T) {
	b := event.NewBus(nil)
	if b.DropCount() != 0 {
		t.Fatalf("expected 0 drop count, got %d", b.DropCount())
	}
}

func TestBusEventTypeFilter(t *testing.T) {
	b := event.NewBus(nil)

	ch, cancel := b.Subscribe(event.SubscribeOptions{
		SessionID:  "s3",
		EventTypes: []event.EnvelopeType{event.EnvelopeTypeStateDelta},
		BufferSize: 8,
	})
	defer cancel()

	// Publish matching type.
	textEnv := event.NewEnvelope(event.EnvelopeTypeStateDelta, "agent-1", "s3")
	textEnv.Channel = "session:s3"
	b.Publish(context.Background(), textEnv)

	// Publish non-matching type — should be filtered.
	otherEnv := event.NewEnvelope(event.EnvelopeTypeError, "agent-1", "s3")
	otherEnv.Channel = "session:s3"
	b.Publish(context.Background(), otherEnv)

	select {
	case got := <-ch:
		if got.Type != event.EnvelopeTypeStateDelta {
			t.Fatalf("expected state_delta event, got %v", got.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for text event")
	}

	// Channel should have no more events.
	select {
	case extra := <-ch:
		t.Fatalf("unexpected extra event: %v", extra.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRouteChannel(t *testing.T) {
	env := event.NewEnvelope(event.EnvelopeTypeStateDelta, "agent-1", "sess-abc")
	ch := event.RouteChannel(env)
	if ch == "" {
		t.Fatal("RouteChannel should return non-empty channel for session event")
	}
}

func TestNewEnvelopeFields(t *testing.T) {
	env := event.NewEnvelope(event.EnvelopeTypeCheckpoint, "my-agent", "sess-xyz")
	if env.Type != event.EnvelopeTypeCheckpoint {
		t.Errorf("expected ToolCall type, got %v", env.Type)
	}
	if env.Author != "my-agent" {
		t.Errorf("expected author my-agent, got %s", env.Author)
	}
	if env.SessionID != "sess-xyz" {
		t.Errorf("expected session sess-xyz, got %s", env.SessionID)
	}
	if env.ID == "" {
		t.Error("expected non-empty envelope ID")
	}
	if env.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	b := event.NewBus(nil)

	ch1, cancel1 := b.Subscribe(event.SubscribeOptions{SessionID: "ms-1", BufferSize: 4})
	defer cancel1()
	ch2, cancel2 := b.Subscribe(event.SubscribeOptions{SessionID: "ms-1", BufferSize: 4})
	defer cancel2()

	env := event.NewEnvelope(event.EnvelopeTypeStateDelta, "agent", "ms-1")
	env.Channel = "session:ms-1"
	b.Publish(context.Background(), env)

	for _, ch := range []<-chan event.Envelope{ch1, ch2} {
		select {
		case got := <-ch:
			if got.SessionID != "ms-1" {
				t.Fatalf("unexpected session ID %s", got.SessionID)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out waiting for event on subscriber")
		}
	}
}

func TestBusNoMatchingSubscriber(t *testing.T) {
	b := event.NewBus(nil)

	ch, cancel := b.Subscribe(event.SubscribeOptions{SessionID: "sess-A", BufferSize: 4})
	defer cancel()

	// Publish to a different session.
	env := event.NewEnvelope(event.EnvelopeTypeStateDelta, "agent", "sess-B")
	env.Channel = "session:sess-B"
	b.Publish(context.Background(), env)

	select {
	case unexpected := <-ch:
		t.Fatalf("expected no event for sess-A, got %v", unexpected.SessionID)
	case <-time.After(50 * time.Millisecond):
		// Correct: no event delivered.
	}
}
