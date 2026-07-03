package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// SequencerPublisher is the publish sink for v2 events.
// Implemented by *Sequencer and the test capturingSequencer.
type SequencerPublisher interface {
	Publish(ctx context.Context, e biz.Event)
}

// SeqAssigner allocates monotonic Seq values per spirit session.
// Implemented by *agent.SeqAssigner (NextSeq(spiritSessionID string) int64).
type SeqAssigner interface {
	NextSeq(spiritSessionID string) int64
}

// ActivityProjector projects runtime callbacks into v2 events.
//
// It owns the BeginStep lifecycle: a step is created via BeginStep, streamed
// via OnReasoningDelta/OnTextDelta, and finalized via OnReasoningDone/OnTextDone
// or OnToolResult. All events are constructed via biz factory functions
// (internal/biz/event_factory.go) — struct literals are forbidden because the
// event fields are unexported.
//
// The projector is single-threaded per turn: callers must not invoke callbacks
// for the same turn concurrently. The mutex only protects the activeStep map
// (which may be read by ProcessEvent from a different goroutine).
type ActivityProjector struct {
	seq         SequencerPublisher
	seqAsg      SeqAssigner
	lg          loggateway.Logger
	mu          sync.Mutex
	activeStep  map[string]*biz.Step // stepID → step
	activeTurn  map[string]*biz.Turn // turnID → turn (for OnTurnEnd)
	activeTask  map[string]*biz.Task // taskID → task (root only, for OnTurnEnd)
	stepCounter atomic.Int64

	// === Per-turn ProcessEvent state ===
	// meta is the ProjectMeta for the current turn, set by OnTurnStart.
	meta ProjectMeta
	// thinkingStepID is the stepID of the current thinking step (lazily
	// created on the first reasoning delta, finalized on OnReasoningDone).
	thinkingStepID string
	// replyStepID is the stepID of the current reply step (lazily created
	// on the first text delta, finalized on OnTextDone).
	replyStepID string
	// toolCallSteps maps model tool_call_id → v2 stepID, for correlating
	// tool response events back to the action step created by OnToolCall.
	toolCallSteps map[string]string
}

// NewActivityProjector constructs an ActivityProjector.
// seq and seqAsg may be nil for compile-check scenarios (e.g. the Task 13
// skipped test); callbacks no-op when the sequencer is nil.
func NewActivityProjector(seq SequencerPublisher, seqAsg SeqAssigner, lg loggateway.Logger) *ActivityProjector {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ActivityProjector{
		seq:           seq,
		seqAsg:        seqAsg,
		lg:            lg.With(loggateway.Domain("projector_v2")),
		activeStep:    make(map[string]*biz.Step),
		activeTurn:    make(map[string]*biz.Turn),
		activeTask:    make(map[string]*biz.Task),
		toolCallSteps: make(map[string]string),
	}
}

// OnTurnStart emits task.created (root turns only) followed by turn.started.
// A "root turn" is one with an empty TeamStageID (i.e. the spirit-level turn,
// not a team member sub-turn). Root turns own the Task entity lifecycle.
//
// OnTurnStart also stores the ProjectMeta for ProcessEvent and resets per-turn
// ProcessEvent state (thinkingStepID, replyStepID, toolCallSteps).
func (p *ActivityProjector) OnTurnStart(ctx context.Context, meta ProjectMeta) {
	// Store meta + reset per-turn ProcessEvent state regardless of seq,
	// so that a projector with a nil sequencer (compile-check) still accepts
	// the configuration without panicking on a nil-map write below.
	p.meta = meta
	p.thinkingStepID = ""
	p.replyStepID = ""
	p.toolCallSteps = make(map[string]string)

	if p.seq == nil {
		return
	}
	if meta.TeamStageID == "" {
		task := meta.newTask(meta.TaskID, biz.TaskStatusRunning, meta.TaskContent)
		p.mu.Lock()
		p.activeTask[meta.TaskID] = &task
		p.mu.Unlock()
		p.seq.Publish(ctx, biz.NewTaskCreatedEvent(task))
	}
	var seq int64
	if p.seqAsg != nil {
		seq = p.seqAsg.NextSeq(meta.SpiritSessionID)
	}
	turn := meta.newTurn(meta.TurnID, seq)
	p.mu.Lock()
	p.activeTurn[meta.TurnID] = &turn
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewTurnStartedEvent(turn))
}

