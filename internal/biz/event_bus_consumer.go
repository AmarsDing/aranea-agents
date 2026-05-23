package biz

import (
	"context"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

type EventBusConsumer struct {
	eventBus contract.Bus
	buffer   *eventBufferHandler
	runner   *runnerCompletionHandler
	state    *stateDeltaHandler
	persist  *eventPersistHandler
}

func NewEventBusConsumer(
	eventBus contract.Bus,
	eventBuffer EnvelopeBuffer,
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

// SetLogger propagates SessionLogWriter to all sub-handlers that need it.
func (c *EventBusConsumer) SetLogger(logger SessionLogWriter) {
	if c == nil {
		return
	}
	if c.runner != nil {
		c.runner.SetLogger(logger)
	}
	if c.state != nil {
		c.state.SetLogger(logger)
	}
	if c.persist != nil {
		c.persist.SetLogger(logger)
	}
}

func (c *EventBusConsumer) Start(ctx context.Context) {
	if c.persist != nil {
		c.persist.Start(ctx)
	}
	ch, unsubscribe := c.eventBus.Subscribe(contract.SubscribeOptions{BufferSize: 256, Reliable: true})
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

func (c *EventBusConsumer) handleEnvelope(ctx context.Context, env contract.Envelope) {
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
