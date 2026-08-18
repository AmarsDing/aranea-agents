package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

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

	// doomDetector watches streamed reply deltas for repetitive output.
	// On detection the turn aborts early (result.DoomLoopDetected=true)
	// instead of streaming an unbounded repetitive response.
	doomDetector *DoomLoopDetector

	// chunkEvents 计数高频 chunk 类事件（text_delta/response），用于
	// stream.event 日志采样节流。consume 循环单 goroutine 访问，无需 atomic。
	chunkEvents int64
}

// streamEventSampleInterval 是高频流式事件日志的采样间隔（首条 + 每 N 条）。
// 00:52 会话补充取证：consume 循环对每个 event 写一条 stream.event Info
// （实测 8287 条/4min），违反「高频路径计数器限流」红线。
const streamEventSampleInterval = 200

// shouldLogStreamEvent 决定第 count 次（1 起）的 evType 事件是否写日志：
// chunk 类（text_delta/response）单条审计价值近零，采样；tool_call /
// runner_completion / response_error 等重要事件逐条保留。
func shouldLogStreamEvent(evType string, count int64) bool {
	switch evType {
	case "text_delta", "response":
		return count == 1 || count%streamEventSampleInterval == 0
	default:
		return true
	}
}

func newTurnStreamConsumer(
	firstByteCtx, turnCtx context.Context,
	projectMeta ProjectMeta,
	firstByteReceived *bool,
	opts *StreamConsumeOptions,
	lg loggateway.Logger,
) *turnStreamConsumer {
	// 带上 session_id：TTFT/stream 事件可按会话关联（语音延迟分段排查）。
	if projectMeta.SessionID != "" {
		lg = lg.With(loggateway.SessionID(projectMeta.SessionID))
	}
	c := &turnStreamConsumer{
		firstByteCtx:      firstByteCtx,
		turnCtx:           turnCtx,
		projectMeta:       projectMeta,
		opts:              opts,
		firstByteReceived: firstByteReceived,
		lg:                lg,
		// 5 consecutive near-identical substantial deltas (≥20 chars) signals a
		// doom loop. Short structural deltas ("]", "\n", "1.") repeat legitimately
		// and are filtered at observation time.
		doomDetector: NewDoomLoopDetector(5, 0.95),
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
		// 2026-07-05 P1 #7 修复：使用 m.TeamStageID（派生自 NewTeamStageActivityID(teamID)），
		// 而非 m.TeamID。Turn.TeamStageID 应指向 TeamStage.ID，与 TeamStage.TeamID 不同。
		// 旧代码 TeamStageID: m.TeamID 导致 Turn.TeamStageID == Turn.TeamID，
		// 前端无法通过 TeamStageID 精确匹配 member steps。
		TeamStageID:      m.TeamStageID,
		TeamRunID:        "",
		TeamID:           m.TeamID,
		MemberSessionID:  m.ParentSessionID,
		AgentKey:         m.AgentID,
		AgentName:        m.AgentDisplayName,
		MemberAgentKeys:  m.MemberAgentKeys,
		TaskContent:      m.TaskContent,
		ParentTaskID:     m.ParentTaskID,
		Synthesis:        m.Synthesis,
		NodeIDToAgentKey: m.NodeIDToAgentKey,
	}
}

