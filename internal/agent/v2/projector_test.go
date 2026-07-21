package v2

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// capturingSequencer collects published events for test assertions.
type capturingSequencer struct {
	events []biz.Event
}

func (c *capturingSequencer) Publish(_ context.Context, e biz.Event) {
	c.events = append(c.events, e)
}

func testProjector() (*ActivityProjector, *capturingSequencer) {
	capture := &capturingSequencer{}
	p := NewActivityProjector(capture, nil, nil)
	p.OnTurnStart(context.Background(), ProjectMeta{
		TaskID:          "task-1",
		TurnID:          "turn-1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		AgentKey:        "agent-1",
	})
	return p, capture
}

func TestEmitNotice(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil // only care about notice events

	err := p.EmitNotice(context.Background(), "model switched to gpt-4", "model_router")
	if err != nil {
		t.Fatalf("EmitNotice returned error: %v", err)
	}

	// Expect 2 events: StepCreatedEvent + StepCompletedEvent
	if len(capture.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(capture.events))
	}

	created, ok := capture.events[0].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected StepCreatedEvent, got %T", capture.events[0])
	}
	if created.Step.Kind != biz.StepKindNotice {
		t.Errorf("expected kind=notice, got %s", created.Step.Kind)
	}
	if created.Step.Content != "model switched to gpt-4" {
		t.Errorf("expected content set on creation, got %s", created.Step.Content)
	}
	if created.Step.NoticeType != "model_router" {
		t.Errorf("expected noticeType=model_router, got %s", created.Step.NoticeType)
	}

	completed, ok := capture.events[1].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[1])
	}
	if completed.Step.Status != biz.StepStatusCompleted {
		t.Errorf("expected status=completed, got %s", completed.Step.Status)
	}
	if completed.Step.Content != "model switched to gpt-4" {
		t.Errorf("expected content set on completion, got %s", completed.Step.Content)
	}
	if completed.Step.NoticeType != "model_router" {
		t.Errorf("expected noticeType preserved, got %s", completed.Step.NoticeType)
	}
}

func TestEmitConfirmRequest(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	stepID, err := p.EmitConfirmRequest(context.Background(), biz.ActivityConfirmParams{
		ToolName:      "shell",
		ToolArguments: `{"cmd":"rm -rf /"}`,
		Content:       "Allow shell execution?",
	})
	if err != nil {
		t.Fatalf("EmitConfirmRequest returned error: %v", err)
	}
	if stepID == "" {
		t.Fatal("expected non-empty stepID")
	}

	// Expect 2 events: StepCreatedEvent (pending) + StepUpdatedEvent (tool_blocked)
	if len(capture.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(capture.events))
	}

	created, ok := capture.events[0].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected StepCreatedEvent, got %T", capture.events[0])
	}
	if created.Step.Kind != biz.StepKindConfirm {
		t.Errorf("expected kind=confirm, got %s", created.Step.Kind)
	}
	if created.Step.Status != biz.StepStatusPending {
		t.Errorf("expected initial status=pending, got %s", created.Step.Status)
	}

	updated, ok := capture.events[1].(*biz.StepUpdatedEvent)
	if !ok {
		t.Fatalf("expected StepUpdatedEvent, got %T", capture.events[1])
	}
	if updated.Step.Status != biz.StepStatusToolBlocked {
		t.Errorf("expected status=tool_blocked, got %s", updated.Step.Status)
	}
	if updated.Step.ToolName != "shell" {
		t.Errorf("expected toolName=shell, got %s", updated.Step.ToolName)
	}
	if updated.Step.Content != "Allow shell execution?" {
		t.Errorf("expected content set, got %s", updated.Step.Content)
	}
	if string(updated.Step.ToolArgs) != `{"cmd":"rm -rf /"}` {
		t.Errorf("expected toolArgs set, got %s", string(updated.Step.ToolArgs))
	}
}

