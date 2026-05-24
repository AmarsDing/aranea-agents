package event_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/event"
)

// Regression: Publish must not panic when Unsubscribe closes the channel concurrently.
func TestBusPublishUnsubscribeRace(t *testing.T) {
	b := event.NewBus()
	ctx := context.Background()
	env := event.NewEnvelope(event.EnvelopeTypeFlowLog, "system", "")
	env.Channel = "monitor"

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, unsub := b.Subscribe(event.SubscribeOptions{BufferSize: 4, Channel: "monitor"})
			defer unsub()
			deadline := time.After(50 * time.Millisecond)
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
				case <-deadline:
					return
				}
			}
		}()
	}

	for i := 0; i < 5000; i++ {
		b.Publish(ctx, env)
	}
	wg.Wait()
}
