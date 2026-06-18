package biz

import (
	"context"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

type EventBusConsumer struct {
	eventBus         contract.Bus
	buffer           *eventBufferHandler
	runner           *runnerCompletionHandler
	state            *stateDeltaHandler
	persist          *eventPersistHandler
	crossProcessSink contract.CrossProcessStore // optional (P1-6): nil when Postgres not configured
	dedup            *event.EventDeduplicator   // T5.2: dedup Critical events by event_id
	logger           SessionLogWriter
}

func NewEventBusConsumer(
	eventBus contract.Bus,
	eventBuffer EnvelopeBuffer,
	sessions *SessionUsecase,
	runnerSync RunnerSnapshotSync,
	usage *UsageUsecase,
	monitorUC *MonitorUsecase,
	memWorker *TurnMemoryWorker,
	eventStore *EventStoreUsecase,
	traceProj *monitor.TraceProjector,
	logger SessionLogWriter,
) *EventBusConsumer {
	return &EventBusConsumer{
		eventBus: eventBus,
		buffer:   newEventBufferHandler(eventBuffer),
		runner:   newRunnerCompletionHandler(sessions, usage, monitorUC, memWorker, traceProj, eventBus, logger),
		state:    newStateDeltaHandler(sessions, runnerSync, logger),
		persist:  newEventPersistHandler(eventStore, logger),
		dedup:    event.NewEventDeduplicator(event.DefaultDedupCapacity),
		logger:   logger,
	}
}

// WithCrossProcessSink sets an optional cross-process event sink (P1-6).
// When set, every envelope is dual-written to this sink in addition to the
// in-process EventStore. The sink must be safe for concurrent use.
func (c *EventBusConsumer) WithCrossProcessSink(sink contract.CrossProcessStore) *EventBusConsumer {
	if c != nil && sink != nil {
		c.crossProcessSink = sink
	}
	return c
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
	// T5.2: Deduplicate Critical events by event_id to handle WAL recovery
	// replays (AS-EVT-01 post-publish failure scenario). When Recover replays
	// a Critical event that was already delivered before the crash (post-publish
	// failure — event reached subscribers but WAL mark failed), skip processing
	// to avoid duplicate side effects on domain handlers (runnerCompletion,
	// stateDelta, etc.). Only Critical events go through WBPF and may be
	// replayed; non-Critical events are never replayed by WAL.
	if contract.IsCriticalWBPFType(env.Type) && c.dedup.IsDuplicate(env.ID) {
		return
	}
	c.buffer.Handle(env)
	if c.persist != nil {
		c.persist.Handle(ctx, env)
	}
	// P1-6: best-effort dual-write to cross-process store (Postgres) for WS
	// reconnect replay across server instances. Failures are logged but do
	// not affect the in-process event flow.
	if c.crossProcessSink != nil && shouldPersistEnvelope(env) {
		if err := c.crossProcessSink.Save(ctx, &env); err != nil && c.logger != nil {
			c.logger.LogSessionWarn(ctx, env.SessionID, "event_store.cross_process",
				"跨进程事件持久化失败",
				LogPair{Key: "type", Value: string(env.Type)},
				LogPair{Key: "id", Value: env.ID},
				LogPair{Key: "error", Value: err})
		}
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
