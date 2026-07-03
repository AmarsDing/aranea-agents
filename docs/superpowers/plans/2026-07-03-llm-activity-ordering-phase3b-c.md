# LLM Activity Ordering Phase 3b-C: Extend v2 Projector + Delete v1 ActivityProjector

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the v2 `ActivityProjector` to fully cover v1's functionality (ActivityEmitter interface, OnError, OnStuckTools, EmitSystemEvent, MemberToolCalls, enhanced OnTurnEnd), wire it into all call sites (chat_orchestrator + team runner + stream_consumer), then delete the v1 `activity_projector.go` (1841 lines) and `activity_event_sequencer.go`.

**Architecture:** The v1 `ActivityProjector` is a per-turn projector that processes trpc-agent-go events into `biz.Activity` entities and publishes them via the v1 `activityEventSequencer`. It also implements `biz.ActivityEmitter` (3 methods) for plugins/hooks. The v2 `ActivityProjector` is a singleton that processes trpc-agent-go events into `biz.Step`/`biz.Task`/`biz.Turn` entities and publishes via the v2 `Sequencer`. v2 currently covers ~40% of v1's functionality (chat rendering only). This plan extends v2 to cover 100%, then deletes v1.

**Tech Stack:** Go, trpc-agent-go, Kratos v2, Wire DI, Vue 3/TypeScript frontend

**Spec:** `docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md` §Phase 3

---

## File Structure

### Modified Files

| File | Change |
|------|--------|
| `internal/biz/step.go` | Add `NoticeType string` field to `Step` struct |
| `internal/agent/v2/projector.go` | Add `Configure`, `Reset`, `EmitNotice`, `EmitConfirmRequest`, `EmitConfirmResult`, `OnError`, `OnStuckTools`, `EmitSystemEvent`, `MemberToolCalls`, enhanced `OnTurnEnd`, `Close`; add compile-time `var _ biz.ActivityEmitter` check |
| `internal/agent/v2/projector_test.go` | New file: unit tests for ActivityEmitter methods |
| `internal/agent/stream_consumer.go` | Remove v1 dual-path; use v2 for all processing; update `finalize()` to call v2 methods |
| `internal/service/chat_orchestrator_turn_phases.go` | Replace v1 projector creation with v2 `Configure` + `WithActivityEmitter`; remove type assertion |
| `internal/team/runner_team_trpc.go` | Replace v1 projector with v2 `Configure` + `WithActivityEmitter` |
| `internal/chatactivity/stream_options.go` | Remove v1 `ActivityProjector` field; keep v2 projector |
| `internal/agent/stream_consumer.go` | Remove v1 `ActivityProjector` from `ChatStreamConsumeOptions` |
| `cmd/admin/wire.go` | Remove v1 `ActivityProjector` provider (if any); keep v2 |
| `web/src/features/chat/v2Types.ts` | Add `NoticeType` field to `Step` interface |

### Deleted Files

| File | Lines | Reason |
|------|-------|--------|
| `internal/agent/activity_projector.go` | 1841 | Replaced by v2 projector |
| `internal/agent/activity_event_sequencer.go` | ~500 | v1 sequencer, replaced by v2 Sequencer |
| `internal/agent/activity_projector_test.go` | ~800 | Tests for deleted v1 projector (if exists) |

---

## Tier 1: ActivityEmitter Interface (unblocks plugins)

### Task 1: Add NoticeType field to biz.Step

**Files:**
- Modify: `internal/biz/step.go`

- [ ] **Step 1: Add NoticeType field to Step struct**

In `internal/biz/step.go`, add `NoticeType string` field to the `Step` struct (after `ToolErrorCode`):

```go
type Step struct {
	ID               string
	TurnID           string
	TaskID           string
	SessionID        string
	SpiritSessionID  string
	Kind             StepKind
	AuthorAgentKey   string
	Seq              int64
	Version          int64
	Content          string
	Reasoning        string
	ToolName         string
	ToolCallID       string
	ToolArgs         json.RawMessage
	ToolResult       json.RawMessage
	ToolDurationMs   int64
	ToolErrorCode    string
	NoticeType       string // kind=notice: notification type (e.g. "model_router", "cost_guard")
	Status           StepStatus
	IsFinal          bool
	StartedAt        time.Time
	CompletedAt      *time.Time
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/biz/...`
Expected: PASS (field addition is backward-compatible)

- [ ] **Step 3: Commit**

```bash
git add internal/biz/step.go
git commit -m "refactor(biz): add NoticeType field to Step for v2 notice events"
```

---

### Task 2: Add Configure + Reset to v2 projector

**Files:**
- Modify: `internal/agent/v2/projector.go`

- [ ] **Step 1: Add Configure and Reset methods**

Add after the `OnTurnStart` method in `internal/agent/v2/projector.go`:

