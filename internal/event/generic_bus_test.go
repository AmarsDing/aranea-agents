package event

import (
	"testing"
)

func TestGenericBus_SubscriberCount(t *testing.T) {
	bus := NewGenericBus[string](nil, nil)
	if got := bus.SubscriberCount(); got != 0 {
		t.Fatalf("initial subscriber count=%d want 0", got)
	}

	_, unsub1 := bus.Subscribe(GenericSubscribeOptions[string]{BufferSize: 1})
	_, unsub2 := bus.Subscribe(GenericSubscribeOptions[string]{BufferSize: 1})
	if got := bus.SubscriberCount(); got != 2 {
		t.Fatalf("subscriber count=%d want 2", got)
	}

	unsub1()
	if got := bus.SubscriberCount(); got != 1 {
		t.Fatalf("after unsub1 subscriber count=%d want 1", got)
	}

	// Double unsubscribe must not decrement twice.
	unsub2()
	unsub2()
	if got := bus.SubscriberCount(); got != 0 {
		t.Fatalf("after unsub2 x2 subscriber count=%d want 0", got)
	}
}