func (c *turnStreamConsumer) consume(events <-chan *trpcevent.Event) (result EventStreamResult) {
	c.consumeStart = time.Now()
	// Y2: panic 兜底。consume 在投影/处理链路上经过 projector、metrics、
	// provider 等大量代码，任何一处 panic 若向上穿透，finalize 不会执行——
	// turn/task 永远停在 running，前端卡死。recover 后按 Cancelled 收尾并
	// 照常发布终态事件。
	defer func() {
		if r := recover(); r != nil {
			c.lg.Error("stream consume panic recovered",
				loggateway.StepID("stream.consume_panic"),
				loggateway.Any("panic", r),
				loggateway.Str("stack", string(debug.Stack())))
			c.result.HasError = true
			c.result.LastError = fmt.Sprintf("stream consumer panic: %v", r)
			c.canceled = true
			c.drainEventsAsync(events)
			c.finalize()
			result = c.result
		}
	}()
	evIdx := 0
	for {
		ev, ok := c.nextEvent(events)
		if c.result.FirstByteTimedOut {
			return c.result
		}
		if !ok {
			c.lg.With(loggateway.StepID("stream.consume_done")).Info("stream consume: channel closed",
				loggateway.Any("ev_count", evIdx))
			c.finalize()
			return c.result
		}
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
		// 高频 chunk 类事件采样节流：首条 + 每 streamEventSampleInterval 条
		// 记录一次并附累计计数；重要事件（tool_call/runner_completion/
		// response_error）逐条保留。
		if evType == "text_delta" || evType == "response" {
			c.chunkEvents++
		}
		if shouldLogStreamEvent(evType, c.chunkEvents) {
			c.lg.With(loggateway.StepID("stream.event")).Info("stream event",
				loggateway.Any("idx", evIdx),
				loggateway.Any("type", evType),
				loggateway.Any("author", ev.Author),
				loggateway.Any("chunk_count", c.chunkEvents))
		}
		c.markFirstByte(ev)
		if !c.handleEvent(ev) {
			// Y1: 早退（doom-loop）必须后台排干 events channel——trpc runner
			// 生产者 goroutine 仍在写入，不排干会在 buffer 满后永久阻塞泄漏
			// （LLM 流也持续烧 token）。canceled 已在检测点置位（终态 Cancelled）。
			c.drainEventsAsync(events)
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
}

// nextEvent waits for the next stream event. Before the first meaningful
// model byte it also selects on firstByteCtx so a silent provider can be
// aborted instead of blocking on a muted channel until the HTTP timeout.
func (c *turnStreamConsumer) nextEvent(events <-chan *trpcevent.Event) (*trpcevent.Event, bool) {
	if c.received || c.firstByteCtx == nil {
		ev, ok := <-events
		return ev, ok
	}
	select {
	case ev, ok := <-events:
		return ev, ok
	case <-c.firstByteCtx.Done():
		if c.shouldAbortForStall() {
			c.abortForStall(events)
			return nil, false
		}
		ev, ok := <-events
		return ev, ok
	}
}

func (c *turnStreamConsumer) shouldAbortForStall() bool {
	if c.received || c.firstByteCtx == nil {
		return false
	}
	if !errors.Is(c.firstByteCtx.Err(), context.DeadlineExceeded) {
		return false
	}
	// Parent deadline (turnCtx) firing makes the child timer DeadlineExceeded
	// as well; that is a caller timeout, not the first-byte stall guard.
	if errors.Is(c.turnCtx.Err(), context.DeadlineExceeded) {
		return false
	}
	return true
}

func (c *turnStreamConsumer) abortForStall(events <-chan *trpcevent.Event) {
	c.result.FirstByteTimedOut = true
	c.result.HasError = true
	c.result.LastError = ErrFirstByteTimeout.Error()
	c.canceled = true
	c.lg.With(loggateway.StepID("stream.first_byte_timeout")).Info("stream consume: first-byte stall, aborting LLM request",
		loggateway.Any("elapsed_ms", time.Since(c.consumeStart).Milliseconds()))
	if c.opts != nil && c.opts.AbortOnStall != nil {
		c.opts.AbortOnStall()
	}
	c.drainEventsAsync(events)
	c.finalize()
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
	if ev.Response != nil && ev.Response.Error != nil {
		return true
	}
	// RunnerCompletion after a silent hang is a cancellation artifact, not a
	// model first byte. Treating it as TTFT hid ErrFirstByteTimeout and made
	// the turn look like an empty_reply.
	if ev.IsRunnerCompletion() {
		return false
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
			if ct := ev.Response.Usage.PromptTokensDetails.CachedTokens; ct > c.result.CachedTok {
				c.result.CachedTok = ct
			}
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
		accumulateStreamUsage(&c.result, ev, c.projectMeta, usage.PromptTokens, usage.CompletionTokens, usage.PromptTokensDetails.CachedTokens)
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
		if c.doomDetector != nil {
			text, _ := ChoiceStreamContent(choice, partial)
			// Filter short structural deltas: they repeat legitimately and
			// would false-positive the similarity check.
			if len(text) >= 20 && c.doomDetector.Observe(text) {
				c.result.HasError = true
				c.result.DoomLoopDetected = true
				c.result.LastError = "doom loop detected: repetitive LLM output, turn aborted"
				// Y1: turn 被中止，终态必须是 Cancelled 而非 Completed——
				// finalize 以 c.canceled 决定 OnTurnEndEnhanced 的终态。
				c.canceled = true
				c.lg.Warn("doom loop detected, aborting turn stream",
					loggateway.StepID("stream.doom_loop"),
					loggateway.Str("invocation_id", c.projectMeta.InvocationID))
				return false
			}
		}
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

// drainEventsAsync 在后台排干剩余 trpc 事件。consume 早退（doom-loop /
// panic recover）后，runner 生产者 goroutine 仍在向 channel 写入；不排干
// 会在 buffer 满后永久阻塞（goroutine 泄漏 + LLM 流持续消耗）。生产者
// 完成 run 后关闭 channel，drainer 随之退出。红线 #13：走 safego。
func (c *turnStreamConsumer) drainEventsAsync(events <-chan *trpcevent.Event) {
	safego.GoBackground("stream-consumer-drain", func() {
		for range events {
		}
	})
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
		if partial {
			_ = provider.VisibleStreamingDelta(&result.Reply, text)
		} else {
			// Non-partial (final aggregated) events carry the full message, not a
			// delta. Appending it to the already-accumulated deltas would
			// duplicate the text — reset and replace instead (aligned with
			// llmcompat.go final-response semantics).
			result.Reply.Reset()
			result.Reply.WriteString(text)
		}
		result.HasContent = true
	}
	if reasoning != "" {
		if partial {
			_ = provider.VisibleStreamingDelta(&result.Reasoning, reasoning)
		} else {
			result.Reasoning.Reset()
			result.Reasoning.WriteString(reasoning)
		}
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
