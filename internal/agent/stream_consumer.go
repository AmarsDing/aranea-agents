package agent

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/internal/metrics"
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
		c.observer = event.NewTurnObserver(eventBus)
	}
	// Legacy path: EventProjector is only needed when ActivityProjector is not
	// active (tests or fallback mode). In AF mode ActivityProjector owns all
	// event projection, persistence, and WS publishing.
	if opts == nil || opts.ActivityProjector == nil {
		if eventBus != nil {
			c.projector = NewEventProjector(eventBus, lg)
			if projectMeta.TeamID != "" && len(projectMeta.MemberAgentKeys) > 0 {
				c.projector.memberStarted = make(map[string]bool)
			}
			if opts != nil {
				c.projector.Configure(projectMeta, opts.MetaResolver)
			}
		}
	}
	// AF phase: configure ActivityProjector for direct event consumption.
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
			c.finalize()
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
	metrics.ChatTTFT.WithLabelValues(c.projectMeta.AgentID, evType).Observe(ttft.Seconds())
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
	// AF-3: ActivityProjector directly consumes trpc events and owns all
	// projection, persistence, and WS publishing. In production it is always
	// non-nil (wired via Wire DI). Return immediately after feeding the event.
	hasAF := c.opts != nil && c.opts.ActivityProjector != nil
	if hasAF {
		c.opts.ActivityProjector.ProcessEvent(c.turnCtx, ev)
		return
	}

	// Legacy path (no ActivityProjector): EventProjector handles projection,
	// WS publishing, activity persistence, tool tracking, and member tool call
	// counting for tests and fallback mode.
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
	if c.opts != nil && c.opts.ActivityPersister != nil {
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
	hasAF := c.opts != nil && c.opts.ActivityProjector != nil

	// AF phase: finalize root task Activity with token usage and copy per-member
	// tool call counts to the result. ActivityProjector owns stuck-tool handling
	// and persistence via its sequencer.
	if hasAF {
		usage := &ActivityUsage{
			PromptTokens:     c.result.PromptTok,
			CompletionTokens: c.result.CompletionTok,
			TotalTokens:      c.result.PromptTok + c.result.CompletionTok,
		}
		c.opts.ActivityProjector.OnTurnEnd(c.turnCtx, usage)
		c.opts.ActivityProjector.OnStuckTools(c.turnCtx)
		c.opts.ActivityProjector.Close()

		if mtc := c.opts.ActivityProjector.MemberToolCalls(); len(mtc) > 0 {
			c.result.MemberToolCalls = mtc
		}
		return
	}

	// Legacy path (no ActivityProjector): emit tool_result envelopes and persist
	// stuck tool Activities for tests and fallback mode.
	if len(c.pendingToolCalls) == 0 {
		return
	}
	pending := c.pendingToolCalls
	for id, tc := range pending {
		c.lg.Warn("stuck tool detected at turn finalization",
			loggateway.StepID("stream.stuck_tool"),
			loggateway.Str("tool_call_id", id),
			loggateway.Str("tool_name", tc.Name),
			loggateway.Str("status", tc.Status),
			loggateway.Str("session_id", c.projectMeta.SessionID),
		)
	}
	if c.eventBus != nil {
		// AS-EVT-01: ToolResult is Critical — must go through Infra.Publish (WBPF).
		// publishStuckToolNotification uses infra.SessionBus for AlertNotify (Informational).
		var infra *event.Infra
		if c.opts != nil {
			infra = c.opts.EventInfra
		}
		if infra != nil {
			PublishStuckToolResultEnvelopes(c.turnCtx, c.projectMeta, infra, pending)
			publishStuckToolNotification(c.turnCtx, c.projectMeta, infra, pending)
		}
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
