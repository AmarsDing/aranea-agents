package biz

import (
	"context"
	"time"

	"aranea-agents/internal/event"
)

type eventBusAdapter struct {
	bus event.Bus
}

var _ DomainEventPublisher = (*eventBusAdapter)(nil)
var _ DomainEventSubscriber = (*eventBusAdapter)(nil)

func NewDomainEventBus(bus event.Bus) DomainEventPublisher {
	if bus == nil {
		return nil
	}
	return &eventBusAdapter{bus: bus}
}

func (a *eventBusAdapter) PublishDomainEvent(de DomainEvent) {
	env := domainEventToEnvelope(de)
	a.bus.Publish(context.Background(), env)
}

func (a *eventBusAdapter) SubscribeDomainEvents() (<-chan DomainEvent, func()) {
	ch, unsub := a.bus.Subscribe(event.SubscribeOptions{BufferSize: 256})
	out := make(chan DomainEvent, 256)
	done := make(chan struct{})
	go func() {
		defer close(out)
		for {
			select {
			case <-done:
				return
			case env, ok := <-ch:
				if !ok {
					return
				}
				de := envelopeToDomainEvent(env)
				if de != nil {
					select {
					case out <- *de:
					default:
					}
				}
			}
		}
	}()
	cancel := func() {
		unsub()
		close(done)
	}
	return out, cancel
}

func domainEventToEnvelope(de DomainEvent) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeType(de.Type), de.Author, de.SessionID)
	env.TeamID = de.TeamID
	if de.Content != nil {
		env.Content = &event.EnvelopeContent{
			Text:      de.Content.Text,
			Reasoning: de.Content.Reasoning,
			IsPartial: de.Content.IsPartial,
		}
	}
	if de.StateDelta != nil {
		env.StateDelta = &event.EnvelopeStateDelta{
			Operation: de.StateDelta.Operation,
			Path:      de.StateDelta.Path,
			ValueJSON: de.StateDelta.ValueJSON,
		}
	}
	if de.Error != nil {
		env.Error = &event.EnvelopeError{
			Type:    de.Error.Type,
			Message: de.Error.Message,
		}
	}
	if de.Usage != nil {
		env.Usage = &event.EnvelopeUsage{
			PromptTokens:     de.Usage.PromptTokens,
			CompletionTokens: de.Usage.CompletionTokens,
			TotalTokens:      de.Usage.TotalTokens,
		}
	}
	if de.ToolCall != nil {
		env.ToolCall = &event.EnvelopeToolCall{
			ID:            de.ToolCall.ID,
			Name:          de.ToolCall.Name,
			ArgumentsJSON: de.ToolCall.ArgumentsJSON,
			ResultJSON:    de.ToolCall.ResultJSON,
			Status:        de.ToolCall.Status,
			DurationMS:    de.ToolCall.DurationMS,
		}
	}
	return env
}

func envelopeToDomainEvent(env event.Envelope) *DomainEvent {
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
		de.Content = &DomainContent{
			Text:      env.Content.Text,
			Reasoning: env.Content.Reasoning,
			IsPartial: env.Content.IsPartial,
		}
	}
	if env.StateDelta != nil {
		de.StateDelta = &DomainStateDelta{
			Operation: env.StateDelta.Operation,
			Path:      env.StateDelta.Path,
			ValueJSON: env.StateDelta.ValueJSON,
		}
	}
	if env.Error != nil {
		de.Error = &DomainError{
			Type:    env.Error.Type,
			Message: env.Error.Message,
		}
	}
	if env.Usage != nil {
		de.Usage = &DomainUsage{
			PromptTokens:     env.Usage.PromptTokens,
			CompletionTokens: env.Usage.CompletionTokens,
			TotalTokens:      env.Usage.TotalTokens,
		}
	}
	if env.ToolCall != nil {
		de.ToolCall = &DomainToolCall{
			ID:            env.ToolCall.ID,
			Name:          env.ToolCall.Name,
			ArgumentsJSON: env.ToolCall.ArgumentsJSON,
			ResultJSON:    env.ToolCall.ResultJSON,
			Status:        env.ToolCall.Status,
			DurationMS:    env.ToolCall.DurationMS,
		}
	}
	return de
}
