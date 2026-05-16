package biz

import (
	"context"
	"log/slog"
	"time"

	"aranea-agents/internal/event"
)

type EventBusConsumer struct {
	eventBus    event.Bus
	eventBuffer *event.Buffer
	sessions    *SessionUsecase
	usage       *UsageUsecase
}

func NewEventBusConsumer(
	eventBus event.Bus,
	eventBuffer *event.Buffer,
	sessions *SessionUsecase,
	usage *UsageUsecase,
) *EventBusConsumer {
	return &EventBusConsumer{
		eventBus:    eventBus,
		eventBuffer: eventBuffer,
		sessions:    sessions,
		usage:       usage,
	}
}

func (c *EventBusConsumer) Start(ctx context.Context) {
	ch, unsubscribe := c.eventBus.Subscribe(event.SubscribeOptions{BufferSize: 256})
	go func() {
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
	}()
}

func (c *EventBusConsumer) handleEnvelope(ctx context.Context, env event.Envelope) {
	c.eventBuffer.Append(env)

	de := envelopeToDomainEvent(env)
	c.handleDomainEvent(ctx, *de)
}

func (c *EventBusConsumer) handleDomainEvent(ctx context.Context, de DomainEvent) {
	switch de.Type {
	case DomainEventRunnerCompletion:
		c.handleRunnerCompletion(ctx, de)
	case DomainEventStateDelta:
		c.handleStateDelta(ctx, de)
	}
}

func (c *EventBusConsumer) handleRunnerCompletion(ctx context.Context, de DomainEvent) {
	if de.Usage == nil {
		return
	}
	if c.usage != nil {
		now := time.Now().UTC()
		_, err := c.usage.RecordTokenUsageEvent(ctx, TokenUsageEvent{
			SessionID:     de.SessionID,
			AgentID:       de.Author,
			InputTokens:   de.Usage.PromptTokens,
			OutputTokens:  de.Usage.CompletionTokens,
			TotalTokens:   de.Usage.TotalTokens,
			OccurredAt:    now.Format(time.RFC3339),
			DateKey:       now.Format("2006-01-02"),
			HourKey:       now.Format("2006-01-02T15"),
			Status:        "ok",
			StreamEnabled: true,
		})
		if err != nil {
			slog.Warn("event_bus_consumer: usage record failed", "error", err, "session_id", de.SessionID)
		}
	}
}

func (c *EventBusConsumer) handleStateDelta(ctx context.Context, de DomainEvent) {
	if de.StateDelta == nil || c.sessions == nil {
		return
	}
	if de.StateDelta.Path == "__state__" {
		err := c.sessions.UpdateRunnerSnapshotJSON(ctx, de.SessionID, de.StateDelta.ValueJSON)
		if err != nil {
			slog.Warn("event_bus_consumer: state delta persist failed", "error", err, "session_id", de.SessionID)
		}
		return
	}
	err := c.sessions.ApplyStateDelta(ctx, de.SessionID, DomainStateDelta{
		Operation: de.StateDelta.Operation,
		Path:      de.StateDelta.Path,
		ValueJSON: de.StateDelta.ValueJSON,
	})
	if err != nil {
		slog.Warn("event_bus_consumer: state delta apply failed", "error", err, "session_id", de.SessionID, "path", de.StateDelta.Path)
	}
}
