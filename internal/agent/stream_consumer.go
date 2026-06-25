package agent

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
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
	projectMeta       ProjectMeta
	opts              *StreamConsumeOptions
	result            EventStreamResult
	firstByteReceived *bool
	received          bool
	lg                loggateway.Logger
	consumeStart      time.Time
}

func newTurnStreamConsumer(
	firstByteCtx, turnCtx context.Context,
	projectMeta ProjectMeta,
	firstByteReceived *bool,
	opts *StreamConsumeOptions,
	lg loggateway.Logger,
) *turnStreamConsumer {
	c := &turnStreamConsumer{
		firstByteCtx:      firstByteCtx,
		turnCtx:           turnCtx,
		projectMeta:       projectMeta,
		opts:              opts,
		firstByteReceived: firstByteReceived,
		lg:                lg,
	}
	// AF phase: configure ActivityProjector for direct event consumption.
	// ActivityProjector is mandatory — production always wires it via Wire DI.
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
	// projection, persistence, and WS publishing. It is mandatory in
	// production (wired via Wire DI). When nil (test scenarios without an
	// ActivityProjector), events are simply not projected.
	if c.opts != nil && c.opts.ActivityProjector != nil {
		c.opts.ActivityProjector.ProcessEvent(c.turnCtx, ev)
	}
}

func (c *turnStreamConsumer) publishContextUsageStep() {
	if c.projectMeta.ContextWindow <= 0 || c.result.PromptTok <= 0 {
		return
	}
	if c.opts == nil || c.opts.ActivityProjector == nil {
		return
	}
	turnTotal := c.result.PromptTok + c.result.CompletionTok
	author := strings.TrimSpace(c.projectMeta.AgentDisplayName)
	if author == "" {
		author = "agent"
	}
	meta := map[string]any{
		"context_prompt_tokens": c.result.PromptTok,
		"prompt_tokens":         c.result.PromptTok,
		"completion_tokens":     c.result.CompletionTok,
		"total_tokens":          turnTotal,
		"turn_total_tokens":     turnTotal,
		"max_tokens":            c.projectMeta.ContextWindow,
		"request_id":            c.projectMeta.RequestID,
		"invocation_id":         c.projectMeta.InvocationID,
		"team_id":               c.projectMeta.TeamID,
		"author":                author,
	}
	c.opts.ActivityProjector.EmitSystemEvent(c.turnCtx, biz.ActivityKindNotice, "context_usage", meta)
}

func (c *turnStreamConsumer) finalize() {
	// AF phase: finalize root task Activity with token usage and copy per-member
	// tool call counts to the result. ActivityProjector owns stuck-tool handling
	// and persistence via its sequencer.
	if c.opts == nil || c.opts.ActivityProjector == nil {
		return
	}
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
