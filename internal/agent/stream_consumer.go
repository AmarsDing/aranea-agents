package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// turnStreamConsumer projects trpc events to envelopes, aggregates reply text, and tracks tool state.
type turnStreamConsumer struct {
	firstByteCtx      context.Context
	turnCtx           context.Context
	eventBus          event.Bus
	projectMeta       ProjectMeta
	opts              *StreamConsumeOptions
	projector         *EventProjector
	result            EventStreamResult
	pendingToolCalls  map[string]event.EnvelopeToolCall
	firstByteReceived *bool
	received          bool
}

func newTurnStreamConsumer(
	firstByteCtx, turnCtx context.Context,
	eventBus event.Bus,
	projectMeta ProjectMeta,
	firstByteReceived *bool,
	opts *StreamConsumeOptions,
) *turnStreamConsumer {
	c := &turnStreamConsumer{
		firstByteCtx:      firstByteCtx,
		turnCtx:           turnCtx,
		eventBus:          eventBus,
		projectMeta:       projectMeta,
		opts:              opts,
		firstByteReceived: firstByteReceived,
		pendingToolCalls:  make(map[string]event.EnvelopeToolCall),
	}
	if eventBus != nil {
		c.projector = NewEventProjector(eventBus)
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
	for ev := range events {
		if c.turnCtx.Err() != nil {
			return c.result
		}
		if ev == nil {
			continue
		}
		c.markFirstByte(ev)
		if c.firstByteCtx.Err() != nil && !c.received {
			return c.result
		}
		if !c.handleEvent(ev) {
			return c.result
		}
	}
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
		accumulateStreamUsage(&c.result, ev, c.projectMeta, usage.PromptTokens, usage.CompletionTokens)
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
	envelopes := c.projector.Project(c.turnCtx, ev, c.projectMeta)
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
		c.eventBus.Publish(c.turnCtx, env)
	}
	if c.opts != nil {
		PublishActivityEnvelopes(c.turnCtx, c.projectMeta, c.opts.ActivityPersister, envelopes)
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
	if c.opts == nil || c.opts.ActivityPersister == nil || len(c.pendingToolCalls) == 0 {
		return
	}
	FinalizeStuckToolActivities(c.turnCtx, c.projectMeta, c.opts.ActivityPersister, c.pendingToolCalls)
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