// Configure sets the ProjectMeta for the current turn WITHOUT emitting events.
// Used by chat_orchestrator to pre-configure the projector before LLM invocation
// so that plugins/hooks can emit notice/confirm events during the call.
// OnTurnStart (called later by the stream consumer) will emit task.created and
// turn.started events and reset per-turn streaming state.
func (p *ActivityProjector) Configure(meta ProjectMeta) {
	p.meta = meta
}

// Reset clears per-turn state. Called when the projector is reused across turns
// or for explicit cleanup. OnTurnStart also resets streaming state, so Reset is
// mainly for clearing the active entity maps between turns.
func (p *ActivityProjector) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activeStep = make(map[string]*biz.Step)
	p.activeTurn = make(map[string]*biz.Turn)
	p.activeTask = make(map[string]*biz.Task)
	p.thinkingStepID = ""
	p.replyStepID = ""
	p.toolCallSteps = make(map[string]string)
}

// === ActivityEmitter interface (biz.ActivityEmitter) ===
//
// These methods allow plugins/hooks (cost_guard, model_router, tool_confirmation)
// to emit notice/confirm events via the projector without importing the agent
// package. The projector is injected into the context via biz.WithActivityEmitter.

// EmitNotice creates a notice step and immediately completes it. The step
// carries NoticeType metadata for frontend rendering. Implements biz.ActivityEmitter.
func (p *ActivityProjector) EmitNotice(ctx context.Context, content, noticeType string) error {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return nil
	}
	// Inline step creation (instead of BeginStep) so NoticeType is set before
	// the StepCreatedEvent is published — the frontend needs it for rendering.
	n := p.stepCounter.Add(1)
	stepID := p.meta.TurnID + "-s" + strconv.Itoa(int(n))
	step := p.meta.newStep(stepID, biz.StepKindNotice, 0)
	step.NoticeType = noticeType
	step.Content = content
	p.mu.Lock()
	p.activeStep[stepID] = &step
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepCreatedEvent(step))
	p.completeStep(ctx, stepID, "", nil)
	return nil
}

// EmitConfirmRequest creates a confirm step with status=tool_blocked and
// returns the step ID for later result correlation via EmitConfirmResult.
// The step stays in tool_blocked until the user responds. Implements
// biz.ActivityEmitter.
func (p *ActivityProjector) EmitConfirmRequest(ctx context.Context, params biz.ActivityConfirmParams) (string, error) {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return "", nil
	}
	stepID := p.BeginStep(p.meta, biz.StepKindConfirm)
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if ok {
		step.ToolName = params.ToolName
		step.ToolArgs = json.RawMessage(params.ToolArguments)
		step.Content = params.Content
		step.Status = biz.StepStatusToolBlocked
		step.Version++
	}
	p.mu.Unlock()
	if ok {
		p.seq.Publish(ctx, biz.NewStepUpdatedEvent(*step))
	}
	return stepID, nil
}

// EmitConfirmResult updates a confirm step with the user's response:
// approved → completed, denied → cancelled. Returns an error if the step ID
// is not found or is not a confirm step. Implements biz.ActivityEmitter.
func (p *ActivityProjector) EmitConfirmResult(ctx context.Context, stepID string, approved bool) error {
	if p == nil || p.seq == nil {
		return nil
	}
	now := time.Now()
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("confirm step not found: %s", stepID)
	}
	if step.Kind != biz.StepKindConfirm {
		p.mu.Unlock()
		return fmt.Errorf("expected confirm kind, got %s", step.Kind)
	}
	if approved {
		step.Status = biz.StepStatusCompleted
	} else {
		step.Status = biz.StepStatusCancelled
	}
	step.CompletedAt = &now
	step.Version++
	delete(p.activeStep, stepID)
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
	return nil
}

// compile-time interface check
var _ biz.ActivityEmitter = (*ActivityProjector)(nil)

// BeginStep creates a new step of the given kind, stores it in the active map,
// emits step.created, and returns the step ID.
//
// The step ID is derived from the turn ID with a "-s<N>" suffix where N is a
// per-projector counter. The step's Seq is left at 0 and lazily allocated on
// the first streaming delta (see emitStreaming).
func (p *ActivityProjector) BeginStep(meta ProjectMeta, kind biz.StepKind) string {
	if p.seq == nil {
		return ""
	}
	n := p.stepCounter.Add(1)
	stepID := meta.TurnID + "-s" + strconv.Itoa(int(n))
	step := meta.newStep(stepID, kind, 0)
	p.mu.Lock()
	p.activeStep[stepID] = &step
	p.mu.Unlock()
	p.seq.Publish(context.Background(), biz.NewStepCreatedEvent(step))
	return stepID
}