func TestEmitConfirmResult(t *testing.T) {
	p, _ := testProjector()

	// Test approved
	stepID, _ := p.EmitConfirmRequest(context.Background(), biz.ActivityConfirmParams{
		ToolName: "shell",
		Content:  "Allow?",
	})
	capture := p.seq.(*capturingSequencer)
	capture.events = nil

	err := p.EmitConfirmResult(context.Background(), stepID, true)
	if err != nil {
		t.Fatalf("EmitConfirmResult approved returned error: %v", err)
	}
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event for approved, got %d", len(capture.events))
	}
	completed, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if completed.Step.Status != biz.StepStatusCompleted {
		t.Errorf("expected status=completed, got %s", completed.Step.Status)
	}
	if completed.Step.Kind != biz.StepKindConfirm {
		t.Errorf("expected kind=confirm, got %s", completed.Step.Kind)
	}

	// Test denied
	stepID2, _ := p.EmitConfirmRequest(context.Background(), biz.ActivityConfirmParams{
		ToolName: "shell",
		Content:  "Allow again?",
	})
	capture.events = nil

	err = p.EmitConfirmResult(context.Background(), stepID2, false)
	if err != nil {
		t.Fatalf("EmitConfirmResult denied returned error: %v", err)
	}
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event for denied, got %d", len(capture.events))
	}
	cancelled, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if cancelled.Step.Status != biz.StepStatusCancelled {
		t.Errorf("expected status=cancelled, got %s", cancelled.Step.Status)
	}

	// Test not found
	err = p.EmitConfirmResult(context.Background(), "nonexistent", true)
	if err == nil {
		t.Error("expected error for nonexistent stepID")
	}
}

func TestOnError(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil // only care about error events

	p.OnError(context.Background(), "LLM call failed", "rate_limit", "429")

	// Expect 3 events: StepCreatedEvent (error) + StepCompletedEvent + TaskFailedEvent
	if len(capture.events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(capture.events))
	}

	// Event[0]: error StepCreatedEvent
	errCreated, ok := capture.events[0].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected StepCreatedEvent, got %T", capture.events[0])
	}
	if errCreated.Step.Kind != biz.StepKindError {
		t.Errorf("expected kind=error, got %s", errCreated.Step.Kind)
	}
	if errCreated.Step.Content != "LLM call failed" {
		t.Errorf("expected content=LLM call failed, got %s", errCreated.Step.Content)
	}
	if errCreated.Step.ToolErrorCode != "rate_limit" {
		t.Errorf("expected toolErrorCode=rate_limit, got %s", errCreated.Step.ToolErrorCode)
	}

	// Event[1]: error StepCompletedEvent
	errCompleted, ok := capture.events[1].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[1])
	}
	if errCompleted.Step.Status != biz.StepStatusCompleted {
		t.Errorf("expected status=completed, got %s", errCompleted.Step.Status)
	}

	// Event[2]: TaskFailedEvent
	failed, ok := capture.events[2].(*biz.TaskFailedEvent)
	if !ok {
		t.Fatalf("expected TaskFailedEvent, got %T", capture.events[2])
	}
	if failed.Task.Status != biz.TaskStatusFailed {
		t.Errorf("expected task status=failed, got %s", failed.Task.Status)
	}
}

func TestOnStuckTools(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	// Create an action step and set it to tool_running (simulating a tool call
	// that never received a result).
	stepID := p.BeginStep(p.meta, biz.StepKindAction)
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.Status = biz.StepStatusToolRunning
		step.ToolName = "shell"
	}
	p.mu.Unlock()
	capture.events = nil // reset to only see stuck-tool events

	p.OnStuckTools(context.Background())

	// Expect 1 event: StepFailedEvent for the stuck tool
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(capture.events))
	}
	failed, ok := capture.events[0].(*biz.StepFailedEvent)
	if !ok {
		t.Fatalf("expected StepFailedEvent, got %T", capture.events[0])
	}
	if failed.Step.Status != biz.StepStatusFailed {
		t.Errorf("expected status=failed, got %s", failed.Step.Status)
	}
	if failed.Step.ToolErrorCode != "tool_timeout" {
		t.Errorf("expected toolErrorCode=tool_timeout, got %s", failed.Step.ToolErrorCode)
	}
}

func TestOnTurnEndEnhanced(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	usage := &ActivityUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}
	p.OnTurnEndEnhanced(context.Background(), p.meta, usage, false)

	// Expect 2 events: TurnCompletedEvent + TaskCompletedEvent
	// (no stuck tools, no remaining active steps)
	if len(capture.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(capture.events))
	}
	tc, ok := capture.events[0].(*biz.TurnCompletedEvent)
	if !ok {
		t.Fatalf("expected TurnCompletedEvent, got %T", capture.events[0])
	}
	if tc.Turn.Status != biz.TurnStatusCompleted {
		t.Errorf("expected turn status=completed, got %s", tc.Turn.Status)
	}
	taskC, ok := capture.events[1].(*biz.TaskCompletedEvent)
	if !ok {
		t.Fatalf("expected TaskCompletedEvent, got %T", capture.events[1])
	}
	if taskC.Task.Status != biz.TaskStatusCompleted {
		t.Errorf("expected task status=completed, got %s", taskC.Task.Status)
	}
}

