package biz

import (
	"aranea-agents/internal/biz/shared"
	"testing"
)

// ── Compile-time interface checks ────────────────────────────────────────────

var _ shared.StateMachine[PlanStatus, PlanBoardEvent] = (*PlanBoardStateMachine)(nil)
var _ shared.StateMachine[GraphStageStatus, GraphStageEvent] = (*GraphStageStateMachine)(nil)
var _ shared.StateMachine[TeamStageStatus, TeamStageEvent] = (*TeamStageStateMachine)(nil)
var _ shared.StateMachine[TeamRunV2Status, TeamRunV2Event] = (*TeamRunV2StateMachine)(nil)

// ── PlanBoard State Machine Tests ────────────────────────────────────────────

func TestPlanBoardStateMachine_ValidTransitions(t *testing.T) {
	sm := NewPlanBoardStateMachine()

	cases := []struct {
		from  PlanStatus
		event PlanBoardEvent
		want  PlanStatus
	}{
		{PlanStatusPlanning, PlanBoardEventExecute, PlanStatusExecuting},
		{PlanStatusPlanning, PlanBoardEventFailEarly, PlanStatusFailed},
		{PlanStatusExecuting, PlanBoardEventComplete, PlanStatusCompleted},
		{PlanStatusExecuting, PlanBoardEventFail, PlanStatusFailed},
		{PlanStatusExecuting, PlanBoardEventPartial, PlanStatusPartialFailure},
	}

	for _, tc := range cases {
		got, err := sm.Transition(tc.from, tc.event)
		if err != nil {
			t.Errorf("Transition(%q, %q): unexpected error: %v", string(tc.from), string(tc.event), err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%q, %q) = %q, want %q", string(tc.from), string(tc.event), string(got), string(tc.want))
		}
	}
}

