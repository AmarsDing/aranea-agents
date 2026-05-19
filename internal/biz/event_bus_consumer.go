package biz

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

type EventBusConsumer struct {
	eventBus    event.Bus
	eventBuffer *event.Buffer
	sessions    *SessionUsecase
	usage       *UsageUsecase
	monitor     *MonitorUsecase
	memWorker   *TurnMemoryWorker
}

func NewEventBusConsumer(
	eventBus event.Bus,
	eventBuffer *event.Buffer,
	sessions *SessionUsecase,
	usage *UsageUsecase,
	monitor *MonitorUsecase,
	memWorker *TurnMemoryWorker,
) *EventBusConsumer {
	return &EventBusConsumer{
		eventBus:    eventBus,
		eventBuffer: eventBuffer,
		sessions:    sessions,
		usage:       usage,
		monitor:     monitor,
		memWorker:   memWorker,
	}
}

func (c *EventBusConsumer) Start(ctx context.Context) {
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
	if c.memWorker != nil {
		c.memWorker.OnRunnerCompletion(ctx, de)
	}
	if c.monitor != nil {
		status := "ok"
		if de.Error != nil {
			status = "error"
		}
		if err := c.monitor.RecordMonitorEvent(ctx, MonitorEventWrite{
			EventKey:     "runner.completion",
			Name:         "Runner completed",
			Description:  strings.TrimSpace(de.Author),
			Status:       status,
			MetadataJSON: monitorRunnerCompletionMeta(de),
		}); err != nil {
			slog.Warn("event_bus_consumer: monitor event failed", "error", err, "session_id", de.SessionID)
		}
	}
	if de.Usage == nil {
		return
	}
	if c.usage != nil {
		now := time.Now().UTC()
		status := "ok"
		if de.Error != nil {
			status = "error"
		}
		_, err := c.usage.RecordTokenUsageEvent(ctx, TokenUsageEvent{
			SessionID:     de.SessionID,
			AgentID:       de.Author,
			InputTokens:   de.Usage.PromptTokens,
			OutputTokens:  de.Usage.CompletionTokens,
			TotalTokens:   de.Usage.TotalTokens,
			OccurredAt:    now.Format(time.RFC3339),
			DateKey:       now.Format("2006-01-02"),
			HourKey:       now.Format("2006-01-02T15"),
			Status:        status,
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