// OnReasoningDelta emits a step.streaming event with DeltaField="reasoning".
// The fourth parameter is reserved for future metadata and currently unused.
func (p *ActivityProjector) OnReasoningDelta(ctx context.Context, stepID, delta, _ string) {
	p.emitStreaming(ctx, stepID, "reasoning", delta)
}

// OnTextDelta emits a step.streaming event with DeltaField="content".
// The fourth parameter is reserved for future metadata and currently unused.
func (p *ActivityProjector) OnTextDelta(ctx context.Context, stepID, delta, _ string) {
	p.emitStreaming(ctx, stepID, "content", delta)
}

// emitStreaming looks up the active step, lazily allocates its Seq if still 0,
// and publishes a StepStreamingEvent. No-op if the step is unknown.
func (p *ActivityProjector) emitStreaming(ctx context.Context, stepID, field, delta string) {
	if p.seq == nil {
		return
	}
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if ok && step.Seq == 0 && p.seqAsg != nil {
		step.Seq = p.seqAsg.NextSeq(step.SpiritSessionID)
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	p.seq.Publish(ctx, biz.NewStepStreamingEvent(step.SpiritSessionID, step.TaskID, stepID, field, delta))
}

// OnReasoningDone finalizes the reasoning content and completes the step.
func (p *ActivityProjector) OnReasoningDone(ctx context.Context, stepID, finalContent string) {
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.Reasoning = finalContent
	}
	p.mu.Unlock()
	p.completeStep(ctx, stepID, "", nil)
}

// OnTextDone finalizes the reply content and completes the step.
func (p *ActivityProjector) OnTextDone(ctx context.Context, stepID, finalContent string, isFinal bool) {
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.IsFinal = isFinal
	}
	p.mu.Unlock()
	p.completeStep(ctx, stepID, finalContent, nil)
}

// OnToolCall ensures an action step exists for the turn (creating one via
// BeginStep if needed), records the tool name/args, transitions it to
// ToolRunning, and emits step.updated.
func (p *ActivityProjector) OnToolCall(ctx context.Context, meta ProjectMeta, toolName string, args json.RawMessage) string {
	if p.seq == nil {
		return ""
	}
	stepID := p.findOrCreateActionStep(meta)
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if ok {
		step.ToolName = toolName
		step.ToolArgs = args
		step.Status = biz.StepStatusToolRunning
		step.Version++
	}
	p.mu.Unlock()
	if ok {
		p.seq.Publish(ctx, biz.NewStepUpdatedEvent(*step))
	}
	return stepID
}

// findOrCreateActionStep returns the ID of an existing action step for the
// turn, or creates a new one via BeginStep. Must be called without holding p.mu.
func (p *ActivityProjector) findOrCreateActionStep(meta ProjectMeta) string {
	p.mu.Lock()
	for id, s := range p.activeStep {
		if s.TurnID == meta.TurnID && s.Kind == biz.StepKindAction {
			p.mu.Unlock()
			return id
		}
	}
	p.mu.Unlock()
	return p.BeginStep(meta, biz.StepKindAction)
}

// OnToolResult completes (success) or fails (error) the action step.
func (p *ActivityProjector) OnToolResult(ctx context.Context, stepID string, result json.RawMessage, err error) {
	if err != nil {
		p.failStep(ctx, stepID, err)
		return
	}
	p.completeStep(ctx, stepID, "", result)
}

// OnTurnEnd emits turn.completed and, for root turns, task.completed.
func (p *ActivityProjector) OnTurnEnd(ctx context.Context, meta ProjectMeta) {
	if p.seq == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	turn, ok := p.activeTurn[meta.TurnID]
	if ok {
		turn.CompletedAt = &now
		turn.Status = biz.TurnStatusCompleted
		turn.Version++
		delete(p.activeTurn, meta.TurnID)
	}
	p.mu.Unlock()
	if ok {
		p.seq.Publish(ctx, biz.NewTurnCompletedEvent(*turn))
	}
	if meta.TeamStageID == "" {
		p.mu.Lock()
		task, tok := p.activeTask[meta.TaskID]
		if tok {
			task.Status = biz.TaskStatusCompleted
			task.CompletedAt = &now
			task.Version++
			delete(p.activeTask, meta.TaskID)
		}
		p.mu.Unlock()
		if tok {
			p.seq.Publish(ctx, biz.NewTaskCompletedEvent(*task))
		}
	}
}

