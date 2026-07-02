package v2

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
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
}

// NewActivityProjector constructs an ActivityProjector.
// seq and seqAsg may be nil for compile-check scenarios (e.g. the Task 13
// skipped test); callbacks no-op when the sequencer is nil.
func NewActivityProjector(seq SequencerPublisher, seqAsg SeqAssigner, lg loggateway.Logger) *ActivityProjector {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ActivityProjector{
		seq:        seq,
		seqAsg:     seqAsg,
		lg:         lg.With(loggateway.Domain("projector_v2")),
		activeStep: make(map[string]*biz.Step),
		activeTurn: make(map[string]*biz.Turn),
		activeTask: make(map[string]*biz.Task),
	}
}

// OnTurnStart emits task.created (root turns only) followed by turn.started.
// A "root turn" is one with an empty TeamStageID (i.e. the spirit-level turn,
// not a team member sub-turn). Root turns own the Task entity lifecycle.
func (p *ActivityProjector) OnTurnStart(ctx context.Context, meta ProjectMeta) {
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
	if err != nil {
		step.ToolErrorCode = err.Error()
	}
	delete(p.activeStep, stepID)
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepFailedEvent(*step))
}