```go
// Configure sets the ProjectMeta for the current turn WITHOUT emitting events.
// Used by chat_orchestrator to pre-configure the projector before LLM invocation
// so that plugins/hooks can emit notice/confirm events during the call.
// OnTurnStart (called later by the stream consumer) will emit task.created and
// turn.started events and reset per-turn streaming state.
func (p *ActivityProjector) Configure(meta ProjectMeta) {
	p.meta = meta
}

// Reset clears per-turn state. Called when the projector is reused across turns.
// OnTurnStart also resets state, so Reset is mainly for explicit cleanup.
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
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/agent/v2/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/agent/v2/projector.go
git commit -m "feat(agent/v2): add Configure and Reset methods to ActivityProjector"
```

---

### Task 3: Implement EmitNotice on v2 projector

**Files:**
- Modify: `internal/agent/v2/projector.go`

- [ ] **Step 1: Write the failing test**

Create `internal/agent/v2/projector_test.go`:

```go
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

func TestEmitNotice(t *testing.T) {
	capture := &capturingSequencer{}
	p := NewActivityProjector(capture, nil, nil)
	p.OnTurnStart(context.Background(), ProjectMeta{
		TaskID:          "task-1",
		TurnID:          "turn-1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		AgentKey:        "agent-1",
	})

	err := p.EmitNotice(context.Background(), "model switched to gpt-4", "model_router")
	if err != nil {
		t.Fatalf("EmitNotice returned error: %v", err)
	}

	// Expect 2 events: StepCreatedEvent + StepCompletedEvent
	// (OnTurnStart also emits TaskCreatedEvent + TurnStartedEvent, so total = 4)
	if len(capture.events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(capture.events))
	}

	// Event[2] should be StepCreatedEvent (notice)
	created, ok := capture.events[2].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected StepCreatedEvent, got %T", capture.events[2])
	}
	if created.Step.Kind != biz.StepKindNotice {
		t.Errorf("expected kind=notice, got %s", created.Step.Kind)
	}
	if created.Step.Content != "model switched to gpt-4" {
		t.Errorf("expected content, got %s", created.Step.Content)
	}
	if created.Step.NoticeType != "model_router" {
		t.Errorf("expected noticeType=model_router, got %s", created.Step.NoticeType)
	}

	// Event[3] should be StepCompletedEvent (notice immediately completed)
	completed, ok := capture.events[3].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[3])
	}
	if completed.Step.Status != biz.StepStatusCompleted {
		t.Errorf("expected status=completed, got %s", completed.Step.Status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/v2/ -run TestEmitNotice -count=1`
Expected: FAIL with "EmitNotice undefined" or compile error

- [ ] **Step 3: Implement EmitNotice**

Add to `internal/agent/v2/projector.go`:

```go
// EmitNotice implements biz.ActivityEmitter. Creates a notice step and
// immediately completes it. The step carries NoticeType metadata for
// frontend rendering.
func (p *ActivityProjector) EmitNotice(ctx context.Context, content, noticeType string) error {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return nil
	}
	stepID := p.BeginStep(p.meta, biz.StepKindNotice)
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.NoticeType = noticeType
	}
	p.mu.Unlock()
	p.completeStep(ctx, stepID, content, nil)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/v2/ -run TestEmitNotice -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/v2/projector.go internal/agent/v2/projector_test.go
git commit -m "feat(agent/v2): implement EmitNotice for ActivityEmitter interface"
```

---

### Task 4: Implement EmitConfirmRequest on v2 projector

**Files:**
- Modify: `internal/agent/v2/projector.go`
- Modify: `internal/agent/v2/projector_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/agent/v2/projector_test.go`:

```go
func TestEmitConfirmRequest(t *testing.T) {
	capture := &capturingSequencer{}
	p := NewActivityProjector(capture, nil, nil)
	p.OnTurnStart(context.Background(), ProjectMeta{
		TaskID:          "task-1",
		TurnID:          "turn-1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		AgentKey:        "agent-1",
	})

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

	// OnTurnStart emits 2 events (TaskCreated + TurnStarted).
	// EmitConfirmRequest emits 2 events (StepCreated + StepUpdated).
	if len(capture.events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(capture.events))
	}

	// Event[2] should be StepCreatedEvent (confirm, status=pending)
	created, ok := capture.events[2].(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("expected StepCreatedEvent, got %T", capture.events[2])
	}
	if created.Step.Kind != biz.StepKindConfirm {
		t.Errorf("expected kind=confirm, got %s", created.Step.Kind)
	}
	if created.Step.Status != biz.StepStatusPending {
		t.Errorf("expected status=pending, got %s", created.Step.Status)
	}

	// Event[3] should be StepUpdatedEvent (status=tool_blocked)
	updated, ok := capture.events[3].(*biz.StepUpdatedEvent)
	if !ok {
		t.Fatalf("expected StepUpdatedEvent, got %T", capture.events[3])
	}
	if updated.Step.Status != biz.StepStatusToolBlocked {
		t.Errorf("expected status=tool_blocked, got %s", updated.Step.Status)
	}
	if updated.Step.ToolName != "shell" {
		t.Errorf("expected toolName=shell, got %s", updated.Step.ToolName)
	}
	if updated.Step.Content != "Allow shell execution?" {
		t.Errorf("expected content, got %s", updated.Step.Content)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/v2/ -run TestEmitConfirmRequest -count=1`
