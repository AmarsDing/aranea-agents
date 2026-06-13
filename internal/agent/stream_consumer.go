package agent

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// turnStreamConsumer projects trpc events to envelopes, aggregates reply text, and tracks tool state.
type turnStreamConsumer struct {
	firstByteCtx      context.Context
	turnCtx           context.Context
	eventBus          event.Bus
	observer          *event.TurnObserver
	projectMeta       ProjectMeta
	opts              *StreamConsumeOptions
	projector         *EventProjector
	result            EventStreamResult
	pendingToolCalls  map[string]event.EnvelopeToolCall
	firstByteReceived *bool
	received          bool
	lg                loggateway.Logger
	consumeStart      time.Time
}

func newTurnStreamConsumer(
	firstByteCtx, turnCtx context.Context,
	eventBus event.Bus,
	projectMeta ProjectMeta,
	firstByteReceived *bool,
	opts *StreamConsumeOptions,
	lg loggateway.Logger,
) *turnStreamConsumer {
	c := &turnStreamConsumer{
		firstByteCtx:      firstByteCtx,
		turnCtx:           turnCtx,
		eventBus:          eventBus,
		projectMeta:       projectMeta,
		opts:              opts,
		firstByteReceived: firstByteReceived,
		pendingToolCalls:  make(map[string]event.EnvelopeToolCall),
		lg:                lg,
	}
	if eventBus != nil {
		c.projector = NewEventProjector(eventBus, lg)
		c.observer = event.NewTurnObserver(eventBus)
		if projectMeta.TeamID != "" && len(projectMeta.MemberAgentKeys) > 0 {
			c.projector.memberStarted = make(map[string]bool)
		}
		if opts != nil {
			c.projector.Configure(projectMeta, opts.MetaResolver)
		}
	}
	// AF phase: configure ActivityProjector for dual-emission
	if opts != nil && opts.ActivityProjector != nil {
		opts.ActivityProjector.Configure(projectMeta, opts.MetaResolver)
		opts.ActivityProjector.Reset()
		opts.ActivityProjector.OnTurnStart(turnCtx, projectMeta)
	}
	return c
}

func (c *turnStreamConsumer) consume(events <-chan *trpcevent.Event) EventStreamResult {
	c.consumeStart = time.Now()
	evIdx := 0
	canceled := false
	for ev := range events {
		evIdx++
		if !canceled && c.turnCtx.Err() != nil {
			canceled = true
			c.lg.With(loggateway.StepID("stream.consume_exit")).Info("stream consume: turnCtx canceled, draining critical events",
				loggateway.Any("ev_count", evIdx))
			// Do NOT return immediately — continue draining to ensure
			// Critical events (ToolResult, RunnerCompletion, StateDelta)
			// are projected and published before exiting.
		}
		if ev == nil {
			continue
		}
		// Log each event for debugging
		evType := "unknown"
		if ev.IsRunnerCompletion() {
			evType = "runner_completion"
		} else if ev.Response != nil && ev.Response.Error != nil {
			evType = "response_error"
		} else if ev.Response != nil {
			evType = "response"
			if len(ev.Response.Choices) > 0 {
				ch := ev.Response.Choices[0]
				if len(ch.Message.ToolCalls) > 0 {
					evType = "tool_call"
				} else if ch.Message.Content != "" {
					evType = "text_delta"
				}
			}
		}
		c.lg.With(loggateway.StepID("stream.event")).Info("stream event",
			loggateway.Any("idx", evIdx),
			loggateway.Any("type", evType),
			loggateway.Any("author", ev.Author))
		c.markFirstByte(ev)
		if c.firstByteCtx.Err() != nil && !c.received {
			c.lg.With(loggateway.StepID("stream.first_byte_timeout")).Info("stream consume: firstByte timeout, draining critical events",
				loggateway.Any("ev_count", evIdx))
			// Do NOT return immediately — drain critical events (ToolResult, RunnerCompletion,
			// StateDelta) just like the turnCtx cancel path, to prevent resource leaks and
			// ensure the frontend receives terminal signals.
			canceled = true
		}
		if !c.handleEvent(ev) {
			return c.result
		}
		// After first-byte timeout or context cancellation, stop once we see
		// RunnerCompletion (the terminal event) to avoid draining indefinitely.
		if canceled && ev.IsRunnerCompletion() {
			c.lg.With(loggateway.StepID("stream.consume_exit")).Info("stream consume: drained to RunnerCompletion",
				loggateway.Any("ev_count", evIdx),
				loggateway.Any("reason", func() string {
					if c.turnCtx.Err() != nil {
						return "turnCtx_canceled"
					}
					return "firstByte_timeout"
				}()))
			c.finalize()
			return c.result
		}
	}
	c.lg.With(loggateway.StepID("stream.consume_done")).Info("stream consume: channel closed",
		loggateway.Any("ev_count", evIdx))
	c.finalize()
	return c.result
}

