package event

import (
	"testing"
)

func TestPublishSessionRevisionEnvelopeSyncStatus(t *testing.T) {
	bus := NewBus()
	ch, unsub := bus.Subscribe(SubscribeOptions{SessionID: "sess-1", BufferSize: 4})
	defer unsub()

	PublishSessionRevisionEnvelope(bus, "sess-1", "run-1", "turn-1", "channel", 2, SessionRunStatusSync)

	select {
	case env := <-ch:
		if env.Metadata["status"] != SessionRunStatusSync {
			t.Fatalf("status = %v, want sync", env.Metadata["status"])
		}
		if env.SessionRevision != 2 {
			t.Fatalf("revision = %d, want 2", env.SessionRevision)
		}
	default:
		t.Fatal("expected envelope on bus")
	}
}

func TestPublishSessionRevisionEnvelopeDefaultCompleted(t *testing.T) {
	bus := NewBus()
	ch, unsub := bus.Subscribe(SubscribeOptions{SessionID: "sess-1", BufferSize: 4})
	defer unsub()

	PublishSessionRevisionEnvelope(bus, "sess-1", "run-1", "turn-1", "", 1, "")

	select {
	case env := <-ch:
		if env.Metadata["status"] != SessionRunStatusCompleted {
			t.Fatalf("status = %v, want completed", env.Metadata["status"])
		}
	default:
		t.Fatal("expected envelope on bus")
	}
}