Expected: FAIL (method not defined)

- [ ] **Step 3: Implement EmitConfirmRequest**

Add to `internal/agent/v2/projector.go`:

```go
// EmitConfirmRequest implements biz.ActivityEmitter. Creates a confirm step
// with status=tool_blocked and returns the step ID for later result correlation.
// The step stays in tool_blocked until EmitConfirmResult is called.
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/v2/ -run TestEmitConfirmRequest -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/v2/projector.go internal/agent/v2/projector_test.go
git commit -m "feat(agent/v2): implement EmitConfirmRequest for ActivityEmitter interface"
```

---

### Task 5: Implement EmitConfirmResult on v2 projector

**Files:**
- Modify: `internal/agent/v2/projector.go`
- Modify: `internal/agent/v2/projector_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/agent/v2/projector_test.go`:

```go
func TestEmitConfirmResult(t *testing.T) {
	capture := &capturingSequencer{}
	p := NewActivityProjector(capture, nil, nil)
	p.OnTurnStart(context.Background(), ProjectMeta{
		TaskID:          "task-1",
		TurnID:          "turn-1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		AgentKey:        "agent-1",
	})

	stepID, _ := p.EmitConfirmRequest(context.Background(), biz.ActivityConfirmParams{
		ToolName: "shell",
		Content:  "Allow?",
	})

	// Reset capture to only see result events
	capture.events = nil

	// Test approved
	err := p.EmitConfirmResult(context.Background(), stepID, true)
	if err != nil {
		t.Fatalf("EmitConfirmResult returned error: %v", err)
	}
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

	// Test denied (need a new confirm step)
	capture.events = nil
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
		t.Fatalf("expected 1 event, got %d", len(capture.events))
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/v2/ -run TestEmitConfirmResult -count=1`
Expected: FAIL (method not defined)

- [ ] **Step 3: Implement EmitConfirmResult**

Add to `internal/agent/v2/projector.go`:

```go
// EmitConfirmResult implements biz.ActivityEmitter. Updates a confirm step
// with the user's response: approved → completed, denied → cancelled.
// Returns an error if the step ID is not found or not a confirm step.
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
```