func TestOnTurnEndEnhancedCanceledWithStuckTool(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	// Create a stuck tool step
	stepID := p.BeginStep(p.meta, biz.StepKindAction)
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.Status = biz.StepStatusToolRunning
		step.ToolName = "shell"
	}
	p.mu.Unlock()
	capture.events = nil

	p.OnTurnEndEnhanced(context.Background(), p.meta, nil, true)

	// Expect 3 events: StepFailedEvent (stuck) + TurnCompletedEvent + TaskCompletedEvent
	if len(capture.events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(capture.events))
	}
	// Event[0]: stuck tool failed
	if _, ok := capture.events[0].(*biz.StepFailedEvent); !ok {
		t.Fatalf("expected StepFailedEvent, got %T", capture.events[0])
	}
	// Event[1]: turn completed
	if _, ok := capture.events[1].(*biz.TurnCompletedEvent); !ok {
		t.Fatalf("expected TurnCompletedEvent, got %T", capture.events[1])
	}
	// Event[2]: task completed
	if _, ok := capture.events[2].(*biz.TaskCompletedEvent); !ok {
		t.Fatalf("expected TaskCompletedEvent, got %T", capture.events[2])
	}
}

func TestEmitSystemEvent(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	p.EmitSystemEvent(context.Background(), biz.ActivityKindNotice, "context_usage", map[string]any{
		"type":          "context_window",
		"prompt_tokens": 1000,
	})

	// EmitSystemEvent delegates to EmitNotice → 2 events (created + completed)
	if len(capture.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(capture.events))
	}
	created, ok := capture.events[0].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected StepCreatedEvent, got %T", capture.events[0])
	}
	if created.Step.Kind != biz.StepKindNotice {
		t.Errorf("expected kind=notice, got %s", created.Step.Kind)
	}
	if created.Step.NoticeType != "context_window" {
		t.Errorf("expected noticeType=context_window, got %s", created.Step.NoticeType)
	}
	if created.Step.Content != "context_usage" {
		t.Errorf("expected content=context_usage, got %s", created.Step.Content)
	}
}

func TestMemberToolCalls(t *testing.T) {
	p, _ := testProjector()

	// Root turn (no TeamStageID) — no member tracking
	p.OnToolCall(context.Background(), p.meta, "tool1", nil)
	if mtc := p.MemberToolCalls(); mtc != nil {
		t.Errorf("expected nil for root turn, got %v", mtc)
	}

	// Team member turn (TeamStageID set)
	teamMeta := p.meta
	teamMeta.TeamStageID = "team-stage-1"
	teamMeta.AgentKey = "member-1"
	p.OnToolCall(context.Background(), teamMeta, "tool1", nil)
	p.OnToolCall(context.Background(), teamMeta, "tool2", nil)

	mtc := p.MemberToolCalls()
	if mtc == nil {
		t.Fatal("expected non-nil memberToolCalls")
	}
	if mtc["member-1"] != 2 {
		t.Errorf("expected member-1 count=2, got %d", mtc["member-1"])
	}
}

