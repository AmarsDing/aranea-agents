package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
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
	safego.Go(context.Background(), "domain-event-adapter", func() {
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
	})
	cancel := func() {
		unsub()
		close(done)
	}
	return out, cancel
}

// --- field-level conversion helpers (eliminate duplicated mapping between directions) ---

func copyContentToEnvelope(src *DomainContent, dst *event.EnvelopeContent) {
	dst.Text = src.Text
	dst.Reasoning = src.Reasoning
	dst.IsPartial = src.IsPartial
}

func copyContentFromEnvelope(src *event.EnvelopeContent, dst *DomainContent) {
	dst.Text = src.Text
	dst.Reasoning = src.Reasoning
	dst.IsPartial = src.IsPartial
}

func copyStateDeltaToEnvelope(src *DomainStateDelta, dst *event.EnvelopeStateDelta) {
	dst.Operation = src.Operation
	dst.Path = src.Path
	dst.ValueJSON = src.ValueJSON
}

func copyStateDeltaFromEnvelope(src *event.EnvelopeStateDelta, dst *DomainStateDelta) {
	dst.Operation = src.Operation
	dst.Path = src.Path
	dst.ValueJSON = src.ValueJSON
}

func copyErrorToEnvelope(src *DomainError, dst *event.EnvelopeError) {
	dst.Type = src.Type
	dst.Message = src.Message
}

func copyErrorFromEnvelope(src *event.EnvelopeError, dst *DomainError) {
	dst.Type = src.Type
	dst.Message = src.Message
}

func copyUsageToEnvelope(src *DomainUsage, dst *event.EnvelopeUsage) {
	dst.PromptTokens = src.PromptTokens
	dst.CompletionTokens = src.CompletionTokens
	dst.TotalTokens = src.TotalTokens
}

func copyUsageFromEnvelope(src *event.EnvelopeUsage, dst *DomainUsage) {
	dst.PromptTokens = src.PromptTokens
	dst.CompletionTokens = src.CompletionTokens
	dst.TotalTokens = src.TotalTokens
}

func copyToolCallToEnvelope(src *DomainToolCall, dst *event.EnvelopeToolCall) {
	dst.ID = src.ID
	dst.Name = src.Name
	dst.ArgumentsJSON = src.ArgumentsJSON
	dst.ResultJSON = src.ResultJSON
	dst.Status = src.Status
	dst.DurationMS = src.DurationMS
}

func copyToolCallFromEnvelope(src *event.EnvelopeToolCall, dst *DomainToolCall) {
	dst.ID = src.ID
	dst.Name = src.Name
	dst.ArgumentsJSON = src.ArgumentsJSON
	dst.ResultJSON = src.ResultJSON
	dst.Status = src.Status
	dst.DurationMS = src.DurationMS
}

// --- top-level conversion ---

func domainEventToEnvelope(de DomainEvent) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeType(de.Type), de.Author, de.SessionID)
	env.TeamID = de.TeamID
	if de.Content != nil {
		env.Content = &event.EnvelopeContent{}
		copyContentToEnvelope(de.Content, env.Content)
	}
	if de.StateDelta != nil {
		env.StateDelta = &event.EnvelopeStateDelta{}
		copyStateDeltaToEnvelope(de.StateDelta, env.StateDelta)
	}
	if de.Error != nil {
		env.Error = &event.EnvelopeError{}
		copyErrorToEnvelope(de.Error, env.Error)
	}
	if de.Usage != nil {
		env.Usage = &event.EnvelopeUsage{}
		copyUsageToEnvelope(de.Usage, env.Usage)
	}
	if de.ToolCall != nil {
		env.ToolCall = &event.EnvelopeToolCall{}
		copyToolCallToEnvelope(de.ToolCall, env.ToolCall)
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

func applyEnvelopeCorrelation(de *DomainEvent, env event.Envelope) {
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

func metaString(meta map[string]any, key string) string {
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}