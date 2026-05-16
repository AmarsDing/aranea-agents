package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/event"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type ProjectMeta struct {
	SessionID          string
	RequestID          string
	InvocationID       string
	ParentInvocationID string
	TeamID             string
	Branch             string
	FilterKey          string
}

type EventProjector struct {
	eventBus event.Bus
}

func NewEventProjector(eventBus event.Bus) *EventProjector {
	return &EventProjector{eventBus: eventBus}
}

func (p *EventProjector) ProjectAndPublish(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) {
	if ev == nil {
		return
	}
	envelopes := p.Project(ctx, ev, meta)
	for _, env := range envelopes {
		p.eventBus.Publish(ctx, env)
	}
}

func (p *EventProjector) Project(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) []event.Envelope {
	if ev == nil {
		return nil
	}

	if ev.IsRunnerCompletion() {
		return []event.Envelope{p.buildRunnerCompletionEnvelope(ev, meta)}
	}

	if ev.Response != nil && ev.Response.Error != nil {
		return []event.Envelope{p.buildErrorEnvelope(ev, meta)}
	}

	if len(ev.StateDelta) > 0 {
		return []event.Envelope{p.buildStateDeltaEnvelope(ev, meta)}
	}

	if ev.Response == nil {
		return nil
	}

	objType := ev.Response.Object
	switch objType {
	case trpcmodel.ObjectTypeChatCompletionChunk:
		return p.projectChatCompletionChunk(ev, meta)
	case trpcmodel.ObjectTypeChatCompletion:
		return p.projectChatCompletion(ev, meta)
	case trpcmodel.ObjectTypeToolResponse:
		return []event.Envelope{p.buildToolResultEnvelope(ev, meta)}
	case trpcmodel.ObjectTypeTransfer:
		return []event.Envelope{p.buildTransferEnvelope(ev, meta)}
	default:
		return nil
	}
}

func (p *EventProjector) baseEnvelope(ev *trpcevent.Event, meta ProjectMeta, typ event.EnvelopeType) event.Envelope {
	env := event.NewEnvelope(typ, ev.Author, meta.SessionID)
	if ev.ID != "" {
		env.ID = ev.ID
	}
	env.RequestID = meta.RequestID
	if ev.RequestID != "" {
		env.RequestID = ev.RequestID
	}
	env.InvocationID = ev.InvocationID
	if meta.InvocationID != "" {
		env.InvocationID = meta.InvocationID
	}
	env.ParentInvocationID = ev.ParentInvocationID
	if meta.ParentInvocationID != "" {
		env.ParentInvocationID = meta.ParentInvocationID
	}
	env.Branch = coalesceStr(ev.Branch, meta.Branch)
	env.FilterKey = coalesceStr(ev.FilterKey, meta.FilterKey)
	env.Tag = ev.Tag
	env.TeamID = meta.TeamID
	env.Version = ev.Version
	if !ev.Timestamp.IsZero() {
		env.Timestamp = ev.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	if len(ev.Extensions) > 0 {
		env.Extensions = make(map[string]string, len(ev.Extensions))
		for k, v := range ev.Extensions {
			env.Extensions[k] = string(v)
		}
	}
	if ev.Actions != nil {
		env.Actions = &event.EnvelopeActions{
			SkipSummarization: ev.Actions.SkipSummarization,
		}
	}
	return env
}

func (p *EventProjector) projectChatCompletionChunk(ev *trpcevent.Event, meta ProjectMeta) []event.Envelope {
	var envelopes []event.Envelope
	for _, choice := range ev.Response.Choices {
		msg := choice.Message
		delta := choice.Delta

		hasContent := strings.TrimSpace(msg.Content) != "" || strings.TrimSpace(delta.Content) != ""
		hasReasoning := strings.TrimSpace(msg.ReasoningContent) != "" || strings.TrimSpace(delta.ReasoningContent) != ""
		hasToolCalls := len(msg.ToolCalls) > 0 || len(delta.ToolCalls) > 0

		if hasToolCalls {
			allCalls := append(msg.ToolCalls, delta.ToolCalls...)
			for _, tc := range allCalls {
				env := p.baseEnvelope(ev, meta, event.EnvelopeTypeToolCall)
				env.ToolCall = &event.EnvelopeToolCall{
					ID:            tc.ID,
					Name:          tc.Function.Name,
					ArgumentsJSON: string(tc.Function.Arguments),
					Status:        "calling",
				}
				if _, ok := ev.LongRunningToolIDs[tc.ID]; ok {
					env.ToolCall.IsLongRunning = true
				}
				envelopes = append(envelopes, env)
			}
			continue
		}

		if hasContent || hasReasoning {
			text := coalesceStr(strings.TrimSpace(msg.Content), strings.TrimSpace(delta.Content))
			reasoning := coalesceStr(strings.TrimSpace(msg.ReasoningContent), strings.TrimSpace(delta.ReasoningContent))

			if ev.Response.IsPartial {
				env := p.baseEnvelope(ev, meta, event.EnvelopeTypeTextDelta)
				env.Content = &event.EnvelopeContent{
					Text:      text,
					Reasoning: reasoning,
					IsPartial: true,
				}
				envelopes = append(envelopes, env)
			} else {
				env := p.baseEnvelope(ev, meta, event.EnvelopeTypeTextDone)
				env.Content = &event.EnvelopeContent{
					Text:      text,
					Reasoning: reasoning,
					IsPartial: false,
				}
				envelopes = append(envelopes, env)
			}
		}
	}
	return envelopes
}

func (p *EventProjector) projectChatCompletion(ev *trpcevent.Event, meta ProjectMeta) []event.Envelope {
	var envelopes []event.Envelope
	for _, choice := range ev.Response.Choices {
		msg := choice.Message
		text := strings.TrimSpace(msg.Content)
		reasoning := strings.TrimSpace(msg.ReasoningContent)

		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				env := p.baseEnvelope(ev, meta, event.EnvelopeTypeToolCall)
				env.ToolCall = &event.EnvelopeToolCall{
					ID:            tc.ID,
					Name:          tc.Function.Name,
					ArgumentsJSON: string(tc.Function.Arguments),
					Status:        "calling",
				}
				envelopes = append(envelopes, env)
			}
		}

		if text != "" || reasoning != "" {
			env := p.baseEnvelope(ev, meta, event.EnvelopeTypeTextDone)
			env.Content = &event.EnvelopeContent{
				Text:      text,
				Reasoning: reasoning,
				IsPartial: false,
			}
			envelopes = append(envelopes, env)
		}
	}
	return envelopes
}

