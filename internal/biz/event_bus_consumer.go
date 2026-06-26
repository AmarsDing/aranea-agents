package biz

import (
	"context"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

type EventBusConsumer struct {
	eventBus contract.Bus
	buffer   *eventBufferHandler
	runner   *runnerCompletionHandler
	state    *stateDeltaHandler
	dedup    *event.EventDeduplicator // T5.2: dedup Critical events by event_id
	logger   SessionLogWriter
}

func NewEventBusConsumer(
	eventBus contract.Bus,
	activityBus ActivityEventBus,
	eventBuffer EnvelopeBuffer,
	sessions *SessionUsecase,
	runnerSync RunnerSnapshotSync,
	usage *UsageUsecase,
	monitorUC *MonitorUsecase,
	memWorker *TurnMemoryWorker,
	traceProj *monitor.TraceProjector,
	logger SessionLogWriter,
) *EventBusConsumer {
	return &EventBusConsumer{
		eventBus: eventBus,
		buffer:   newEventBufferHandler(eventBuffer),
		runner:   newRunnerCompletionHandler(sessions, usage, monitorUC, memWorker, traceProj, activityBus, logger),
		state:    newStateDeltaHandler(sessions, runnerSync, logger),
		dedup:    event.NewEventDeduplicator(event.DefaultDedupCapacity),
		logger:   logger,
	}
}

func (c *EventBusConsumer) Start(ctx context.Context) {
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
	// T5.2: Deduplicate Critical events by event_id. Historically this guarded
	// against WAL recovery replays; with event_store/WAL removed (Phase 1c-2)
	// there is no replay path, but the dedup is retained as a defensive
	// measure against accidental double-publish by upstream producers.
	if contract.IsCriticalWBPFType(env.Type) && c.dedup.IsDuplicate(env.ID) {
		return
	}
	c.buffer.Handle(env)

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