// completeStep marks the step completed, removes it from the active map, and
// emits step.completed. content (if non-empty) overwrites step.Content; result
// (if non-nil) overwrites step.ToolResult.
func (p *ActivityProjector) completeStep(ctx context.Context, stepID, content string, result json.RawMessage) {
	if p.seq == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if !ok {
		p.mu.Unlock()
		return
	}
	if content != "" {
		step.Content = content
	}
	if result != nil {
		step.ToolResult = result
	}
	step.Status = biz.StepStatusCompleted
	step.CompletedAt = &now
	step.Version++
	delete(p.activeStep, stepID)
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
}

// failStep marks the step failed, records the error code, removes it from the
// active map, and emits step.failed.
func (p *ActivityProjector) failStep(ctx context.Context, stepID string, err error) {
	if p.seq == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if !ok {
		p.mu.Unlock()
		return
	}
	step.Status = biz.StepStatusFailed
	step.CompletedAt = &now
	step.Version++
	if err != nil {
		step.ToolErrorCode = err.Error()
	}
	delete(p.activeStep, stepID)
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepFailedEvent(*step))
}

// === ProcessEvent: trpc event → v2 callback dispatch ===

// ProcessEvent dispatches a trpc runtime event to the appropriate v2 callbacks.
// It is the v2 equivalent of v1 ActivityProjector.ProcessEvent.
//
// The projector must have OnTurnStart called (to set ProjectMeta) before
// ProcessEvent is invoked. ProcessEvent owns the BeginStep lifecycle for
// thinking/reply steps: it lazily creates them on the first delta and
// finalizes them on the Done (non-partial) chunk.
//
// Error handling: error-bearing events that are NOT tool responses are logged
// but do not trigger a v2 callback (v2 baseline has no OnError). Tool response
// errors are routed through OnToolResult's err path so the action step is
// marked failed.
func (p *ActivityProjector) ProcessEvent(ctx context.Context, ev *trpcevent.Event) {
	if ev == nil || p.seq == nil || p.meta.TaskID == "" {
		return
	}
	if ev.Response == nil {
		return
	}

	// Tool responses carrying errors are handled by processToolResponse
	// (failStep path), not short-circuited here. Other error events are logged
	// but have no v2 callback in the baseline.
	if ev.Response.Error != nil && ev.Response.Object != trpcmodel.ObjectTypeToolResponse {
		p.lg.Warn("projector_v2: error event (no v2 OnError callback)",
			loggateway.Str("type", ev.Response.Error.Type),
			loggateway.Str("msg", ev.Response.Error.Message))
		return
	}

	switch ev.Response.Object {
	case trpcmodel.ObjectTypeChatCompletionChunk:
		p.processChatChunk(ctx, ev)
	case trpcmodel.ObjectTypeChatCompletion:
		p.processChatCompletion(ctx, ev)
	case trpcmodel.ObjectTypeToolResponse:
		p.processToolResponse(ctx, ev)
	}
}

