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

func (c *EventBusConsumer) handleEnvelope(ctx context.Context, env Envelope) {
	c.eventBuffer.Append(env)

	switch env.Type {
	case EnvelopeTypeRunnerCompletion:
		c.handleRunnerCompletion(ctx, env)
	case EnvelopeTypeStateDelta:
		c.handleStateDelta(ctx, env)
	}
}

func (c *EventBusConsumer) handleRunnerCompletion(ctx context.Context, env Envelope) {
	if env.Usage == nil {
		return
	}
	if c.usage != nil {
		now := time.Now().UTC()
		_, err := c.usage.RecordTokenUsageEvent(ctx, TokenUsageEvent{
			SessionID:     env.SessionID,
			AgentID:       env.Author,
			InputTokens:   env.Usage.PromptTokens,
			OutputTokens:  env.Usage.CompletionTokens,
			TotalTokens:   env.Usage.TotalTokens,
			OccurredAt:    now.Format(time.RFC3339),
			DateKey:       now.Format("2006-01-02"),
			HourKey:       now.Format("2006-01-02T15"),
			Status:        "ok",
			StreamEnabled: true,
		})
		if err != nil {
			slog.Warn("event_bus_consumer: usage record failed", "error", err, "session_id", env.SessionID)
		}
	}
}

func (c *EventBusConsumer) handleStateDelta(ctx context.Context, env Envelope) {
	if env.StateDelta == nil || c.sessions == nil {
		return
	}
	if env.StateDelta.Path == "__state__" {
		err := c.sessions.UpdateRunnerSnapshotJSON(ctx, env.SessionID, env.StateDelta.ValueJSON)
		if err != nil {
			slog.Warn("event_bus_consumer: state delta persist failed", "error", err, "session_id", env.SessionID)
		}
		return
	}
	err := c.sessions.ApplyStateDelta(ctx, env.SessionID, *env.StateDelta)
	if err != nil {
		slog.Warn("event_bus_consumer: state delta apply failed", "error", err, "session_id", env.SessionID, "path", env.StateDelta.Path)
	}
}