func (p *EventProjector) buildRunnerCompletionEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeRunnerCompletion)
	if ev.Response != nil && ev.Response.Usage != nil {
		env.Usage = &event.EnvelopeUsage{
			PromptTokens:     ev.Response.Usage.PromptTokens,
			CompletionTokens: ev.Response.Usage.CompletionTokens,
			TotalTokens:      ev.Response.Usage.TotalTokens,
		}
	}
	return env
}

func (p *EventProjector) buildErrorEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeError)
	if ev.Response != nil && ev.Response.Error != nil {
		errType := ev.Response.Error.Type
		if errType == "" {
			errType = "run_error"
		}
		env.Error = &event.EnvelopeError{
			Type:    errType,
			Message: ev.Response.Error.Message,
		}
	}
	return env
}

func (p *EventProjector) buildStateDeltaEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeStateDelta)
	if len(ev.StateDelta) > 0 {
		combined, _ := json.Marshal(ev.StateDelta)
		env.StateDelta = &event.EnvelopeStateDelta{
			Operation: "set",
			Path:      "__state__",
			ValueJSON: string(combined),
		}
	}
	return env
}

func (p *EventProjector) buildToolResultEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeToolResult)
	if ev.Response != nil && len(ev.Response.Choices) > 0 {
		msg := ev.Response.Choices[0].Message
		resultJSON, _ := json.Marshal(msg.Content)
		env.ToolCall = &event.EnvelopeToolCall{
			Status:     "success",
			ResultJSON: string(resultJSON),
		}
	}
	return env
}

func (p *EventProjector) buildTransferEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeTransfer)
	if ev.Response != nil && len(ev.Response.Choices) > 0 {
		msg := ev.Response.Choices[0].Message
		parts := strings.SplitN(msg.Content, "→", 2)
		from := strings.TrimSpace(parts[0])
		to := ""
		if len(parts) > 1 {
			to = strings.TrimSpace(parts[1])
		}
		env.Transfer = &event.EnvelopeTransfer{
			FromAgent: from,
			ToAgent:   to,
		}
	}
	if env.Transfer == nil {
		env.Transfer = &event.EnvelopeTransfer{
			FromAgent: ev.ParentInvocationID,
			ToAgent:   ev.Author,
		}
	}
	return env
}

func (p *EventProjector) BuildLogEnvelope(level, message, source, sessionID string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeLog, source, sessionID)
	env.Metadata = map[string]any{
		"level":  level,
		"source": source,
	}
	env.Content = &event.EnvelopeContent{
		Text:      message,
		IsPartial: false,
	}
	return env
}

func (p *EventProjector) BuildIntentPassEnvelope(payload map[string]any, sessionID, teamID string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeIntentPass, "system", sessionID)
	env.TeamID = teamID
	env.Metadata = payload
	return env
}

func (p *EventProjector) BuildMemberMessageStartEnvelope(author, sessionID, teamID, branch string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeMemberMessageStart, author, sessionID)
	env.TeamID = teamID
	env.Branch = branch
	return env
}

func (p *EventProjector) BuildMemberDeltaEnvelope(author, sessionID, teamID, text string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeMemberDelta, author, sessionID)
	env.TeamID = teamID
	env.Content = &event.EnvelopeContent{
		Text:      text,
		IsPartial: true,
	}
	return env
}

func (p *EventProjector) BuildMemberMessageDoneEnvelope(author, sessionID, teamID, text string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeMemberMessageDone, author, sessionID)
	env.TeamID = teamID
	env.Content = &event.EnvelopeContent{
		Text:      text,
		IsPartial: false,
	}
	return env
}

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func roughTokenEstimateFromText(text string) int {
	return len(text) / 4
}

func FormatMonitorMessage(phase, sessionID string, args ...any) string {
	var sb strings.Builder
	sb.WriteString(phase)
	fmt.Fprintf(&sb, " session_id=%s", sessionID)
	for i := 0; i+1 < len(args); i += 2 {
		fmt.Fprintf(&sb, " %v=%v", args[i], args[i+1])
	}
	return sb.String()
}
