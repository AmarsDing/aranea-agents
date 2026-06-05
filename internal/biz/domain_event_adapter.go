package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/safego"
)

type eventBusAdapter struct {
	bus    contract.Bus
	sysLog SystemLogWriter
}

var _ DomainEventPublisher = (*eventBusAdapter)(nil)
var _ DomainEventSubscriber = (*eventBusAdapter)(nil)

func NewDomainEventBus(bus contract.Bus) DomainEventPublisher {
	if bus == nil {
		return nil
	}
	return &eventBusAdapter{bus: bus}
}

func (a *eventBusAdapter) SetSystemLog(sysLog SystemLogWriter) {
	a.sysLog = sysLog
}

func (a *eventBusAdapter) PublishDomainEvent(de DomainEvent) {
	env := domainEventToEnvelope(de)
	a.bus.Publish(context.Background(), env)
}

func (a *eventBusAdapter) SubscribeDomainEvents() (<-chan DomainEvent, func()) {
	ch, unsub := a.bus.Subscribe(contract.SubscribeOptions{BufferSize: 256})
	out := make(chan DomainEvent, 256)
	done := make(chan struct{})
	safego.Go(appctx.Ctx(), "domain-event-adapter", func() {
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
						if a.sysLog != nil {
							a.sysLog.LogWarn("domain_event.adapter.drop", "DomainEvent 输出缓冲满，丢弃事件",
								LogPair{Key: "type", Value: string(de.Type)}, LogPair{Key: "session_id", Value: de.SessionID})
						}
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

func copyContentToEnvelope(src *DomainContent, dst *contract.EnvelopeContent) {
	dst.Text = src.Text
	dst.Reasoning = src.Reasoning
	dst.IsPartial = src.IsPartial
}

func copyContentFromEnvelope(src *contract.EnvelopeContent, dst *DomainContent) {
	dst.Text = src.Text
	dst.Reasoning = src.Reasoning
	dst.IsPartial = src.IsPartial
}

func copyStateDeltaToEnvelope(src *DomainStateDelta, dst *contract.EnvelopeStateDelta) {
	dst.Operation = src.Operation
	dst.Path = src.Path
	dst.ValueJSON = src.ValueJSON
}

func copyStateDeltaFromEnvelope(src *contract.EnvelopeStateDelta, dst *DomainStateDelta) {
	dst.Operation = src.Operation
	dst.Path = src.Path
	dst.ValueJSON = src.ValueJSON
}

func copyErrorToEnvelope(src *DomainError, dst *contract.EnvelopeError) {
	dst.Type = src.Type
	dst.Message = src.Message
}

func copyErrorFromEnvelope(src *contract.EnvelopeError, dst *DomainError) {
	dst.Type = src.Type
	dst.Message = src.Message
}

func copyUsageToEnvelope(src *DomainUsage, dst *contract.EnvelopeUsage) {
	dst.PromptTokens = src.PromptTokens
	dst.CompletionTokens = src.CompletionTokens
	dst.TotalTokens = src.TotalTokens
}

func copyUsageFromEnvelope(src *contract.EnvelopeUsage, dst *DomainUsage) {
	dst.PromptTokens = src.PromptTokens
	dst.CompletionTokens = src.CompletionTokens
	dst.TotalTokens = src.TotalTokens
}

func copyToolCallToEnvelope(src *DomainToolCall, dst *contract.EnvelopeToolCall) {
	dst.ID = src.ID
	dst.Name = src.Name
	dst.ArgumentsJSON = src.ArgumentsJSON
	dst.ResultJSON = src.ResultJSON
	dst.Status = src.Status
	dst.DurationMS = src.DurationMS
}

func copyToolCallFromEnvelope(src *contract.EnvelopeToolCall, dst *DomainToolCall) {
	dst.ID = src.ID
	dst.Name = src.Name
	dst.ArgumentsJSON = src.ArgumentsJSON
	dst.ResultJSON = src.ResultJSON
	dst.Status = src.Status
	dst.DurationMS = src.DurationMS
}

// --- top-level conversion ---

func domainEventToEnvelope(de DomainEvent) contract.Envelope {
	env := contract.NewEnvelope(contract.EnvelopeType(de.Type), de.Author, de.SessionID)
	env.TeamID = de.TeamID
	if de.Content != nil {
		env.Content = &contract.EnvelopeContent{}
		copyContentToEnvelope(de.Content, env.Content)
	}
	if de.StateDelta != nil {
		env.StateDelta = &contract.EnvelopeStateDelta{}
		copyStateDeltaToEnvelope(de.StateDelta, env.StateDelta)
	}
	if de.Error != nil {
		env.Error = &contract.EnvelopeError{}
		copyErrorToEnvelope(de.Error, env.Error)
	}
	if de.Usage != nil {
		env.Usage = &contract.EnvelopeUsage{}
		copyUsageToEnvelope(de.Usage, env.Usage)
	}
	if de.ToolCall != nil {
		env.ToolCall = &contract.EnvelopeToolCall{}
		copyToolCallToEnvelope(de.ToolCall, env.ToolCall)
	}
	return env
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

func metaBool(meta map[string]any, key string) bool {
	v, ok := meta[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true") || strings.TrimSpace(t) == "1"
	default:
		return false
	}
}
