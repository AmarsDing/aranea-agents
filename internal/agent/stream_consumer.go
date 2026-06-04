package agent

import (
	"context"
	"strings"

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
	return c
}

func (c *turnStreamConsumer) consume(events <-chan *trpcevent.Event) EventStreamResult {
	evIdx := 0
	for ev := range events {
		evIdx++
		if c.turnCtx.Err() != nil {
			c.lg.With(loggateway.StepID("stream.consume_exit")).Info("stream consume: turnCtx canceled",
				loggateway.Any("ev_count", evIdx))
			return c.result
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
			c.lg.With(loggateway.StepID("stream.first_byte_timeout")).Info("stream consume: firstByte timeout",
				loggateway.Any("ev_count", evIdx))
			return c.result
		}
		if !c.handleEvent(ev) {
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

	var onDelta func(string) error
	if c.opts != nil {
		onDelta = c.opts.OnReplyDelta
	}
	partial := ev.Response.IsPartial
	for _, choice := range ev.Response.Choices {
		if err := accumulateChoiceStream(&c.result, choice, partial, onDelta); err != nil {
			c.result.HasError = true
			c.result.LastError = err.Error()
			return false
		}
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

func (c *turnStreamConsumer) finalize() {
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

func accumulateChoiceStream(result *EventStreamResult, choice trpcmodel.Choice, partial bool, onReplyDelta func(string) error) error {
	if result == nil {
		return nil
	}
	text, reasoning := ChoiceStreamContent(choice, partial)
	if text != "" {
		_ = provider.VisibleStreamingDelta(&result.Reply, text)
		result.HasContent = true
		if onReplyDelta != nil {
			if err := onReplyDelta(result.Reply.String()); err != nil {
				return err
			}
		}
	}
	if reasoning != "" {
		_ = provider.VisibleStreamingDelta(&result.Reasoning, reasoning)
		result.HasContent = true
	}
	return nil
}

func isChatCompletionStreamObject(objType string) bool {
	switch objType {
	case "", trpcmodel.ObjectTypeChatCompletionChunk, trpcmodel.ObjectTypeChatCompletion:
		return true
	default:
		return false
	}
}