// processChatChunk handles streaming chat completion chunks.
// Partial chunks emit deltas; non-partial (final) chunks finalize the step.
func (p *ActivityProjector) processChatChunk(ctx context.Context, ev *trpcevent.Event) {
	for _, choice := range ev.Response.Choices {
		// Tool calls (from both Message and Delta)
		allToolCalls := append(choice.Message.ToolCalls, choice.Delta.ToolCalls...)
		for _, tc := range allToolCalls {
			p.handleToolCall(ctx, tc.ID, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
		}

		// Text and reasoning content
		text, reasoning := choiceStreamContent(choice, ev.Response.IsPartial)
		if ev.Response.IsPartial {
			if reasoning != "" {
				p.handleReasoningDelta(ctx, reasoning)
			}
			if text != "" {
				p.handleTextDelta(ctx, text)
			}
		} else {
			// Final chunk: finalize the step even if content is empty.
			p.handleReasoningDone(ctx, reasoning)
			p.handleTextDone(ctx, text)
		}
	}
}

// processChatCompletion handles non-streaming chat completion events.
func (p *ActivityProjector) processChatCompletion(ctx context.Context, ev *trpcevent.Event) {
	for _, choice := range ev.Response.Choices {
		msg := choice.Message

		for _, tc := range msg.ToolCalls {
			p.handleToolCall(ctx, tc.ID, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
		}

		text := strings.TrimSpace(msg.Content)
		reasoning := strings.TrimSpace(msg.ReasoningContent)
		p.handleReasoningDone(ctx, reasoning)
		p.handleTextDone(ctx, text)
	}
}

// processToolResponse handles tool result events.
func (p *ActivityProjector) processToolResponse(ctx context.Context, ev *trpcevent.Event) {
	if len(ev.Response.Choices) == 0 {
		return
	}
	msg := ev.Response.Choices[0].Message
	toolID := strings.TrimSpace(msg.ToolID)
	if toolID == "" {
		return
	}

	p.mu.Lock()
	stepID, ok := p.toolCallSteps[toolID]
	delete(p.toolCallSteps, toolID)
	p.mu.Unlock()
	if !ok {
		// Tool result without a prior OnToolCall — nothing to complete.
		return
	}

	resultJSON := json.RawMessage(msg.Content)
	if ev.Response.Error != nil {
		p.OnToolResult(ctx, stepID, resultJSON, &toolResponseError{msg: ev.Response.Error.Message, code: ev.Response.Error.Type})
		return
	}
	p.OnToolResult(ctx, stepID, resultJSON, nil)
}

// toolResponseError wraps a trpc ResponseError as a Go error so it can be
// passed to OnToolResult's err parameter. failStep records err.Error() as
// ToolErrorCode.
type toolResponseError struct {
	msg  string
	code string
}

func (e *toolResponseError) Error() string {
	if e.code != "" {
		return e.code + ": " + e.msg
	}
	return e.msg
}

// handleToolCall records the tool_call_id → stepID mapping and delegates to OnToolCall.
func (p *ActivityProjector) handleToolCall(ctx context.Context, toolCallID, toolName string, args json.RawMessage) {
	stepID := p.OnToolCall(ctx, p.meta, toolName, args)
	if toolCallID != "" && stepID != "" {
		p.mu.Lock()
		p.toolCallSteps[toolCallID] = stepID
		p.mu.Unlock()
	}
}

// handleReasoningDelta lazily creates a thinking step on the first delta,
// then emits the streaming delta.
func (p *ActivityProjector) handleReasoningDelta(ctx context.Context, delta string) {
	if p.thinkingStepID == "" {
		p.thinkingStepID = p.BeginStep(p.meta, biz.StepKindThinking)
	}
	p.OnReasoningDelta(ctx, p.thinkingStepID, delta, "")
}

// handleTextDelta lazily creates a reply step on the first delta,
// then emits the streaming delta.
func (p *ActivityProjector) handleTextDelta(ctx context.Context, delta string) {
	if p.replyStepID == "" {
		p.replyStepID = p.BeginStep(p.meta, biz.StepKindReply)
	}
	p.OnTextDelta(ctx, p.replyStepID, delta, "")
}

// handleReasoningDone finalizes the thinking step. If no thinking step was
// created (no prior delta) and the content is empty, this is a no-op.
func (p *ActivityProjector) handleReasoningDone(ctx context.Context, finalContent string) {
	if p.thinkingStepID == "" {
		// No thinking step was started. If there's reasoning content, create
		// and immediately complete a step for it; otherwise skip.
		if strings.TrimSpace(finalContent) == "" {
			return
		}
		p.thinkingStepID = p.BeginStep(p.meta, biz.StepKindThinking)
	}
	p.OnReasoningDone(ctx, p.thinkingStepID, finalContent)
	p.thinkingStepID = ""
}

// handleTextDone finalizes the reply step. If no reply step was created and
// the content is empty, this is a no-op.
func (p *ActivityProjector) handleTextDone(ctx context.Context, finalContent string) {
	if p.replyStepID == "" {
		if strings.TrimSpace(finalContent) == "" {
			return
		}
		p.replyStepID = p.BeginStep(p.meta, biz.StepKindReply)
	}
	// isFinal=true for the root turn's reply so the frontend can mark it.
	p.OnTextDone(ctx, p.replyStepID, finalContent, true)
	p.replyStepID = ""
}

// choiceStreamContent extracts text and reasoning from a streaming choice.
// Mirrors internal/agent.ChoiceStreamContent but is duplicated here to avoid
// importing internal/agent (which would re-introduce the circular dependency).
func choiceStreamContent(choice trpcmodel.Choice, partial bool) (text, reasoning string) {
	msg := choice.Message
	delta := choice.Delta
	if partial {
		text = delta.Content
		reasoning = delta.ReasoningContent
		if text == "" {
			text = strings.TrimSpace(msg.Content)
		}
		if reasoning == "" {
			reasoning = strings.TrimSpace(msg.ReasoningContent)
		}
		return text, reasoning
	}
	text = firstNonEmpty(msg.Content, delta.Content)
	reasoning = firstNonEmpty(msg.ReasoningContent, delta.ReasoningContent)
	return text, reasoning
}

// firstNonEmpty returns the first non-empty string after trimming.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