func TestPlanBoardStateMachine_InvalidTransitions(t *testing.T) {
	sm := NewPlanBoardStateMachine()

	terminalStates := []PlanStatus{PlanStatusCompleted, PlanStatusFailed, PlanStatusPartialFailure}
	allEvents := []PlanBoardEvent{
		PlanBoardEventExecute, PlanBoardEventFailEarly, PlanBoardEventComplete,
		PlanBoardEventFail, PlanBoardEventPartial,
	}

	for _, state := range terminalStates {
		for _, event := range allEvents {
			got, err := sm.Transition(state, event)
			if err == nil {
				t.Errorf("terminal state %q should reject event %q (got %q)", string(state), string(event), string(got))
			}
		}
	}

	// Planning cannot directly reach completed/partial_failure (must go through executing)
	invalidFromPlanning := []struct {
		event PlanBoardEvent
		name  string
	}{
		{PlanBoardEventComplete, "planning→complete"},
		{PlanBoardEventPartial, "planning→partial"},
	}
	for _, tc := range invalidFromPlanning {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := sm.Transition(PlanStatusPlanning, tc.event); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestIsPlanBoardTerminal(t *testing.T) {
	cases := []struct {
		state PlanStatus
		want  bool
	}{
		{PlanStatusCompleted, true},
		{PlanStatusFailed, true},
		{PlanStatusPartialFailure, true},
		{PlanStatusPlanning, false},
		{PlanStatusExecuting, false},
	}
	for _, tc := range cases {
		got := IsPlanBoardTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsPlanBoardTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── GraphStage State Machine Tests ───────────────────────────────────────────

func TestGraphStageStateMachine_ValidTransitions(t *testing.T) {
	sm := NewGraphStageStateMachine()

	cases := []struct {
		from  GraphStageStatus
		event GraphStageEvent
		want  GraphStageStatus
	}{
		{GraphStageStatusRunning, GraphStageEventComplete, GraphStageStatusCompleted},
		{GraphStageStatusRunning, GraphStageEventFail, GraphStageStatusFailed},
		{GraphStageStatusRunning, GraphStageEventInterrupt, GraphStageStatusInterrupted},
	}

	for _, tc := range cases {
		got, err := sm.Transition(tc.from, tc.event)
		if err != nil {
			t.Errorf("Transition(%q, %q): unexpected error: %v", string(tc.from), string(tc.event), err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%q, %q) = %q, want %q", string(tc.from), string(tc.event), string(got), string(tc.want))
		}
	}
}

func TestGraphStageStateMachine_TerminalStatesNoOutgoing(t *testing.T) {
	sm := NewGraphStageStateMachine()

	terminalStates := []GraphStageStatus{GraphStageStatusCompleted, GraphStageStatusFailed, GraphStageStatusInterrupted}
	allEvents := []GraphStageEvent{GraphStageEventComplete, GraphStageEventFail, GraphStageEventInterrupt}

	for _, state := range terminalStates {
		for _, event := range allEvents {
			_, err := sm.Transition(state, event)
			if err == nil {
				t.Errorf("terminal state %q should reject event %q", string(state), string(event))
			}
		}
	}
}

func TestIsGraphStageTerminal(t *testing.T) {
	cases := []struct {
		state GraphStageStatus
		want  bool
	}{
		{GraphStageStatusCompleted, true},
		{GraphStageStatusFailed, true},
		{GraphStageStatusInterrupted, true},
		{GraphStageStatusRunning, false},
	}
	for _, tc := range cases {
		got := IsGraphStageTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsGraphStageTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── TeamStage State Machine Tests ────────────────────────────────────────────

func TestTeamStageStateMachine_ValidTransitions(t *testing.T) {
	sm := NewTeamStageStateMachine()

	cases := []struct {
		from  TeamStageStatus
		event TeamStageEvent
		want  TeamStageStatus
	}{
		{TeamStageStatusPending, TeamStageEventStart, TeamStageStatusRunning},
		{TeamStageStatusRunning, TeamStageEventComplete, TeamStageStatusCompleted},
		{TeamStageStatusRunning, TeamStageEventFail, TeamStageStatusFailed},
		{TeamStageStatusRunning, TeamStageEventCancel, TeamStageStatusCancelled},
		{TeamStageStatusRunning, TeamStageEventInterrupt, TeamStageStatusWaitingHuman},
		{TeamStageStatusWaitingHuman, TeamStageEventResume, TeamStageStatusRunning},
		{TeamStageStatusWaitingHuman, TeamStageEventCancel, TeamStageStatusCancelled},
		{TeamStageStatusWaitingHuman, TeamStageEventFail, TeamStageStatusFailed},
	}

	for _, tc := range cases {
		got, err := sm.Transition(tc.from, tc.event)
		if err != nil {
			t.Errorf("Transition(%q, %q): unexpected error: %v", string(tc.from), string(tc.event), err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%q, %q) = %q, want %q", string(tc.from), string(tc.event), string(got), string(tc.want))
		}
	}
}

func TestTeamStageStateMachine_InvalidTransitions(t *testing.T) {
	sm := NewTeamStageStateMachine()

	// Terminal states reject all events
	terminalStates := []TeamStageStatus{TeamStageStatusCompleted, TeamStageStatusFailed, TeamStageStatusCancelled}
	allEvents := []TeamStageEvent{
		TeamStageEventStart, TeamStageEventComplete, TeamStageEventFail,
		TeamStageEventCancel, TeamStageEventInterrupt, TeamStageEventResume,
	}
	for _, state := range terminalStates {
		for _, event := range allEvents {
			_, err := sm.Transition(state, event)
			if err == nil {
				t.Errorf("terminal state %q should reject event %q", string(state), string(event))
			}
		}
	}

	// Pending cannot directly reach terminal states (must go through running)
	invalidFromPending := []struct {
		event TeamStageEvent
		name  string
	}{
		{TeamStageEventComplete, "pending→complete"},
		{TeamStageEventFail, "pending→fail"},
		{TeamStageEventCancel, "pending→cancel"},
		{TeamStageEventInterrupt, "pending→interrupt"},
		{TeamStageEventResume, "pending→resume"},
	}
	for _, tc := range invalidFromPending {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := sm.Transition(TeamStageStatusPending, tc.event); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}

	// WaitingHuman cannot complete directly (must resume first)
	if _, err := sm.Transition(TeamStageStatusWaitingHuman, TeamStageEventComplete); err == nil {
		t.Errorf("waiting_human→complete should be invalid (must resume first)")
	}

	// Running cannot resume (only waiting_human can)
	if _, err := sm.Transition(TeamStageStatusRunning, TeamStageEventResume); err == nil {
		t.Errorf("running→resume should be invalid")
	}
}

func TestTeamStageStateMachine_FullLifecycle(t *testing.T) {
	sm := NewTeamStageStateMachine()

	// Pending → Running → WaitingHuman → Running → Completed
	steps := []struct {
		from  TeamStageStatus
		event TeamStageEvent
		want  TeamStageStatus
	}{
		{TeamStageStatusPending, TeamStageEventStart, TeamStageStatusRunning},
		{TeamStageStatusRunning, TeamStageEventInterrupt, TeamStageStatusWaitingHuman},
		{TeamStageStatusWaitingHuman, TeamStageEventResume, TeamStageStatusRunning},
		{TeamStageStatusRunning, TeamStageEventComplete, TeamStageStatusCompleted},
	}

	current := TeamStageStatusPending
	for _, step := range steps {
		got, err := sm.Transition(step.from, step.event)
		if err != nil {
			t.Fatalf("Transition(%q, %q): unexpected error: %v", string(step.from), string(step.event), err)
		}
		if got != step.want {
			t.Fatalf("Transition(%q, %q) = %q, want %q", string(step.from), string(step.event), string(got), string(step.want))
		}
		current = got
	}

	if !IsTeamStageTerminal(current) {
		t.Fatalf("expected terminal state, got %q", string(current))
	}
}

func TestIsTeamStageTerminal(t *testing.T) {
	cases := []struct {
		state TeamStageStatus
		want  bool
	}{
		{TeamStageStatusCompleted, true},
		{TeamStageStatusFailed, true},
		{TeamStageStatusCancelled, true},
		{TeamStageStatusPending, false},
		{TeamStageStatusRunning, false},
		{TeamStageStatusWaitingHuman, false},
	}
	for _, tc := range cases {
		got := IsTeamStageTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsTeamStageTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── TeamRun v2 State Machine Tests ───────────────────────────────────────────

func TestTeamRunV2StateMachine_ValidTransitions(t *testing.T) {
	sm := NewTeamRunV2StateMachine()

	cases := []struct {
		from  TeamRunV2Status
		event TeamRunV2Event
		want  TeamRunV2Status
	}{
		{TeamRunV2StatusRunning, TeamRunV2EventComplete, TeamRunV2StatusCompleted},
		{TeamRunV2StatusRunning, TeamRunV2EventFail, TeamRunV2StatusFailed},
		{TeamRunV2StatusRunning, TeamRunV2EventCancel, TeamRunV2StatusCancelled},
	}

	for _, tc := range cases {
		got, err := sm.Transition(tc.from, tc.event)
		if err != nil {
			t.Errorf("Transition(%q, %q): unexpected error: %v", string(tc.from), string(tc.event), err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%q, %q) = %q, want %q", string(tc.from), string(tc.event), string(got), string(tc.want))
		}
	}
}

func TestTeamRunV2StateMachine_TerminalStatesNoOutgoing(t *testing.T) {
	sm := NewTeamRunV2StateMachine()

	terminalStates := []TeamRunV2Status{TeamRunV2StatusCompleted, TeamRunV2StatusFailed, TeamRunV2StatusCancelled}
	allEvents := []TeamRunV2Event{TeamRunV2EventComplete, TeamRunV2EventFail, TeamRunV2EventCancel}

	for _, state := range terminalStates {
		for _, event := range allEvents {
			_, err := sm.Transition(state, event)
			if err == nil {
				t.Errorf("terminal state %q should reject event %q", string(state), string(event))
			}
		}
	}
}

func TestIsTeamRunV2Terminal(t *testing.T) {
	cases := []struct {
		state TeamRunV2Status
		want  bool
	}{
		{TeamRunV2StatusCompleted, true},
		{TeamRunV2StatusFailed, true},
		{TeamRunV2StatusCancelled, true},
		{TeamRunV2StatusRunning, false},
	}
	for _, tc := range cases {
		got := IsTeamRunV2Terminal(tc.state)
		if got != tc.want {
			t.Errorf("IsTeamRunV2Terminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}