// TestHandleTextDelta_WhitespaceNoCreate verifies that a pure-whitespace
// leading delta does NOT create a ReplyStep. Only when a delta with non-blank
// content arrives should the step be created. This prevents empty ReplyBlocks
// in the frontend when LLM emits leading "\n" or " " deltas.
//
// Spec: docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md §4.1.1
func TestHandleTextDelta_WhitespaceNoCreate(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil // ignore turn.started/task.created setup

	// 1. Pure-whitespace delta: should NOT create a step
	p.handleTextDelta(context.Background(), "\n   ")
	if stepID, ok := p.replyStepIDs[p.meta.AgentKey]; ok && stepID != "" {
		t.Errorf("expected replyStepIDs empty after whitespace delta, got %q", stepID)
	}
	if len(capture.events) != 0 {
		t.Errorf("expected 0 events after whitespace delta, got %d", len(capture.events))
	}

	// 2. Real-content delta: should create a step + emit streaming
	p.handleTextDelta(context.Background(), "Hello")
	if stepID, ok := p.replyStepIDs[p.meta.AgentKey]; !ok || stepID == "" {
		t.Fatal("expected replyStepIDs set after real-content delta")
	}
	// Expect 2 events: StepCreatedEvent (BeginStep) + StepStreamingEvent (OnTextDelta)
	if len(capture.events) != 2 {
		t.Fatalf("expected 2 events after real-content delta, got %d", len(capture.events))
	}
	created, ok := capture.events[0].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected events[0]=StepCreatedEvent, got %T", capture.events[0])
	}
	if created.Step.Kind != biz.StepKindReply {
		t.Errorf("expected kind=reply, got %s", created.Step.Kind)
	}
	if _, ok := capture.events[1].(*biz.StepStreamingEvent); !ok {
		t.Fatalf("expected events[1]=StepStreamingEvent, got %T", capture.events[1])
	}

	// 3. Subsequent whitespace delta on existing step: should still stream
	//    (whitespace is meaningful content once the step exists)
	capture.events = nil
	p.handleTextDelta(context.Background(), " \n")
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 streaming event for trailing whitespace, got %d", len(capture.events))
	}
	if _, ok := capture.events[0].(*biz.StepStreamingEvent); !ok {
		t.Fatalf("expected StepStreamingEvent, got %T", capture.events[0])
	}
}

// TestHandleTextDone_EmptyContentCancelled verifies that when a ReplyStep was
// created (e.g. by an earlier non-blank delta that turned out to be only
// whitespace after TrimSpace) but finalContent is empty, the step is finalized
// with Status=cancelled (not completed) so the frontend can filter it out.
//
// Spec: docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md §4.1.2
func TestHandleTextDone_EmptyContentCancelled(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	// Simulate: LLM emitted a delta that created the step, but the accumulated
	// content is whitespace. Use BeginStep directly to control the step's
	// initial Content, then call handleTextDone with empty finalContent.
	stepID := p.BeginStep(p.meta, biz.StepKindReply)
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.Content = " " // whitespace-only accumulated content
	}
	p.mu.Unlock()
	p.replyStepIDs[p.meta.AgentKey] = stepID
	capture.events = nil // ignore the step.created from BeginStep

	p.handleTextDone(context.Background(), "")

	// Expect 1 event: StepCompletedEvent with Status=cancelled, IsFinal=false
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(capture.events))
	}
	completed, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if completed.Step.Status != biz.StepStatusCancelled {
		t.Errorf("expected status=cancelled, got %s", completed.Step.Status)
	}
	if completed.Step.IsFinal {
		t.Error("expected IsFinal=false for cancelled empty reply")
	}
	if stepID, ok := p.replyStepIDs[p.meta.AgentKey]; ok && stepID != "" {
		t.Errorf("expected replyStepIDs cleared after done, got %q", stepID)
	}
}

// TestHandleTextDone_NormalReply verifies the normal path still works:
// real content → Status=completed, IsFinal=true.
func TestHandleTextDone_NormalReply(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	p.handleTextDelta(context.Background(), "Hello")
	capture.events = nil // ignore created/streaming

	p.handleTextDone(context.Background(), "Hello world")

	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(capture.events))
	}
	completed, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if completed.Step.Status != biz.StepStatusCompleted {
		t.Errorf("expected status=completed, got %s", completed.Step.Status)
	}
	if !completed.Step.IsFinal {
		t.Error("expected IsFinal=true for normal reply")
	}
	if completed.Step.Content != "Hello world" {
		t.Errorf("expected content='Hello world', got %q", completed.Step.Content)
	}
}

// TestHandleTextDone_NoReplyStep_NoOp verifies that when no ReplyStep exists
// and finalContent is empty, no events are emitted (existing no-op path).
func TestHandleTextDone_NoReplyStep_NoOp(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	p.handleTextDone(context.Background(), "")

	if len(capture.events) != 0 {
		t.Fatalf("expected 0 events for no-op, got %d", len(capture.events))
	}
	if stepID, ok := p.replyStepIDs[p.meta.AgentKey]; ok && stepID != "" {
		t.Errorf("expected replyStepIDs empty, got %q", stepID)
	}
}
