package biz

import (
	"context"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

type EventBusConsumer struct {
	eventBus event.Bus
	buffer   *eventBufferHandler
	runner   *runnerCompletionHandler
	state    *stateDeltaHandler
	persist  *eventPersistHandler
}

func NewEventBusConsumer(
	eventBus event.Bus,
	eventBuffer *event.Buffer,
	sessions *SessionUsecase,
	usage *UsageUsecase,
	monitor *MonitorUsecase,
	memWorker *TurnMemoryWorker,
	eventStore *EventStoreUsecase,
) *EventBusConsumer {
	return &EventBusConsumer{
		eventBus: eventBus,
		buffer:   newEventBufferHandler(eventBuffer),
		runner:   newRunnerCompletionHandler(sessions, usage, monitor, memWorker),
		state:    newStateDeltaHandler(sessions),
		persist:  newEventPersistHandler(eventStore),
	}
}

func (c *EventBusConsumer) Start(ctx context.Context) {
	if c.persist != nil {
		c.persist.Start(ctx)
	}
	ch, unsubscribe := c.eventBus.Subscribe(event.SubscribeOptions{BufferSize: 256, Reliable: true})
	safego.Go(ctx, "event-bus-consumer", func() {
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-ch:
				if !ok {
					return
				}
				c.handleEnvelope(ctx, env)
			}
		}
	})
}

func (c *EventBusConsumer) handleEnvelope(ctx context.Context, env event.Envelope) {
	c.buffer.Handle(env)
	if c.persist != nil {
		c.persist.Handle(ctx, env)
	}

	de := envelopeToDomainEvent(env)
	c.handleDomainEvent(ctx, *de)
}

func (c *EventBusConsumer) handleDomainEvent(ctx context.Context, de DomainEvent) {
	switch de.Type {
	case DomainEventRunnerCompletion:
		c.runner.Handle(ctx, de)
	case DomainEventStateDelta:
		c.state.Handle(ctx, de)
	}
}
