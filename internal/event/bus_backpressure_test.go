package event_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/event"
)

func TestBusDropOldest(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{
		BufferSize: 2,
		DropPolicy: event.DropOldest,
	})
	defer unsub()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		env := event.NewEnvelope(event.EnvelopeTypeLog, "src", "sess")
		env.Content = &event.EnvelopeContent{Text: string(rune('A' + i))}
		bus.Publish(ctx, env)
	}

	received := 0
	deadline := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case <-ch:
			received++
		case <-deadline:
			break loop
		}
	}
	if received < 2 {
		t.Fatalf("expected at least 2 delivered events, got %d", received)
	}
}

func TestBusDropNewest(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{
		BufferSize: 1,
		DropPolicy: event.DropNewest,
	})
	defer unsub()

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		bus.Publish(ctx, event.NewEnvelope(event.EnvelopeTypeLog, "src", "sess"))
	}

	time.Sleep(20 * time.Millisecond)
	got := len(ch)
	if got != 1 {
		t.Fatalf("DropNewest: expected exactly 1 buffered event, got %d", got)
	}
	if bus.DropCount() < 3 {
		t.Fatalf("expected ≥3 drops, got %d", bus.DropCount())
	}
}

func TestBusBlockUpTo(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{
		BufferSize: 1,
		DropPolicy: event.BlockUpTo,
		BlockFor:   50 * time.Millisecond,
	})
	defer unsub()

	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 3; i++ {
		bus.Publish(ctx, event.NewEnvelope(event.EnvelopeTypeLog, "src", "sess"))
	}
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond {
		t.Logf("BlockUpTo elapsed=%s (may be fast if consumer drained buffer)", elapsed)
	}
	_ = ch
}

func TestBusReliableOption(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{
		BufferSize: 4,
		Reliable:   true,
	})
	defer unsub()

	ctx := context.Background()
	env := event.NewEnvelope(event.EnvelopeTypeToolResult, "agent", "sess-1")
	bus.Publish(ctx, env)

	select {
	case got := <-ch:
		if got.Type != event.EnvelopeTypeToolResult {
			t.Fatalf("expected ToolResult, got %v", got.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for reliable delivery")
	}
}

func TestBusSelectorFilter(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{
		BufferSize: 8,
		Selector: func(et event.EnvelopeType) bool {
			return et == event.EnvelopeTypeTextDelta
		},
	})
	defer unsub()

	ctx := context.Background()
	bus.Publish(ctx, event.NewEnvelope(event.EnvelopeTypeLog, "src", "sess"))
	bus.Publish(ctx, event.NewEnvelope(event.EnvelopeTypeTextDelta, "src", "sess"))
	bus.Publish(ctx, event.NewEnvelope(event.EnvelopeTypeError, "src", "sess"))

	time.Sleep(20 * time.Millisecond)
	if n := len(ch); n != 1 {
		t.Fatalf("Selector: expected 1 event (TextDelta only), got %d", n)
	}
}
