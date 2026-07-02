package server

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// fakeHub captures BroadcastToSession calls for testing.
type fakeHub struct {
	mu   sync.Mutex
	msgs map[string][][]byte // sessionID → messages
}

func newFakeHub() *fakeHub {
	return &fakeHub{msgs: make(map[string][][]byte)}
}

// BroadcastToSession implements WSMessageBroadcaster.
func (f *fakeHub) BroadcastToSession(sid string, msg []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs[sid] = append(f.msgs[sid], msg)
}

// Msgs returns a snapshot of messages received for the given session.
func (f *fakeHub) Msgs(sid string) [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.msgs[sid]))
	copy(out, f.msgs[sid])
	return out
}

// TestWSV2Subscriber_ForwardsEvents verifies that a v2 event published to V2Bus
// is forwarded to the WS hub via BroadcastToSession, wrapped in a wsEnvelope
// with Type="v2_event" and the event's EventKind.
//
// Deviation 1: uses biz.NewTaskCreatedEvent factory (not struct literal).
// The factory derives spiritSessionID from task.SessionID.
func TestWSV2Subscriber_ForwardsEvents(t *testing.T) {
	t.Parallel()
	bus := event.NewV2Bus()
	hub := newFakeHub()
	sub := NewWSV2Subscriber(bus, hub, loggateway.NewNoop())
	defer sub.Close()

	// Deviation 1: use factory. Task.SessionID is used as spiritSessionID
	// by the factory (see internal/biz/event_factory.go).
	task := biz.Task{
		ID:        "t-1",
		SessionID: "sess-1",
		Status:    biz.TaskStatusRunning,
		Version:   1,
	}
	bus.Publish(context.Background(), biz.NewTaskCreatedEvent(task))

	// Wait for async delivery (subscriber goroutine + bus fan-out).
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("expected 1 message on sess-1, got %d", len(hub.Msgs("sess-1")))
		default:
		}
		if len(hub.Msgs("sess-1")) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Verify the envelope content.
	msg := hub.Msgs("sess-1")[0]
	var env wsEnvelope
	if err := json.Unmarshal(msg, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != "v2_event" {
		t.Errorf("envelope.Type = %q, want %q", env.Type, "v2_event")
	}
	if env.Kind != string(biz.EventKindTaskCreated) {
		t.Errorf("envelope.Kind = %q, want %q", env.Kind, biz.EventKindTaskCreated)
	}
}

// TestWSV2Subscriber_CloseStopsGoroutine verifies that Close stops the
// subscriber goroutine and does not leak.
func TestWSV2Subscriber_CloseStopsGoroutine(t *testing.T) {
	t.Parallel()
	bus := event.NewV2Bus()
	hub := newFakeHub()
	sub := NewWSV2Subscriber(bus, hub, loggateway.NewNoop())

	// Close should return promptly and not block.
	done := make(chan struct{})
	go func() {
		_ = sub.Close()
		close(done)
	}()
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return within 2s")
	}

	// After Close, publishing should not panic or deliver to hub.
	bus.Publish(context.Background(), biz.NewTaskCreatedEvent(biz.Task{
		ID: "t-2", SessionID: "sess-2", Version: 1,
	}))
	time.Sleep(20 * time.Millisecond)
	if got := len(hub.Msgs("sess-2")); got != 0 {
		t.Errorf("expected 0 messages after Close, got %d", got)
	}
}

// TestWSV2Subscriber_NilLoggerDoesNotPanic verifies that a nil logger is
// replaced with a Noop logger inside the constructor.
func TestWSV2Subscriber_NilLoggerDoesNotPanic(t *testing.T) {
	t.Parallel()
	bus := event.NewV2Bus()
	hub := newFakeHub()
	sub := NewWSV2Subscriber(bus, hub, nil)
	defer sub.Close()

	bus.Publish(context.Background(), biz.NewTaskCreatedEvent(biz.Task{
		ID: "t-3", SessionID: "sess-3", Version: 1,
	}))
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			return // event may or may not arrive; we just want no panic
		default:
		}
		if len(hub.Msgs("sess-3")) >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
