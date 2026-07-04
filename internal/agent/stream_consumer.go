package agent

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/agent/v2"
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
	// canceled tracks whether the turn was cancelled (context cancellation
	// or first-byte timeout). Set in consume(), read in finalize() to
	// propagate the correct terminal status to OnTurnEnd.
	canceled bool

	// v2Projector is the per-turn v2 ActivityProjector. When non-nil, all
	// trpc events are projected into v2 events (Step/Task/Turn). The v1
	// dual-path has been removed; v2 is the sole projection path.
	// 2026-07-04 问题 4 修复：每个 turn（spirit + 每个 team member）由
	// ProjectorFactory.NewProjector() 创建独立实例，避免共享单例的竞态。
	v2Projector *v2.ActivityProjector
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
	// v2 path: wire the v2 projector from opts and start the v2 turn.
	// The v2 projector was pre-configured (Configure) by the chat_orchestrator
	// or team runner before LLM invocation. Each turn gets its own projector
	// instance via ProjectorFactory.NewProjector() (per-turn isolation).
	// OnTurnStart emits task.created + turn.started, preserving any early
	// notice/confirm steps emitted by plugins during the LLM call.
	if opts != nil && opts.V2Projector != nil {
		c.v2Projector = opts.V2Projector
		v2Meta := V2ProjectMetaFromV1(projectMeta)
		c.v2Projector.OnTurnStart(turnCtx, v2Meta)
	}
	return c
}

// V2ProjectMetaFromV1 converts a v1 ProjectMeta to a v2 ProjectMeta.
// The v2 ProjectMeta is a subset of v1's fields (the v2 model has fewer
// session-tree fields; the rest are derived at the team/graph layer).
// Exported so chat_orchestrator and team runner can construct v2 meta
// without duplicating the field mapping.
func V2ProjectMetaFromV1(m ProjectMeta) v2.ProjectMeta {
	return v2.ProjectMeta{
		SessionID:       m.SessionID,
		SpiritSessionID: m.SpiritSessionID,
		TaskID:          m.RequestID,
		TurnID:          m.InvocationID,
		ParentTurnID:    m.ParentInvocationID,
		TeamStageID:     m.TeamID, // team member turns are identified by non-empty TeamID
		TeamRunID:       "",
		TeamID:          m.TeamID,
		MemberSessionID: m.ParentSessionID,
		AgentKey:        m.AgentID,
		AgentName:       m.AgentDisplayName,
		MemberAgentKeys: m.MemberAgentKeys,
		TaskContent:     m.TaskContent,
		ParentTaskID:    m.ParentTaskID,
		NodeIDToAgentKey: m.NodeIDToAgentKey,
	}
}

func (c *turnStreamConsumer) consume(events <-chan *trpcevent.Event) EventStreamResult {
	c.consumeStart = time.Now()
	evIdx := 0
	for ev := range events {
		evIdx++
		if !c.canceled && c.turnCtx.Err() != nil {
			c.canceled = true
			c.lg.With(loggateway.StepID("stream.consume_exit")).Info("stream consume: turnCtx canceled, draining important events",
				loggateway.Any("ev_count", evIdx))
			// Do NOT return immediately — continue draining to ensure
			// Important events (ToolResult, Error, RunnerCompletion)
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
			c.lg.With(loggateway.StepID("stream.first_byte_timeout")).Info("stream consume: firstByte timeout, draining important events",
				loggateway.Any("ev_count", evIdx))
			// Do NOT return immediately — drain Important events (ToolResult, Error, RunnerCompletion)
			// just like the turnCtx cancel path, to prevent resource leaks and
			// ensure the frontend receives terminal signals.
			c.canceled = true
		}
		if !c.handleEvent(ev) {
			c.finalize()
			return c.result
		}
		// After first-byte timeout or context cancellation, stop once we see
		// RunnerCompletion (the terminal event) to avoid draining indefinitely.
		if c.canceled && ev.IsRunnerCompletion() {
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
			c.result.UsageSource = "runner_completion"
		}
		return true
	}

	if ev.Response != nil && ev.Response.Error != nil {
		// TECH-DEBT(usage-source): when the stream errors mid-flight, the framework
		// (pkg/trpc-agent-go/model/openai/openai.go:emitStreamingFinalResponse) suppresses
		// the final chat.completion event that carries the usage payload. This causes
		// PromptTok/CompletionTok to remain at 0 even though tokens were consumed.
		// Cannot fix here (red line #27 — framework source is read-only). The
		// downstream EstimateTokensIfMissing fallback estimates from text; UsageSource
		// remains "" to signal the missing usage for observability/diagnostics.
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
		// Streaming usage is interim; only mark as streaming if RunnerCompletion
		// hasn't already set the authoritative source.
		if c.result.UsageSource == "" {
			c.result.UsageSource = "streaming"
		}
	}

	partial := ev.Response.IsPartial
	for _, choice := range ev.Response.Choices {
		accumulateChoiceStream(&c.result, choice, partial)
	}
	return true
}

func (c *turnStreamConsumer) projectAndTrackTools(ev *trpcevent.Event) {
	// v2 path: the v2 projector directly consumes trpc events and projects
	// them into v2 events (Step/Task/Turn). When nil (test scenarios without
	// a v2 projector), events are simply not projected.
	if c.v2Projector != nil {
		c.v2Projector.ProcessEvent(c.turnCtx, ev)
	}
}

func (c *turnStreamConsumer) publishContextUsageStep() {
	if c.projectMeta.ContextWindow <= 0 || c.result.PromptTok <= 0 {
		return
	}
	if c.v2Projector == nil {
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
	c.v2Projector.EmitSystemEvent(c.turnCtx, biz.ActivityKindNotice, "context_usage", meta)
}

func (c *turnStreamConsumer) finalize() {
	// v2 path: finalize the v2 turn. OnTurnEndEnhanced handles stuck-tool
	// detection, remaining-step finalization, and emits turn.completed +
	// task.completed. Close is a no-op for the per-turn v2 projector.
	if c.v2Projector != nil {
		v2Meta := V2ProjectMetaFromV1(c.projectMeta)
		usage := &v2.ActivityUsage{
			PromptTokens:     c.result.PromptTok,
			CompletionTokens: c.result.CompletionTok,
			TotalTokens:      c.result.PromptTok + c.result.CompletionTok,
		}
		c.v2Projector.OnTurnEndEnhanced(c.turnCtx, v2Meta, usage, c.canceled)
		c.v2Projector.Close()
		if mtc := c.v2Projector.MemberToolCalls(); len(mtc) > 0 {
			c.result.MemberToolCalls = mtc
		}
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
