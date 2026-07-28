// Package biz — v2 Task State Machine (AS-FSM-01, P2-Y1)
//
// # v2 Task State Diagram (biz.Task / TaskStatus — LLM activity ordering 主任务实体)
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Running : start
//	Pending --> Completed : complete
//	Pending --> Failed : fail
//	Pending --> Cancelled : cancel
//	Pending --> Interrupted : interrupt (orphaned recovery)
//	Running --> Completed : complete
//	Running --> Failed : fail
//	Running --> Cancelled : cancel
//	Running --> Interrupted : interrupt (orphaned recovery)
//	Interrupted --> Running : resume (ResumeInterruptedTask CAS)
//	Interrupted --> Completed : complete (recovery placeholder terminalized)
//	Interrupted --> Failed : fail
//	Interrupted --> Cancelled : cancel
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//
// ```
//
// 注意：本文件是 v2 biz.Task（TaskStatus）的唯一权威状态机。graph 执行域的
// GraphTaskStatus 状态机见 task_state_machine.go——两者是不同实体，禁止混用。
package biz

import (
	"sort"

	"aranea-agents/internal/biz/shared"
)

// ── v2 Task Event types ──────────────────────────────────────────────────────

// TaskV2TransitionEvent enumerates the events that drive a v2 biz.Task through
// its lifecycle. Named with the V2 infix to avoid collision with
// TaskTransitionEvent (graph task domain) in the same package.
// Stability:evolving
type TaskV2TransitionEvent string

const (
	TaskV2EventStart    TaskV2TransitionEvent = "start"
	TaskV2EventComplete TaskV2TransitionEvent = "complete"
	TaskV2EventFail     TaskV2TransitionEvent = "fail"
	TaskV2EventCancel   TaskV2TransitionEvent = "cancel"
	// TaskV2EventInterrupt terminalizes an in-flight task on process-restart
	// orphaned recovery (data/v2_recovery_repo.go). Resumable via resume.
	TaskV2EventInterrupt TaskV2TransitionEvent = "interrupt"
	// TaskV2EventResume re-activates an interrupted task
	// (task_v2_repo.ResumeInterruptedTask CAS interrupted → running).
	TaskV2EventResume TaskV2TransitionEvent = "resume"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// taskV2TransitionRules mirrors the persistence-layer guards exactly:
//   - CompleteTaskTerminal CAS accepts {pending, running, interrupted} → terminal
//     (StatusNotIn terminal set; interrupted is a recovery placeholder that may
//     be terminalized).
//   - v2 orphaned recovery CAS accepts {pending, running} → interrupted.
//   - ResumeInterruptedTask CAS accepts interrupted → running.
//
// Terminal states (completed, failed, cancelled) have no outgoing transitions.
var taskV2TransitionRules = []shared.TransitionRule[TaskStatus, TaskV2TransitionEvent]{
	{From: TaskStatusPending, Event: TaskV2EventStart, To: TaskStatusRunning},
	{From: TaskStatusPending, Event: TaskV2EventComplete, To: TaskStatusCompleted},
	{From: TaskStatusPending, Event: TaskV2EventFail, To: TaskStatusFailed},
	{From: TaskStatusPending, Event: TaskV2EventCancel, To: TaskStatusCancelled},
	{From: TaskStatusPending, Event: TaskV2EventInterrupt, To: TaskStatusInterrupted},

	{From: TaskStatusRunning, Event: TaskV2EventComplete, To: TaskStatusCompleted},
	{From: TaskStatusRunning, Event: TaskV2EventFail, To: TaskStatusFailed},
	{From: TaskStatusRunning, Event: TaskV2EventCancel, To: TaskStatusCancelled},
	{From: TaskStatusRunning, Event: TaskV2EventInterrupt, To: TaskStatusInterrupted},

	{From: TaskStatusInterrupted, Event: TaskV2EventResume, To: TaskStatusRunning},
	{From: TaskStatusInterrupted, Event: TaskV2EventComplete, To: TaskStatusCompleted},
	{From: TaskStatusInterrupted, Event: TaskV2EventFail, To: TaskStatusFailed},
	{From: TaskStatusInterrupted, Event: TaskV2EventCancel, To: TaskStatusCancelled},
}

// ── State machine type ───────────────────────────────────────────────────────

// TaskV2StateMachine is the explicit state machine for the v2 biz.Task entity.
// Stability:evolving
type TaskV2StateMachine struct {
	inner *shared.GenericStateMachine[TaskStatus, TaskV2TransitionEvent]
}

// NewTaskV2StateMachine builds the v2 Task state machine from the transition table.
func NewTaskV2StateMachine() *TaskV2StateMachine {
	return &TaskV2StateMachine{inner: shared.NewGenericStateMachine(taskV2TransitionRules)}
}

// Transition validates and returns the target state for (from, event).
func (sm *TaskV2StateMachine) Transition(from TaskStatus, event TaskV2TransitionEvent) (TaskStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *TaskV2StateMachine) CanTransition(from, to TaskStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns the sorted states reachable from the given state.
func (sm *TaskV2StateMachine) ValidTargets(from TaskStatus) []TaskStatus {
	return sm.inner.ValidTargets(from)
}

// ── Package-level conveniences ───────────────────────────────────────────────

var defaultTaskV2SM = NewTaskV2StateMachine()

// TransitionTaskV2Status validates and returns the target state for (from, event).
func TransitionTaskV2Status(from TaskStatus, event TaskV2TransitionEvent) (TaskStatus, error) {
	return defaultTaskV2SM.Transition(from, event)
}

// CanTransitionTaskV2Status reports whether a direct transition from→to is legal.
func CanTransitionTaskV2Status(from, to TaskStatus) bool {
	return defaultTaskV2SM.CanTransition(from, to)
}

// taskV2KnownStateSet is derived from the transition table — the single
// source of truth for "is this a valid v2 task status".
var taskV2KnownStateSet = func() map[TaskStatus]bool {
	m := make(map[TaskStatus]bool, len(taskV2TransitionRules))
	for _, r := range taskV2TransitionRules {
		m[r.From] = true
		m[r.To] = true
	}
	return m
}()

// IsTerminalTaskV2Status reports whether the status is terminal (a KNOWN
// status with no outgoing transitions). Unknown statuses are not terminal.
// interrupted is NOT terminal — it is a resumable recovery placeholder.
func IsTerminalTaskV2Status(s TaskStatus) bool {
	return taskV2KnownStateSet[s] && len(defaultTaskV2SM.ValidTargets(s)) == 0
}

// TerminalTaskV2Statuses returns the sorted terminal status set. Single source
// of truth for persistence-layer CAS guards (e.g. CompleteTaskTerminal's
// StatusNotIn list) — do not hardcode the set at call sites.
func TerminalTaskV2Statuses() []TaskStatus {
	out := make([]TaskStatus, 0, 3)
	for s := range taskV2KnownStateSet {
		if IsTerminalTaskV2Status(s) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
