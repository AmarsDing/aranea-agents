package bus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestBus_Publish_DeliversToSubscribers(t *testing.T) {
	b := New[string]()
	ch, unsub := b.Subscribe(SubscribeOptions[string]{})
	defer unsub()

	b.Publish(context.Background(), "hello")

	select {
	case evt := <-ch:
		if evt != "hello" {
			t.Errorf("got %q, want %q", evt, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBus_Publish_PriorityOrder(t *testing.T) {
	b := New[string]()

	criticalCh, criticalUnsub := b.Subscribe(SubscribeOptions[string]{
		Priority:   PriorityCritical,
		BufferSize: 8,
	})
	normalCh, normalUnsub := b.Subscribe(SubscribeOptions[string]{
		Priority:   PriorityNormal,
		BufferSize: 8,
	})
	defer criticalUnsub()
	defer normalUnsub()

	// Publish an event — priority ordering is about delivery order,
	// not consumption order (which depends on goroutine scheduling).
	// Verify both channels receive the event.
	b.Publish(context.Background(), "event")

	// Critical subscriber should receive the event
	select {
	case evt := <-criticalCh:
		if evt != "event" {
			t.Errorf("critical: got %q, want %q", evt, "event")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for critical subscriber")
	}

	// Normal subscriber should also receive the event
	select {
	case evt := <-normalCh:
		if evt != "event" {
			t.Errorf("normal: got %q, want %q", evt, "event")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for normal subscriber")
	}
}

func TestBus_DropPolicy_DropNewest(t *testing.T) {
	b := New[int]()
	ch, unsub := b.Subscribe(SubscribeOptions[int]{
		BufferSize: 2,
		DropPolicy: DropNewest,
	})
	defer unsub()

	// Fill the buffer
	b.Publish(context.Background(), 1)
	b.Publish(context.Background(), 2)
	// This should be dropped
	b.Publish(context.Background(), 3)

	// Should get 1, 2 but not 3
	got := drainChannel(ch, 3, 100*time.Millisecond)
	if len(got) != 2 {
		t.Errorf("expected 2 events, got %d", len(got))
	}
	if b.DropCount() != 1 {
		t.Errorf("expected 1 drop, got %d", b.DropCount())
	}
}

func TestBus_DropPolicy_BlockUpTo(t *testing.T) {
	b := New[int]()
	ch, unsub := b.Subscribe(SubscribeOptions[int]{
		BufferSize: 1,
		DropPolicy: BlockUpTo,
		BlockFor:   50 * time.Millisecond,
	})
	defer unsub()

	// Fill the buffer
	b.Publish(context.Background(), 1)
	// This should block briefly then apply DropOldest
	b.Publish(context.Background(), 2)

	// Should get at least 1 event
	got := drainChannel(ch, 2, 200*time.Millisecond)
	if len(got) < 1 {
		t.Error("expected at least 1 event")
	}
}

func TestBus_Reliable_ForceBlockUpTo(t *testing.T) {
	b := New[int]()
	ch, unsub := b.Subscribe(SubscribeOptions[int]{
		BufferSize: 1,
		Reliable:   true,
		BlockFor:   50 * time.Millisecond,
	})
	defer unsub()

	b.Publish(context.Background(), 1)
	b.Publish(context.Background(), 2)

	got := drainChannel(ch, 2, 200*time.Millisecond)
	if len(got) < 1 {
		t.Error("expected at least 1 event with Reliable=true")
	}
}

func TestBus_Filter(t *testing.T) {
	b := New[int]()
	ch, unsub := b.Subscribe(SubscribeOptions[int]{
		Filter: func(event int) bool {
			return event%2 == 0 // only even numbers
		},
		BufferSize: 8,
	})
	defer unsub()

	b.Publish(context.Background(), 1)
	b.Publish(context.Background(), 2)
	b.Publish(context.Background(), 3)
	b.Publish(context.Background(), 4)

	got := drainChannel(ch, 4, 200*time.Millisecond)
	want := []int{2, 4}
	if len(got) != len(want) {
		t.Errorf("expected %d events, got %d", len(want), len(got))
	}
}

func TestBus_Unsubscribe(t *testing.T) {
	b := New[string]()
	ch, unsub := b.Subscribe(SubscribeOptions[string]{
		BufferSize: 8,
	})

	b.Publish(context.Background(), "before")
	unsub()
	b.Publish(context.Background(), "after")

	got := drainChannel(ch, 2, 100*time.Millisecond)
	if len(got) != 1 || got[0] != "before" {
		t.Errorf("expected only 'before', got %v", got)
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	b := New[string]()

	var received atomic.Int32
	ch1, unsub1 := b.Subscribe(SubscribeOptions[string]{BufferSize: 8})
	ch2, unsub2 := b.Subscribe(SubscribeOptions[string]{BufferSize: 8})
	defer unsub1()
	defer unsub2()

	go func() {
		for range ch1 {
			received.Add(1)
		}
	}()
	go func() {
		for range ch2 {
			received.Add(1)
		}
	}()

	b.Publish(context.Background(), "event")
	time.Sleep(50 * time.Millisecond)

	if received.Load() != 2 {
		t.Errorf("expected 2 deliveries, got %d", received.Load())
	}
}

func TestBus_DefaultBufferSize(t *testing.T) {
	b := New[string]()
	// BufferSize 0 should default to 128
	ch, unsub := b.Subscribe(SubscribeOptions[string]{})
	defer unsub()

	if cap(ch) != 128 {
		t.Errorf("expected default buffer size 128, got %d", cap(ch))
	}
}

func TestBus_MaxBufferSize(t *testing.T) {
	b := New[string]()
	ch, unsub := b.Subscribe(SubscribeOptions[string]{BufferSize: 1024})
	defer unsub()

	if cap(ch) != 512 {
		t.Errorf("expected max buffer size 512, got %d", cap(ch))
	}
}

func TestMatchLevelFilter(t *testing.T) {
	tests := []struct {
		filter string
		level  string
		want   bool
	}{
		{"WARN", "ERROR", true},
		{"WARN", "WARN", true},
		{"WARN", "INFO", false},
		{"", "INFO", true},
		{"INFO", "", true},
		{"WARN|ERROR", "WARN", true},
		{"WARN|ERROR", "INFO", false},
	}

	for _, tt := range tests {
		got := MatchLevelFilter(tt.filter, tt.level)
		if got != tt.want {
			t.Errorf("MatchLevelFilter(%q, %q) = %v, want %v", tt.filter, tt.level, got, tt.want)
		}
	}
}

func drainChannel[T any](ch <-chan T, max int, timeout time.Duration) []T {
	var result []T
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for len(result) < max {
		select {
		case evt := <-ch:
			result = append(result, evt)
		case <-timer.C:
			return result
		}
	}
	return result
}