func (c *turnStreamConsumer) markFirstByte(ev *trpcevent.Event) {
	if c.received || ev == nil {
		return
	}
	if !countsAsFirstByte(ev) {
		return
	}
	c.received = true
	ttft := time.Since(c.consumeStart)
	evType := "unknown"
	if ev.IsRunnerCompletion() {
		evType = "runner_completion"
	} else if ev.Response != nil && ev.Response.Error != nil {
		evType = "response_error"
	} else if ev.Response != nil {
		evType = "response"
		if len(ev.Response.Choices) > 0 {
			ch := ev.Response.Choices[0]
			if len(ch.Message.ToolCalls) > 0 || len(ch.Delta.ToolCalls) > 0 {
				evType = "tool_call"
			} else if ch.Message.Content != "" {
				evType = "text_delta"
			}
		}
	}
	c.lg.With(loggateway.StepID("stream.first_byte")).Info("stream first byte received (TTFT)",
		loggateway.Duration(ttft.Milliseconds()),
		loggateway.Any("ttft_ms", ttft.Milliseconds()),
		loggateway.Any("first_byte_type", evType),
		loggateway.Any("author", ev.Author))
	if c.firstByteReceived != nil {
		*c.firstByteReceived = true
	}
}

func countsAsFirstByte(ev *trpcevent.Event) bool {
	if ev.IsRunnerCompletion() {
		return true
	}
	if ev.Response != nil && ev.Response.Error != nil {
		return true
	}
	if ev.Response == nil {
		return false
	}
	if !isChatCompletionStreamObject(ev.Response.Object) {
		return false
	}
	for _, choice := range ev.Response.Choices {
		if ChoiceHasStreamContent(choice, ev.Response.IsPartial) {
			return true
		}
		if len(choice.Message.ToolCalls) > 0 || len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func (c *turnStreamConsumer) handleEvent(ev *trpcevent.Event) bool {
	if ev.Response != nil && ev.Response.Error != nil {
		c.result.HasError = true
		c.result.LastError = ev.Response.Error.Message
	}

	c.projectAndTrackTools(ev)

	if ev.IsRunnerCompletion() {
		if ev.Response != nil && ev.Response.Usage != nil {
			c.result.PromptTok = ev.Response.Usage.PromptTokens
			c.result.CompletionTok = ev.Response.Usage.CompletionTokens
		}
		return true
	}

	if ev.Response != nil && ev.Response.Error != nil {
		return true
	}
	if ev.Response == nil {
		return true
	}
	if !isChatCompletionStreamObject(ev.Response.Object) {
		return true
	}
	if usage := ev.Response.Usage; usage != nil {
		prevPrompt := c.result.PromptTok
		accumulateStreamUsage(&c.result, ev, c.projectMeta, usage.PromptTokens, usage.CompletionTokens)
		if c.result.PromptTok > prevPrompt {
			c.publishContextUsageStep()
		}
	}

	partial := ev.Response.IsPartial
	for _, choice := range ev.Response.Choices {
		accumulateChoiceStream(&c.result, choice, partial)
	}
	return true
}

func (c *turnStreamConsumer) projectAndTrackTools(ev *trpcevent.Event) {
	if c.projector == nil {
		return
	}
	meta := c.projectMeta
	if ev.IsRunnerCompletion() {
		meta.TurnPromptTokens = c.result.PromptTok
		meta.TurnCompletionTok = c.result.CompletionTok
	}
	envelopes := c.projector.Project(c.turnCtx, ev, meta)
	for _, env := range envelopes {
		c.trackToolEnvelope(env)
		if c.projectMeta.TeamID != "" && env.Type == event.EnvelopeTypeToolCall {
			author := strings.TrimSpace(env.Author)
			if author != "" && isTeamMemberAuthor(author, c.projectMeta) {
				if c.result.MemberToolCalls == nil {
					c.result.MemberToolCalls = make(map[string]int)
				}
				c.result.MemberToolCalls[author]++
			}
		}
		if c.observer != nil {
			c.observer.PublishChat(c.turnCtx, env)
		} else if c.eventBus != nil {
			c.eventBus.Publish(c.turnCtx, env)
		}
	}
	if c.opts != nil {
		PublishActivityEnvelopes(c.turnCtx, c.projectMeta, c.opts.ActivityPersister, envelopes, c.lg)
	}

	// AF phase: dual-emit Activity events alongside existing envelopes
	c.projectActivityEvents(ev, envelopes)
}

func (c *turnStreamConsumer) publishContextUsageStep() {
	if c.projectMeta.ContextWindow <= 0 || c.result.PromptTok <= 0 {
		return
	}
	if c.eventBus == nil && c.observer == nil {
		return
	}
	turnTotal := c.result.PromptTok + c.result.CompletionTok
	env := event.NewEnvelope(event.EnvelopeTypeContextUsage, strings.TrimSpace(c.projectMeta.AgentDisplayName), c.projectMeta.SessionID)
	if env.Author == "" {
		env.Author = "agent"
	}
	env.RequestID = c.projectMeta.RequestID
	env.InvocationID = c.projectMeta.InvocationID
	env.TeamID = c.projectMeta.TeamID
	env.Usage = &event.EnvelopeUsage{
		ContextPromptTokens: c.result.PromptTok,
		PromptTokens:        c.result.PromptTok,
		CompletionTokens:    c.result.CompletionTok,
		TotalTokens:         turnTotal,
		TurnTotalTokens:     turnTotal,
		MaxTokens:           c.projectMeta.ContextWindow,
	}
	if c.observer != nil {
		c.observer.PublishChat(c.turnCtx, env)
	} else if c.eventBus != nil {
		c.eventBus.Publish(c.turnCtx, env)
	}
}

func (c *turnStreamConsumer) trackToolEnvelope(env event.Envelope) {
	if env.ToolCall == nil {
		return
	}
	id := strings.TrimSpace(env.ToolCall.ID)
	if id == "" {
		id = strings.TrimSpace(env.ToolCall.Name)
	}
	switch env.Type {
	case event.EnvelopeTypeToolCall:
		if id != "" {
			c.pendingToolCalls[id] = *env.ToolCall
		}
	case event.EnvelopeTypeToolResult:
		delete(c.pendingToolCalls, id)
	}
}

// projectActivityEvents maps projected envelopes to ActivityProjector callbacks.
// This is the dual-emission bridge: existing envelopes are translated into Activity semantic events.
func (c *turnStreamConsumer) projectActivityEvents(ev *trpcevent.Event, envelopes []event.Envelope) {
	if c.opts == nil || c.opts.ActivityProjector == nil {
		return
	}
	ap := c.opts.ActivityProjector

	for _, env := range envelopes {
		switch env.Type {
		case event.EnvelopeTypeToolCall:
			if env.ToolCall != nil {
				startedAt := time.Now().UTC()
				if t, err := time.Parse(time.RFC3339Nano, env.ToolCall.StartedAt); err == nil {
					startedAt = t
				}
				ap.OnToolCall(c.turnCtx, env.ToolCall.ID, env.ToolCall.Name, env.ToolCall.ArgumentsJSON, env.Author, startedAt)
			}
		case event.EnvelopeTypeToolResult:
			if env.ToolCall != nil {
				ap.OnToolResult(c.turnCtx, env.ToolCall.ID, env.ToolCall.ResultJSON, env.ToolCall.Status, env.ToolCall.ErrorCode, env.ToolCall.DurationMS)
			}
		case event.EnvelopeTypeTextDelta:
			if env.Content != nil && env.Content.IsPartial {
				if env.Content.Reasoning != "" {
					ap.OnReasoningDelta(c.turnCtx, env.Author, env.Content.Reasoning, true)
				}
				if env.Content.Text != "" {
					ap.OnTextDelta(c.turnCtx, env.Author, env.Content.Text)
				}
			}
		case event.EnvelopeTypeTextDone:
			if env.Content != nil && !env.Content.IsPartial {
				if env.Content.Reasoning != "" {
					// Check reasoning_as_display from extensions
					reasoningAsDisplay := false
					if env.Extensions != nil {
						if v, ok := env.Extensions["reasoning_as_display"]; ok && v == "true" {
							reasoningAsDisplay = true
						}
					}
					ap.OnReasoningDone(c.turnCtx, env.Author, env.Content.Reasoning, reasoningAsDisplay)
				}
				if env.Content.Text != "" {
					ap.OnTextDone(c.turnCtx, env.Author, env.Content.Text)
				}
			}
		case event.EnvelopeTypeError:
			if env.Error != nil {
				ap.OnError(c.turnCtx, env.Error.Message)
			}
		}
	}
}

func (c *turnStreamConsumer) finalize() {
	// AF phase: finalize root task Activity
	if c.opts != nil && c.opts.ActivityProjector != nil {
		c.opts.ActivityProjector.OnTurnEnd(c.turnCtx)
	}
	if len(c.pendingToolCalls) == 0 {
		return
	}
	pending := c.pendingToolCalls
	if c.eventBus != nil {
		PublishStuckToolResultEnvelopes(c.turnCtx, c.projectMeta, c.eventBus, pending)
	}
	if c.opts != nil && c.opts.ActivityPersister != nil {
		FinalizeStuckToolActivities(c.turnCtx, c.projectMeta, c.opts.ActivityPersister, pending, c.lg)
	}
}

func accumulateChoiceStream(result *EventStreamResult, choice trpcmodel.Choice, partial bool) {
	if result == nil {
		return
	}
	text, reasoning := ChoiceStreamContent(choice, partial)
	if text != "" {
		_ = provider.VisibleStreamingDelta(&result.Reply, text)
		result.HasContent = true
	}
	if reasoning != "" {
		_ = provider.VisibleStreamingDelta(&result.Reasoning, reasoning)
		result.HasContent = true
	}
}

func isChatCompletionStreamObject(objType string) bool {
	switch objType {
	case "", trpcmodel.ObjectTypeChatCompletionChunk, trpcmodel.ObjectTypeChatCompletion:
		return true
	default:
		return false
	}
}
