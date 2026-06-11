package shared

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// ── Test types ────────────────────────────────────────────────────────────────

type TestState string

const (
	StateIdle      TestState = "idle"
	StateRunning   TestState = "running"
	StateCompleted TestState = "completed"
	StateFailed    TestState = "failed"
	StateCancelled TestState = "cancelled"
)

type TestEvent string

const (
	EventStart  TestEvent = "start"
	EventFinish TestEvent = "finish"
	EventFail   TestEvent = "fail"
	EventCancel TestEvent = "cancel"
	EventRetry  TestEvent = "retry"
)

// ── Guard helpers ─────────────────────────────────────────────────────────────

var guardPass func(context.Context) bool = func(context.Context) bool { return true }
var guardFail func(context.Context) bool = func(context.Context) bool { return false }

// ── Helpers ───────────────────────────────────────────────────────────────────

func newTestMachine(rules []TransitionRule[TestState, TestEvent]) *GenericStateMachine[TestState, TestEvent] {
	return NewGenericStateMachine(rules)
}

func defaultRules() []TransitionRule[TestState, TestEvent] {
	return []TransitionRule[TestState, TestEvent]{
		{From: StateIdle, Event: EventStart, To: StateRunning},
		{From: StateRunning, Event: EventFinish, To: StateCompleted},
		{From: StateRunning, Event: EventFail, To: StateFailed},
		{From: StateRunning, Event: EventCancel, To: StateCancelled},
		{From: StateFailed, Event: EventRetry, To: StateRunning},
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestGenericStateMachine_Transition_ValidTransitions(t *testing.T) {
	sm := newTestMachine(defaultRules())

	tests := []struct {
		from       TestState
		event      TestEvent
		wantTarget TestState
	}{
		{StateIdle, EventStart, StateRunning},
		{StateRunning, EventFinish, StateCompleted},
		{StateRunning, EventFail, StateFailed},
		{StateRunning, EventCancel, StateCancelled},
		{StateFailed, EventRetry, StateRunning},
	}

	for _, tt := range tests {
		got, err := sm.Transition(tt.from, tt.event)
		if err != nil {
			t.Errorf("Transition(%s, %s) unexpected error: %v", tt.from, tt.event, err)
			continue
		}
		if got != tt.wantTarget {
			t.Errorf("Transition(%s, %s) = %s, want %s", tt.from, tt.event, got, tt.wantTarget)
		}
	}
}

func TestGenericStateMachine_Transition_InvalidTransitions(t *testing.T) {
	sm := newTestMachine(defaultRules())

	tests := []struct {
		from  TestState
		event TestEvent
	}{
		{StateIdle, EventFinish},
		{StateIdle, EventFail},
		{StateIdle, EventCancel},
		{StateCompleted, EventStart},
		{StateCompleted, EventFinish},
		{StateCancelled, EventStart},
		{StateRunning, EventStart},
		{StateFailed, EventStart},
	}

	for _, tt := range tests {
		_, err := sm.Transition(tt.from, tt.event)
		if err == nil {
			t.Errorf("Transition(%s, %s) should fail, but succeeded", tt.from, tt.event)
			continue
		}
		if !strings.Contains(err.Error(), "invalid state transition") {
			t.Errorf("Transition(%s, %s) error = %v, want error containing 'invalid state transition'", tt.from, tt.event, err)
		}
		if !strings.Contains(err.Error(), string(tt.from)) || !strings.Contains(err.Error(), string(tt.event)) {
			t.Errorf("Transition(%s, %s) error = %v, want error containing from and event", tt.from, tt.event, err)
		}
	}
}

func TestGenericStateMachine_Transition_GuardPass(t *testing.T) {
	rules := []TransitionRule[TestState, TestEvent]{
		{From: StateIdle, Event: EventStart, To: StateRunning, Guard: guardPass},
	}
	sm := newTestMachine(rules)

	got, err := sm.Transition(StateIdle, EventStart)
	if err != nil {
		t.Fatalf("Transition with passing guard unexpected error: %v", err)
	}
	if got != StateRunning {
		t.Errorf("Transition = %s, want %s", got, StateRunning)
	}
}

func TestGenericStateMachine_Transition_GuardFail(t *testing.T) {
	rules := []TransitionRule[TestState, TestEvent]{
		{From: StateIdle, Event: EventStart, To: StateRunning, Guard: guardFail},
	}
	sm := newTestMachine(rules)

	_, err := sm.Transition(StateIdle, EventStart)
	if err == nil {
		t.Fatal("Transition with failing guard should return error")
	}
	if !strings.Contains(err.Error(), "guard rejected") {
		t.Errorf("error = %v, want error containing 'guard rejected'", err)
	}
}

func TestGenericStateMachine_Transition_GuardNil(t *testing.T) {
	rules := []TransitionRule[TestState, TestEvent]{
		{From: StateIdle, Event: EventStart, To: StateRunning, Guard: nil},
	}
	sm := newTestMachine(rules)

	got, err := sm.Transition(StateIdle, EventStart)
	if err != nil {
		t.Fatalf("Transition with nil guard unexpected error: %v", err)
	}
	if got != StateRunning {
		t.Errorf("Transition = %s, want %s", got, StateRunning)
	}
}

func TestGenericStateMachine_CanTransition(t *testing.T) {
	sm := newTestMachine(defaultRules())

	tests := []struct {
		from TestState
		to   TestState
		want bool
	}{
		{StateIdle, StateRunning, true},
		{StateRunning, StateCompleted, true},
		{StateRunning, StateFailed, true},
		{StateRunning, StateCancelled, true},
		{StateFailed, StateRunning, true},
		// Invalid
		{StateIdle, StateCompleted, false},
		{StateIdle, StateFailed, false},
		{StateCompleted, StateRunning, false},
		{StateCancelled, StateRunning, false},
		{StateRunning, StateIdle, false},
		{StateFailed, StateCompleted, false},
	}

	for _, tt := range tests {
		got := sm.CanTransition(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestGenericStateMachine_ValidTargets_Sorted(t *testing.T) {
	sm := newTestMachine(defaultRules())

	// StateRunning can go to: completed, failed, cancelled (alphabetical order)
	targets := sm.ValidTargets(StateRunning)

	want := []TestState{StateCancelled, StateCompleted, StateFailed}
	if len(targets) != len(want) {
		t.Fatalf("ValidTargets(running) = %v, want %v", targets, want)
	}
	for i, w := range want {
		if targets[i] != w {
			t.Errorf("ValidTargets(running)[%d] = %s, want %s", i, targets[i], w)
		}
	}
}

func TestGenericStateMachine_ValidTargets_SingleTarget(t *testing.T) {
	sm := newTestMachine(defaultRules())

	targets := sm.ValidTargets(StateIdle)
	if len(targets) != 1 || targets[0] != StateRunning {
		t.Errorf("ValidTargets(idle) = %v, want [running]", targets)
	}
}

func TestGenericStateMachine_ValidTargets_NoTransitions(t *testing.T) {
	sm := newTestMachine(defaultRules())

	// Completed has no outgoing transitions
	targets := sm.ValidTargets(StateCompleted)
	if targets != nil {
		t.Errorf("ValidTargets(completed) = %v, want nil", targets)
	}
}

func TestGenericStateMachine_ValidTargets_UnknownState(t *testing.T) {
	sm := newTestMachine(defaultRules())

	targets := sm.ValidTargets(TestState("unknown"))
	if targets != nil {
		t.Errorf("ValidTargets(unknown) = %v, want nil", targets)
	}
}

func TestGenericStateMachine_EmptyStateMachine(t *testing.T) {
	sm := newTestMachine(nil)

	_, err := sm.Transition(StateIdle, EventStart)
	if err == nil {
		t.Error("Transition on empty machine should fail")
	}

	if sm.CanTransition(StateIdle, StateRunning) {
		t.Error("CanTransition on empty machine should return false")
	}

	targets := sm.ValidTargets(StateIdle)
	if targets != nil {
		t.Errorf("ValidTargets on empty machine = %v, want nil", targets)
	}
}

func TestGenericStateMachine_DuplicateRulePanics(t *testing.T) {
	rules := []TransitionRule[TestState, TestEvent]{
		{From: StateIdle, Event: EventStart, To: StateRunning},
		{From: StateIdle, Event: EventStart, To: StateFailed},
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Error("NewGenericStateMachine with duplicate rules should panic")
		}
		if !strings.Contains(fmt.Sprint(r), "duplicate transition rule") {
			t.Errorf("panic message = %v, want 'duplicate transition rule'", r)
		}
	}()

	NewGenericStateMachine(rules)
}

func TestGenericStateMachine_InterfaceCompliance(t *testing.T) {
	var _ StateMachine[TestState, TestEvent] = (*GenericStateMachine[TestState, TestEvent])(nil)
}

func TestGenericStateMachine_SameFromAndToState(t *testing.T) {
	rules := []TransitionRule[TestState, TestEvent]{
		{From: StateRunning, Event: EventRetry, To: StateRunning},
	}
	sm := newTestMachine(rules)

	got, err := sm.Transition(StateRunning, EventRetry)
	if err != nil {
		t.Fatalf("Transition(running, retry) unexpected error: %v", err)
	}
	if got != StateRunning {
		t.Errorf("Transition = %s, want %s", got, StateRunning)
	}
	if !sm.CanTransition(StateRunning, StateRunning) {
		t.Error("CanTransition(running, running) should be true")
	}
}
