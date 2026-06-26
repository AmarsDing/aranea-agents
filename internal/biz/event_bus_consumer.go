package biz

import (
	"context"
	"strings"
	"time"

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

// --- Legacy Envelope → DomainEvent conversion ---
//
// These functions are retained for EventBusConsumer which still subscribes to
// the legacy contract.Bus (Envelope bus). They will be removed once
// EventBusConsumer is migrated to ActivityEventBus (ADR-03 Blocker D/E/F).
// The forward direction (DomainEvent → Envelope) has been fully removed;
// the DomainEvent bridge now publishes ActivityEvents (Domain=system).

func copyContentFromEnvelope(src *contract.EnvelopeContent, dst *DomainContent) {
	dst.Text = src.Text
	dst.Reasoning = src.Reasoning
	dst.IsPartial = src.IsPartial
}

func copyStateDeltaFromEnvelope(src *contract.EnvelopeStateDelta, dst *DomainStateDelta) {
	dst.Operation = src.Operation
	dst.Path = src.Path
	dst.ValueJSON = src.ValueJSON
}

func copyErrorFromEnvelope(src *contract.EnvelopeError, dst *DomainError) {
	dst.Type = src.Type
	dst.Message = src.Message
}

func copyUsageFromEnvelope(src *contract.EnvelopeUsage, dst *DomainUsage) {
	dst.PromptTokens = src.PromptTokens
	dst.CompletionTokens = src.CompletionTokens
	dst.TotalTokens = src.TotalTokens
}

func copyToolCallFromEnvelope(src *contract.EnvelopeToolCall, dst *DomainToolCall) {
	dst.ID = src.ID
	dst.Name = src.Name
	dst.ArgumentsJSON = src.ArgumentsJSON
	dst.ResultJSON = src.ResultJSON
	dst.Status = src.Status
	dst.DurationMS = src.DurationMS
}

func envelopeToDomainEvent(env contract.Envelope) *DomainEvent {
	de := &DomainEvent{
		ID:        env.ID,
		Type:      DomainEventType(env.Type),
		Author:    env.Author,
		SessionID: env.SessionID,
		TeamID:    env.TeamID,
	}
	if env.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, env.Timestamp); err == nil {
			de.Timestamp = t
		} else {
			de.Timestamp = time.Now()
		}
	} else {
		de.Timestamp = time.Now()
	}
	if env.Content != nil {
		de.Content = &DomainContent{}
		copyContentFromEnvelope(env.Content, de.Content)
	}
	if env.StateDelta != nil {
		de.StateDelta = &DomainStateDelta{}
		copyStateDeltaFromEnvelope(env.StateDelta, de.StateDelta)
	}
	if env.Error != nil {
		de.Error = &DomainError{}
		copyErrorFromEnvelope(env.Error, de.Error)
	}
	if env.Usage != nil {
		de.Usage = &DomainUsage{}
		copyUsageFromEnvelope(env.Usage, de.Usage)
	}
	if env.ToolCall != nil {
		de.ToolCall = &DomainToolCall{}
		copyToolCallFromEnvelope(env.ToolCall, de.ToolCall)
	}
	applyEnvelopeCorrelation(de, env)
	return de
}

func applyEnvelopeCorrelation(de *DomainEvent, env contract.Envelope) {
	de.RequestID = strings.TrimSpace(env.RequestID)
	de.InvocationID = strings.TrimSpace(env.InvocationID)
	if env.Metadata != nil {
		if v := metaString(env.Metadata, "run_id"); v != "" {
			de.RunID = v
		}
		if v := metaString(env.Metadata, "trace_id"); v != "" {
			de.TraceID = v
		}
		if v := metaString(env.Metadata, "agent_id"); v != "" {
			de.AgentID = v
		}
		if v := metaString(env.Metadata, "agent_display_name"); v != "" {
			de.AgentDisplayName = v
		}
		if v := metaString(env.Metadata, "run_kind"); v != "" {
			de.RunKind = v
		}
		if v := metaString(env.Metadata, "usage_event_id"); v != "" {
			de.UsageEventID = v
		}
	}
	if de.RunID == "" {
		de.RunID = de.InvocationID
	}
}