Also add `"fmt"` to the imports if not already present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/v2/ -run TestEmitConfirmResult -count=1`
Expected: PASS

- [ ] **Step 5: Add compile-time interface check**

Add at the end of `internal/agent/v2/projector.go`:

```go
// compile-time interface check
var _ biz.ActivityEmitter = (*ActivityProjector)(nil)
```

- [ ] **Step 6: Verify all tests pass**

Run: `go test ./internal/agent/v2/ -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/agent/v2/projector.go internal/agent/v2/projector_test.go
git commit -m "feat(agent/v2): implement EmitConfirmResult + compile-time ActivityEmitter check"
```

---

## Tier 2: v2 ProcessEvent Extension (unblocks v1 ProcessEvent removal)

### Task 6: Add OnError to v2 projector

**Files:**
- Modify: `internal/agent/v2/projector.go`
- Modify: `internal/agent/v2/projector_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/agent/v2/projector_test.go`:

```go
func TestOnError(t *testing.T) {
	capture := &capturingSequencer{}
	p := NewActivityProjector(capture, nil, nil)
	p.OnTurnStart(context.Background(), ProjectMeta{
		TaskID:          "task-1",
		TurnID:          "turn-1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		AgentKey:        "agent-1",
	})
	capture.events = nil // Only care about error events

	p.OnError(context.Background(), "LLM call failed", "rate_limit", "429")

	// Expect 1 event: TaskFailedEvent
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(capture.events))
	}
	failed, ok := capture.events[0].(*biz.TaskFailedEvent)
	if !ok {
		t.Fatalf("expected TaskFailedEvent, got %T", capture.events[0])
	}
	if failed.Task.Status != biz.TaskStatusFailed {
		t.Errorf("expected status=failed, got %s", failed.Task.Status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/v2/ -run TestOnError -count=1`
Expected: FAIL (method not defined)

- [ ] **Step 3: Implement OnError**

Add to `internal/agent/v2/projector.go`:

```go
// OnError marks the root task as failed and emits a TaskFailedEvent.
// If no root task exists (error before OnTurnStart), the error is logged
// but no event is emitted (the stream consumer handles pre-turn errors
// separately).
func (p *ActivityProjector) OnError(ctx context.Context, errMsg, errType, errCode string) {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return
	}
	now := time.Now()
	p.mu.Lock()
	task, ok := p.activeTask[p.meta.TaskID]
	if ok {
		task.Status = biz.TaskStatusFailed
		task.ErrorMessage = errMsg
		task.ErrorType = errType
		task.CompletedAt = &now
		task.Version++
		delete(p.activeTask, p.meta.TaskID)
	}
	p.mu.Unlock()
	if ok {
		p.seq.Publish(ctx, biz.NewTaskFailedEvent(*task))
	}
}
```

Note: Check that `biz.Task` has `ErrorMessage` and `ErrorType` fields. If not, use the existing fields or add them. Verify by reading `internal/biz/task.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/v2/ -run TestOnError -count=1`
Expected: PASS (adjust field names if Task struct differs)

- [ ] **Step 5: Commit**

```bash
git add internal/agent/v2/projector.go internal/agent/v2/projector_test.go
git commit -m "feat(agent/v2): add OnError to mark root task as failed"
```

---

### Task 7: Extend v2 ProcessEvent with error routing

**Files:**
- Modify: `internal/agent/v2/projector.go`

- [ ] **Step 1: Update ProcessEvent to route errors to OnError**

In `internal/agent/v2/projector.go`, find the `ProcessEvent` method. Replace the error logging block (lines ~347-352) with a call to `OnError`:

```go
// Old code (replace):
if ev.Response.Error != nil && ev.Response.Object != trpcmodel.ObjectTypeToolResponse {
    p.lg.Warn("projector_v2: error event (no v2 OnError callback)",
        loggateway.Str("type", ev.Response.Error.Type),
        loggateway.Str("msg", ev.Response.Error.Message))
    return
}

// New code:
if ev.Response.Error != nil && ev.Response.Object != trpcmodel.ObjectTypeToolResponse {
    errType := ev.Response.Error.Type
    if errType == "" {
        errType = "run_error"
    }
    p.OnError(ctx, ev.Response.Error.Message, errType, "")
    return
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/agent/v2/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/agent/v2/projector.go
git commit -m "feat(agent/v2): route error events to OnError in ProcessEvent"
```

---

### Task 8: Add OnStuckTools to v2 projector

**Files:**
- Modify: `internal/agent/v2/projector.go`
- Modify: `internal/agent/v2/projector_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/agent/v2/projector_test.go`:

```go
func TestOnStuckTools(t *testing.T) {
	capture := &capturingSequencer{}
	p := NewActivityProjector(capture, nil, nil)
	p.OnTurnStart(context.Background(), ProjectMeta{
		TaskID:          "task-1",
		TurnID:          "turn-1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		AgentKey:        "agent-1",
	})

	// Create an action step that stays in tool_running (no OnToolResult)
	stepID := p.BeginStep(p.meta, biz.StepKindAction)
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.Status = biz.StepStatusToolRunning
		step.ToolName = "shell"
	}
	p.mu.Unlock()
	capture.events = nil

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
	if failed.Step.ToolErrorCode == "" {
		t.Error("expected non-empty ToolErrorCode for stuck tool")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/v2/ -run TestOnStuckTools -count=1`
Expected: FAIL (method not defined)

- [ ] **Step 3: Implement OnStuckTools**

Add to `internal/agent/v2/projector.go`. Also add a stuck tool error code constant:

```go
const stuckToolErrorCode = "tool_timeout"

// OnStuckTools marks all tool_running steps as failed. Called from
// stream_consumer.finalize() when the turn ends with pending tool calls.
func (p *ActivityProjector) OnStuckTools(ctx context.Context) {
	if p == nil || p.seq == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	var stuck []*biz.Step
	for id, step := range p.activeStep {
		if step.Status == biz.StepStatusToolRunning {
			step.Status = biz.StepStatusFailed
			step.CompletedAt = &now
			step.ToolErrorCode = stuckToolErrorCode
			step.Version++
			stuck = append(stuck, step)
			delete(p.activeStep, id)
		}
	}
	p.mu.Unlock()
	for _, step := range stuck {
		p.lg.Warn("stuck tool detected at turn finalization",
			loggateway.StepID("agent.v2.projector.stuck_tool"),
			loggateway.Str("step_id", step.ID),
			loggateway.Str("tool_name", step.ToolName),
		)
		p.seq.Publish(ctx, biz.NewStepFailedEvent(*step))
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/v2/ -run TestOnStuckTools -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/v2/projector.go internal/agent/v2/projector_test.go
git commit -m "feat(agent/v2): add OnStuckTools to mark stuck tool steps as failed"
```

---

### Task 9: Add EmitSystemEvent to v2 projector

**Files:**
- Modify: `internal/agent/v2/projector.go`

- [ ] **Step 1: Implement EmitSystemEvent**

The v1 `EmitSystemEvent` creates a transient Activity (not persisted, only WS-broadcast). In v2, this maps to creating a notice step with the system event metadata. Add to `internal/agent/v2/projector.go`:

```go
// EmitSystemEvent emits a notice step for system-level notifications
// (e.g. context_usage token counts). The step is created and immediately
// completed, carrying the provided metadata in NoticeType + Content.
// The kind parameter is accepted for v1 compatibility but v2 always
// uses StepKindNotice (v1 used biz.ActivityKindNotice).
func (p *ActivityProjector) EmitSystemEvent(ctx context.Context, kind biz.ActivityKind, content string, meta map[string]any) {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return
	}
	noticeType := content
	if t, ok := meta["type"].(string); ok && t != "" {
		noticeType = t
	}
	// Serialize meta to content for the notice step
	contentText := content
	if content == "" {
		if data, err := json.Marshal(meta); err == nil {
			contentText = string(data)
		}
	}
	_ = p.EmitNotice(ctx, contentText, noticeType)
}
```

Note: The v1 `EmitSystemEvent` uses `biz.ActivityKindNotice` as the kind parameter in the stream_consumer call. The v2 version always creates a notice step. The `kind` parameter is accepted but ignored (v1 only ever passes `biz.ActivityKindNotice`).

- [ ] **Step 2: Verify build**

Run: `go build ./internal/agent/v2/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/agent/v2/projector.go
git commit -m "feat(agent/v2): add EmitSystemEvent for context_usage notifications"
```

---

### Task 10: Add MemberToolCalls + Close to v2 projector

**Files:**
- Modify: `internal/agent/v2/projector.go`

- [ ] **Step 1: Add memberToolCalls tracking field + MemberToolCalls accessor + Close**

In `internal/agent/v2/projector.go`, add `memberToolCalls` field to the struct:

```go
type ActivityProjector struct {
	// ... existing fields ...
	memberToolCalls map[string]int // agentKey → tool call count
}
```

Update `NewActivityProjector` to initialize:
```go
memberToolCalls: make(map[string]int),
```

Update `OnToolCall` to increment member tool call counts (after setting step fields):
```go
// After the existing step update in OnToolCall, track member tool calls:
if meta.TeamStageID != "" && meta.AgentKey != "" {
	p.mu.Lock()
	p.memberToolCalls[meta.AgentKey]++
	p.mu.Unlock()
}
```

Add the accessor and Close methods:

```go
// MemberToolCalls returns per-member tool call counts observed during the
// turn. Used by stream_consumer for team run step persistence.
func (p *ActivityProjector) MemberToolCalls() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.memberToolCalls) == 0 {
		return nil
	}
	out := make(map[string]int, len(p.memberToolCalls))
	for k, v := range p.memberToolCalls {
		out[k] = v
	}
	return out
}

// Close is a no-op for the singleton v2 projector. Per-turn cleanup is
// handled by OnTurnEnd and Reset. The v2 Sequencer is a singleton and
// must NOT be closed per-turn (it would break other concurrent turns).
func (p *ActivityProjector) Close() {
	// no-op: sequencer is a singleton, not closed per-turn
}
```

- [ ] **Step 2: Update OnTurnStart to reset memberToolCalls**

In `OnTurnStart`, add reset:
```go
p.memberToolCalls = make(map[string]int)
```

- [ ] **Step 3: Verify build + tests**

Run: `go test ./internal/agent/v2/ -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/agent/v2/projector.go
git commit -m "feat(agent/v2): add MemberToolCalls tracking + Close no-op"
```

---

### Task 11: Enhance v2 OnTurnEnd with stuck tools + usage + canceled

**Files:**
- Modify: `internal/agent/v2/projector.go`
- Modify: `internal/agent/v2/projector_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/agent/v2/projector_test.go`:

```go
func TestOnTurnEndEnhanced(t *testing.T) {
	capture := &capturingSequencer{}
	p := NewActivityProjector(capture, nil, nil)
	p.OnTurnStart(context.Background(), ProjectMeta{
		TaskID:          "task-1",
		TurnID:          "turn-1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		AgentKey:        "agent-1",
	})
	capture.events = nil

	usage := &ActivityUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}
	p.OnTurnEndEnhanced(context.Background(), p.meta, usage, false)

	// Expect 2 events: TurnCompletedEvent + TaskCompletedEvent
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/v2/ -run TestOnTurnEndEnhanced -count=1`
Expected: FAIL (method not defined)

- [ ] **Step 3: Implement OnTurnEndEnhanced**

Add `ActivityUsage` type and `OnTurnEndEnhanced` method to `internal/agent/v2/projector.go`:

```go
// ActivityUsage holds token usage for a turn. Mirrors agent.ActivityUsage.
type ActivityUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// OnTurnEndEnhanced is the v1-compatible OnTurnEnd that also handles stuck
// tools, token usage, and cancellation. This is the method the stream_consumer
// should call instead of OnTurnEnd.
func (p *ActivityProjector) OnTurnEndEnhanced(ctx context.Context, meta ProjectMeta, usage *ActivityUsage, canceled bool) {
	if p == nil || p.seq == nil {
		return
	}
	// 1. Mark stuck tools as failed (before finalizing turn)
	p.OnStuckTools(ctx)

	// 2. Finalize remaining active steps with terminal status
	now := time.Now()
	terminalStatus := biz.StepStatusCompleted
	if canceled {
		terminalStatus = biz.StepStatusCancelled
	}
	p.mu.Lock()
	var remaining []*biz.Step
	for id, step := range p.activeStep {
		step.Status = terminalStatus
		step.CompletedAt = &now
		step.Version++
		remaining = append(remaining, step)
		delete(p.activeStep, id)
	}
	p.mu.Unlock()
	for _, step := range remaining {
		p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
	}

	// 3. Emit turn.completed
	p.OnTurnEnd(ctx, meta)

	// 4. Attach usage to task (if root turn and usage provided)
	if usage != nil && meta.TeamStageID == "" {
		// Task was already completed by OnTurnEnd; emit an updated event
		// with usage fields. The repo's UpsertTask will merge the fields.
		p.mu.Lock()
		task, ok := p.activeTask[meta.TaskID]
		if ok {
			task.PromptTokens = int64(usage.PromptTokens)
			task.CompletionTokens = int64(usage.CompletionTokens)
			task.Version++
			delete(p.activeTask, meta.TaskID)
		}
		p.mu.Unlock()
		// Note: OnTurnEnd already deleted the task from activeTask and
		// emitted TaskCompletedEvent. If we need usage on the task, we
		// should modify OnTurnEnd to accept usage. For now, usage is
		// attached to the EventStreamResult by the stream_consumer,
		// not to the Task entity.
	}
}
```

Note: The `biz.Task` struct may need `PromptTokens`/`CompletionTokens` fields. Check `internal/biz/task.go` and add if missing. If the task already carries these fields, the usage attachment works. If not, skip the usage attachment for now — the stream_consumer already attaches usage to `EventStreamResult`, which is consumed by the team runner for persistence.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/v2/ -run TestOnTurnEndEnhanced -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/v2/projector.go internal/agent/v2/projector_test.go
git commit -m "feat(agent/v2): add OnTurnEndEnhanced with stuck tools + usage + canceled"
```

---

## Tier 3: Wire v2 + Remove v1 Dual-Path

### Task 12: Update chat_orchestrator to use v2 projector

**Files:**
- Modify: `internal/service/chat_orchestrator_turn_phases.go`

- [ ] **Step 1: Replace v1 projector creation with v2 Configure + WithActivityEmitter**

In `internal/service/chat_orchestrator_turn_phases.go`, find the block around lines 209-227 that creates the v1 projector. Replace with v2 configuration:

```go
// OLD CODE (replace):
if o.activityUpserter() != nil && o.td().Pipeline.ActivityBus != nil {
    categorizer := chatagent.NewToolCategorizerFromCatalog(ctx, o.td().ReadDeps.Tools)
    ap := chatagent.NewActivityProjector(o.td().Pipeline.ActivityBus, o.activityUpserter(), o.lg(), categorizer)
    ap.Reset()
    earlyMeta := chatagent.ProjectMeta{
        SessionID:        sessionID,
        RequestID:        userMsg.ID,
        InvocationID:     admit.runID,
        RunID:            admit.runID,
        TraceID:          emitter.TraceID(),
        AgentID:          ag.ID,
        AgentDisplayName: ag.DisplayName,
        ContextWindow:    o.resolveContextWindowTokens(runCtx, sess, ag, admit.provider, admit.model),
        Source:           event.EnvelopeSourceFromContext(runCtx),
        TaskContent:      content,
    }
    runCtx = ap.OnTurnStart(runCtx, earlyMeta)
    runCtx = biz.WithActivityEmitter(runCtx, ap)
}

// NEW CODE:
if o.infraDeps.V2Projector != nil {
    earlyMeta := chatagent.ProjectMeta{
        SessionID:        sessionID,
        RequestID:        userMsg.ID,
        InvocationID:     admit.runID,
        RunID:            admit.runID,
        TraceID:          emitter.TraceID(),
        AgentID:          ag.ID,
        AgentDisplayName: ag.DisplayName,
        ContextWindow:    o.resolveContextWindowTokens(runCtx, sess, ag, admit.provider, admit.model),
        Source:           event.EnvelopeSourceFromContext(runCtx),
        TaskContent:      content,
    }
    v2Meta := chatagent.V2ProjectMetaFromV1(earlyMeta)
    o.infraDeps.V2Projector.Configure(v2Meta)
    runCtx = biz.WithActivityEmitter(runCtx, o.infraDeps.V2Projector)
}
```

Note: `V2ProjectMetaFromV1` is already defined in `internal/agent/stream_consumer.go` as `v2ProjectMetaFromV1`. It may need to be exported or duplicated. Check visibility.

- [ ] **Step 2: Remove the v1 type assertion at line 534**

Find the block around lines 533-537:
```go
// OLD CODE (replace):
if emitter := biz.ActivityEmitterFromContext(runCtx); emitter != nil {
    if p, ok := emitter.(*chatagent.ActivityProjector); ok {
        streamOpts.ActivityProjector = p
    }
}

// NEW CODE: (remove entirely — v2 projector is already wired via V2Projector)
```

- [ ] **Step 3: Verify build**

Run: `go build ./internal/service/...`
Expected: PASS (may need to fix unused imports)

- [ ] **Step 4: Commit**

```bash
git add internal/service/chat_orchestrator_turn_phases.go
git commit -m "refactor(service): replace v1 projector with v2 Configure + WithActivityEmitter"
```

---

### Task 13: Update team runner to use v2 projector

**Files:**
- Modify: `internal/team/runner_team_trpc.go`

- [ ] **Step 1: Replace v1 projector with v2 Configure + WithActivityEmitter**

In `internal/team/runner_team_trpc.go` around line 180, find where the v1 projector is created and injected. Replace with v2 configuration:

```go
// OLD CODE (replace):
runCtx = biz.WithActivityEmitter(runCtx, streamOpts.ActivityProjector)

// NEW CODE:
if v2Projector != nil {
    v2Meta := /* construct v2.ProjectMeta from team context */
    v2Projector.Configure(v2Meta)
    runCtx = biz.WithActivityEmitter(runCtx, v2Projector)
}
```

Note: The exact code depends on the current team runner implementation. Read the file around line 180 to understand the context and the available `v2Projector` variable.

- [ ] **Step 2: Verify build**

Run: `go build ./internal/team/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/team/runner_team_trpc.go
git commit -m "refactor(team): replace v1 projector with v2 Configure + WithActivityEmitter"
```

---

### Task 14: Remove v1 dual-path from stream_consumer

**Files:**
- Modify: `internal/agent/stream_consumer.go`
- Modify: `internal/agent/stream_options.go`

- [ ] **Step 1: Remove v1 ActivityProjector from ChatStreamConsumeOptions**

In `internal/agent/stream_options.go` (or wherever `ChatStreamConsumeOptions` is defined), remove the `ActivityProjector` field:

```go
// Remove this field:
ActivityProjector *ActivityProjector
```

Keep the `V2Projector` field.

- [ ] **Step 2: Update projectAndTrackTools to use only v2**

In `internal/agent/stream_consumer.go`, update `projectAndTrackTools`:

```go
// OLD CODE:
func (c *turnStreamConsumer) projectAndTrackTools(ev *trpcevent.Event) {
    if c.opts != nil && c.opts.ActivityProjector != nil {
        c.opts.ActivityProjector.ProcessEvent(c.turnCtx, ev)
    }
    if c.v2Enabled.Load() && c.v2Projector != nil {
        c.v2Projector.ProcessEvent(c.turnCtx, ev)
    }
}

// NEW CODE:
func (c *turnStreamConsumer) projectAndTrackTools(ev *trpcevent.Event) {
    if c.v2Projector != nil {
        c.v2Projector.ProcessEvent(c.turnCtx, ev)
    }
}
```

- [ ] **Step 3: Update publishContextUsageStep to use v2**

```go
// OLD CODE:
c.opts.ActivityProjector.EmitSystemEvent(c.turnCtx, biz.ActivityKindNotice, "context_usage", meta)

// NEW CODE:
c.v2Projector.EmitSystemEvent(c.turnCtx, biz.ActivityKindNotice, "context_usage", meta)
```

- [ ] **Step 4: Update finalize() to use v2 only**

```go
// OLD CODE:
func (c *turnStreamConsumer) finalize() {
    if c.opts != nil && c.opts.ActivityProjector != nil {
        usage := &ActivityUsage{...}
        c.opts.ActivityProjector.OnStuckTools(c.turnCtx)
        c.opts.ActivityProjector.OnTurnEnd(c.turnCtx, usage, c.canceled)
        c.opts.ActivityProjector.Close()
        if mtc := c.opts.ActivityProjector.MemberToolCalls(); len(mtc) > 0 {
            c.result.MemberToolCalls = mtc
        }
    }
    if c.v2Enabled.Load() && c.v2Projector != nil {
        v2Meta := v2ProjectMetaFromV1(c.projectMeta)
        c.v2Projector.OnTurnEnd(c.turnCtx, v2Meta)
    }
}

// NEW CODE:
func (c *turnStreamConsumer) finalize() {
    if c.v2Projector != nil {
        v2Meta := v2ProjectMetaFromV1(c.projectMeta)
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
```

- [ ] **Step 5: Remove v1 ActivityUsage type if no longer used**

Search for `ActivityUsage` references — if only used in `finalize()`, remove the v1 type definition from `activity_projector.go` (will be deleted anyway in Task 15).

- [ ] **Step 6: Verify build**

Run: `go build ./internal/agent/... ./internal/service/... ./internal/team/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/agent/stream_consumer.go internal/agent/stream_options.go
git commit -m "refactor(agent): remove v1 dual-path from stream_consumer, use v2 only"
```

---

## Tier 4: Delete v1 + Verify

### Task 15: Delete v1 activity_projector.go + activity_event_sequencer.go

**Files:**
- Delete: `internal/agent/activity_projector.go` (1841 lines)
- Delete: `internal/agent/activity_event_sequencer.go` (~500 lines)
- Delete: `internal/agent/activity_projector_test.go` (if exists, ~800 lines)
- Modify: `internal/agent/stream_options.go` (remove v1 imports/types)

- [ ] **Step 1: Search for remaining references to v1 projector types**

Run: `grep -rn "ActivityProjector\b" internal/ --include="*.go" | grep -v "_test.go" | grep -v "v2/" | grep -v "activity_projector.go"`

Fix any remaining references (e.g., `chatagent.ActivityProjector`, `chatagent.NewActivityProjector`).

- [ ] **Step 2: Delete v1 projector files**

Delete the following files:
- `internal/agent/activity_projector.go`
- `internal/agent/activity_event_sequencer.go`
- `internal/agent/activity_projector_test.go` (if exists)

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: PASS (fix any remaining references)

- [ ] **Step 4: Run all tests**

Run: `go test ./internal/agent/... ./internal/service/... ./internal/team/... -count=1`
Expected: PASS (fix any test failures)

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(agent): delete v1 activity_projector.go + activity_event_sequencer.go (~2300 lines)

v1 ActivityProjector (1841 lines) and activityEventSequencer (~500 lines)
have been fully replaced by the v2 projector (internal/agent/v2/).
The v2 projector now implements biz.ActivityEmitter, handles error/stuck-tool/
system-event routing, and is wired into all call sites (chat_orchestrator,
team runner, stream_consumer)."
```

---

### Task 16: Update frontend v2Types + verify rendering

**Files:**
- Modify: `web/src/features/chat/v2Types.ts`

- [ ] **Step 1: Add NoticeType field to frontend Step interface**

In `web/src/features/chat/v2Types.ts`, add `NoticeType` to the `Step` interface:

```typescript
export interface Step {
  // ... existing fields ...
  NoticeType?: string  // kind=notice: notification type (e.g. "model_router", "cost_guard")
}
```

- [ ] **Step 2: Verify frontend build**

Run: `cd web && pnpm lint && pnpm build`
Expected: PASS

- [ ] **Step 3: Run frontend tests**

Run: `cd web && pnpm test`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd web && git add src/features/chat/v2Types.ts
git commit -m "feat(chat/v2): add NoticeType field to Step interface for notice rendering"
```

---

### Task 17: Full validation

- [ ] **Step 1: Backend build + tests**

Run: `go build ./... && go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 2: Frontend build + tests**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: PASS

- [ ] **Step 3: Wire check (if Wire providers changed)**

Run: `make wire && go build ./cmd/admin`
Expected: PASS (wire_gen.go should have no v1 projector providers)

- [ ] **Step 4: Final commit (if any cleanup needed)**

```bash
git add -A
git commit -m "chore: Phase 3b-C final cleanup"
```

---

## Known Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| v2 projector is a singleton with per-turn state; concurrent turns (team members) may corrupt state | Turns within a session are sequential; team member turns go through separate stream consumers. The `activeStep` map is protected by mutex. This is a pre-existing design, not introduced by this plan. |
| `EmitConfirmResult` lookups in `activeStep` may fail if the step was created in a different turn context | Confirm steps are created and resolved within the same turn (the turn blocks until user responds). The step stays in `activeStep` until resolved. |
| `OnTurnEndEnhanced` emits duplicate events if `OnTurnEnd` was already called | `OnTurnEndEnhanced` calls `OnTurnEnd` internally; do not call both. The stream consumer should only call `OnTurnEndEnhanced`. |
| v1 `EmitSystemEvent` was transient (not persisted); v2 version persists the notice step | This is acceptable — persisted notices survive page refresh, which is an improvement over v1. The DB upsert is idempotent. |
| Frontend `useBlockedStatus` expects `step.Status === 'running'` for confirm, but v2 uses `tool_blocked` | Check `useBlockedStatus.ts` — it already checks `step.Status === 'tool_blocked'` (line 31-33). The confirm step uses `tool_blocked` status, which is correct. |

---

## Phase 3b-D Roadmap (Future)

After Phase 3b-C completes, the remaining v1 cleanup is:

1. **Delete v1 ActivityEventBus** — used by frontend `useSystemEventNotification` for system event routing. Requires migrating system event routing to v2 event pipeline.
2. **Delete v1 Activity Schema** — `activities` table in Ent. Requires migrating all activity reads to v2 entities (Step/Task/Turn/etc.).
3. **Delete v1 ActivityEvent WS subscriber** — `WSV1Subscriber` for `activity_event` channel. Requires verifying all frontend WS consumers use v2 events.
