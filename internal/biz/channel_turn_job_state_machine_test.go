package biz

import (
	"testing"
)

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestChannelTurnJobStateMachine_ValidTransitions(t *testing.T) {
	cases := []struct {
		from  string
		event string
		want  string
	}{
		// accepted →
		{ChannelTurnJobStatusAccepted, JobEventStart, ChannelTurnJobStatusRunning},
		{ChannelTurnJobStatusAccepted, JobEventQueue, ChannelTurnJobStatusQueued},
		{ChannelTurnJobStatusAccepted, JobEventCancel, ChannelTurnJobStatusCancelled},
		{ChannelTurnJobStatusAccepted, JobEventAsyncQueue, ChannelTurnJobStatusAsyncQueued},
		// running →
		{ChannelTurnJobStatusRunning, JobEventComplete, ChannelTurnJobStatusCompleted},
		{ChannelTurnJobStatusRunning, JobEventFail, ChannelTurnJobStatusFailed},
		{ChannelTurnJobStatusRunning, JobEventTimeout, ChannelTurnJobStatusTimeout},
		{ChannelTurnJobStatusRunning, JobEventCancel, ChannelTurnJobStatusCancelled},
		{ChannelTurnJobStatusRunning, JobEventAsyncQueue, ChannelTurnJobStatusAsyncQueued},
		// queued →
		{ChannelTurnJobStatusQueued, JobEventDequeue, ChannelTurnJobStatusRunning},
		{ChannelTurnJobStatusQueued, JobEventCancel, ChannelTurnJobStatusCancelled},
		// async_queued →
		{ChannelTurnJobStatusAsyncQueued, JobEventAsyncStart, ChannelTurnJobStatusRunning},
		{ChannelTurnJobStatusAsyncQueued, JobEventAsyncFail, ChannelTurnJobStatusFailed},
		{ChannelTurnJobStatusAsyncQueued, JobEventAsyncCancel, ChannelTurnJobStatusCancelled},
		{ChannelTurnJobStatusAsyncQueued, JobEventTimeout, ChannelTurnJobStatusTimeout},
	}

	for _, tc := range cases {
		got, err := TransitionChannelTurnJob(tc.from, tc.event)
		if err != nil {
			t.Errorf("TransitionChannelTurnJob(%q, %q): unexpected error: %v", tc.from, tc.event, err)
			continue
		}
		if got != tc.want {
			t.Errorf("TransitionChannelTurnJob(%q, %q) = %q, want %q", tc.from, tc.event, got, tc.want)
		}
	}
}

// ── Invalid transitions ──────────────────────────────────────────────────────

func TestChannelTurnJobStateMachine_InvalidTransitions(t *testing.T) {
	cases := []struct {
		name  string
		from  string
		event string
	}{
		// Terminal states reject all events
		{"completed→complete", ChannelTurnJobStatusCompleted, JobEventComplete},
		{"completed→fail", ChannelTurnJobStatusCompleted, JobEventFail},
		{"completed→cancel", ChannelTurnJobStatusCompleted, JobEventCancel},
		{"completed→start", ChannelTurnJobStatusCompleted, JobEventStart},
		{"failed→complete", ChannelTurnJobStatusFailed, JobEventComplete},
		{"failed→start", ChannelTurnJobStatusFailed, JobEventStart},
		{"failed→cancel", ChannelTurnJobStatusFailed, JobEventCancel},
		{"timeout→complete", ChannelTurnJobStatusTimeout, JobEventComplete},
		{"timeout→start", ChannelTurnJobStatusTimeout, JobEventStart},
		{"timeout→cancel", ChannelTurnJobStatusTimeout, JobEventCancel},
		{"cancelled→complete", ChannelTurnJobStatusCancelled, JobEventComplete},
		{"cancelled→start", ChannelTurnJobStatusCancelled, JobEventStart},
		{"cancelled→fail", ChannelTurnJobStatusCancelled, JobEventFail},

		// accepted cannot complete/fail/timeout directly
		{"accepted→complete", ChannelTurnJobStatusAccepted, JobEventComplete},
		{"accepted→fail", ChannelTurnJobStatusAccepted, JobEventFail},
		{"accepted→timeout", ChannelTurnJobStatusAccepted, JobEventTimeout},
		{"accepted→dequeue", ChannelTurnJobStatusAccepted, JobEventDequeue},

		// running cannot start or dequeue
		{"running→start", ChannelTurnJobStatusRunning, JobEventStart},
		{"running→dequeue", ChannelTurnJobStatusRunning, JobEventDequeue},

		// queued cannot complete/fail/timeout/start/async_queue
		{"queued→complete", ChannelTurnJobStatusQueued, JobEventComplete},
		{"queued→fail", ChannelTurnJobStatusQueued, JobEventFail},
		{"queued→timeout", ChannelTurnJobStatusQueued, JobEventTimeout},
		{"queued→start", ChannelTurnJobStatusQueued, JobEventStart},
		{"queued→async_queue", ChannelTurnJobStatusQueued, JobEventAsyncQueue},

		// async_queued cannot start/dequeue/complete directly
		{"async_queued→start", ChannelTurnJobStatusAsyncQueued, JobEventStart},
		{"async_queued→dequeue", ChannelTurnJobStatusAsyncQueued, JobEventDequeue},
		{"async_queued→complete", ChannelTurnJobStatusAsyncQueued, JobEventComplete},
		{"async_queued→queue", ChannelTurnJobStatusAsyncQueued, JobEventQueue},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TransitionChannelTurnJob(tc.from, tc.event)
			if err == nil {
				t.Errorf("TransitionChannelTurnJob(%q, %q): expected error, got state %q", tc.from, tc.event, got)
			}
		})
	}
}

// ── CanTransition ─────────────────────────────────────────────────────────────

func TestChannelTurnJobStateMachine_CanTransition(t *testing.T) {
	cases := []struct {
		from  string
		event string
		want  bool
	}{
		// Cancellable non-terminal states
		{ChannelTurnJobStatusAccepted, JobEventCancel, true},
		{ChannelTurnJobStatusRunning, JobEventCancel, true},
		{ChannelTurnJobStatusQueued, JobEventCancel, true},
		{ChannelTurnJobStatusAsyncQueued, JobEventAsyncCancel, true},
		// Terminal states cannot cancel
		{ChannelTurnJobStatusCompleted, JobEventCancel, false},
		{ChannelTurnJobStatusFailed, JobEventCancel, false},
		{ChannelTurnJobStatusTimeout, JobEventCancel, false},
		{ChannelTurnJobStatusCancelled, JobEventCancel, false},
		// Running can complete/fail/timeout
		{ChannelTurnJobStatusRunning, JobEventComplete, true},
		{ChannelTurnJobStatusRunning, JobEventFail, true},
		{ChannelTurnJobStatusRunning, JobEventTimeout, true},
		// Accepted cannot complete directly
		{ChannelTurnJobStatusAccepted, JobEventComplete, false},
	}

	for _, tc := range cases {
		got := CanTransitionChannelTurnJob(tc.from, tc.event)
		if got != tc.want {
			t.Errorf("CanTransitionChannelTurnJob(%q, %q) = %v, want %v", tc.from, tc.event, got, tc.want)
		}
	}
}

// ── Unknown status ────────────────────────────────────────────────────────────

func TestChannelTurnJobStateMachine_UnknownStatus(t *testing.T) {
	_, err := TransitionChannelTurnJob("unknown_status", JobEventStart)
	if err == nil {
		t.Error("expected error for unknown status, got nil")
	}
}

// ── Full lifecycle: accepted → running → completed ───────────────────────────

func TestChannelTurnJobStateMachine_FullLifecycle_Sync(t *testing.T) {
	steps := []struct {
		from  string
		event string
		want  string
	}{
		{ChannelTurnJobStatusAccepted, JobEventStart, ChannelTurnJobStatusRunning},
		{ChannelTurnJobStatusRunning, JobEventComplete, ChannelTurnJobStatusCompleted},
	}

	current := ChannelTurnJobStatusAccepted
	for _, step := range steps {
		got, err := TransitionChannelTurnJob(step.from, step.event)
		if err != nil {
			t.Fatalf("TransitionChannelTurnJob(%q, %q): unexpected error: %v", step.from, step.event, err)
		}
		if got != step.want {
			t.Fatalf("TransitionChannelTurnJob(%q, %q) = %q, want %q", step.from, step.event, got, step.want)
		}
		current = got
	}
	if !IsChannelTurnJobTerminalStatus(current) {
		t.Fatalf("expected terminal state, got %q", current)
	}
}

// ── Full lifecycle: accepted → queued → running → completed ──────────────────

func TestChannelTurnJobStateMachine_FullLifecycle_Queued(t *testing.T) {
	steps := []struct {
		from  string
		event string
		want  string
	}{
		{ChannelTurnJobStatusAccepted, JobEventQueue, ChannelTurnJobStatusQueued},
		{ChannelTurnJobStatusQueued, JobEventDequeue, ChannelTurnJobStatusRunning},
		{ChannelTurnJobStatusRunning, JobEventComplete, ChannelTurnJobStatusCompleted},
	}

	current := ChannelTurnJobStatusAccepted
	for _, step := range steps {
		got, err := TransitionChannelTurnJob(step.from, step.event)
		if err != nil {
			t.Fatalf("TransitionChannelTurnJob(%q, %q): unexpected error: %v", step.from, step.event, err)
		}
		if got != step.want {
			t.Fatalf("TransitionChannelTurnJob(%q, %q) = %q, want %q", step.from, step.event, got, step.want)
		}
		current = got
	}
	if !IsChannelTurnJobTerminalStatus(current) {
		t.Fatalf("expected terminal state, got %q", current)
	}
}

// ── Full lifecycle: accepted → async_queued → running → completed ────────────

func TestChannelTurnJobStateMachine_FullLifecycle_Async(t *testing.T) {
	steps := []struct {
		from  string
		event string
		want  string
	}{
		{ChannelTurnJobStatusAccepted, JobEventAsyncQueue, ChannelTurnJobStatusAsyncQueued},
		{ChannelTurnJobStatusAsyncQueued, JobEventAsyncStart, ChannelTurnJobStatusRunning},
		{ChannelTurnJobStatusRunning, JobEventComplete, ChannelTurnJobStatusCompleted},
	}

	current := ChannelTurnJobStatusAccepted
	for _, step := range steps {
		got, err := TransitionChannelTurnJob(step.from, step.event)
		if err != nil {
			t.Fatalf("TransitionChannelTurnJob(%q, %q): unexpected error: %v", step.from, step.event, err)
		}
		if got != step.want {
			t.Fatalf("TransitionChannelTurnJob(%q, %q) = %q, want %q", step.from, step.event, got, step.want)
		}
		current = got
	}
	if !IsChannelTurnJobTerminalStatus(current) {
		t.Fatalf("expected terminal state, got %q", current)
	}
}

// ── Typed state machine direct tests ─────────────────────────────────────────

func TestChannelTurnJobStateMachine_TypedTransition(t *testing.T) {
	sm := NewChannelTurnJobStateMachine()

	to, err := sm.Transition(ChannelTurnJobStateAccepted, ChannelTurnJobEventStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if to != ChannelTurnJobStateRunning {
		t.Errorf("got %q, want %q", to, ChannelTurnJobStateRunning)
	}
}

func TestChannelTurnJobStateMachine_TypedCanTransition(t *testing.T) {
	sm := NewChannelTurnJobStateMachine()

	if !sm.CanTransition(ChannelTurnJobStateAccepted, ChannelTurnJobStateRunning) {
		t.Error("expected accepted→running to be valid")
	}
	if sm.CanTransition(ChannelTurnJobStateCompleted, ChannelTurnJobStateRunning) {
		t.Error("expected completed→running to be invalid")
	}
}

func TestChannelTurnJobStateMachine_TypedValidTargets(t *testing.T) {
	sm := NewChannelTurnJobStateMachine()

	targets := sm.ValidTargets(ChannelTurnJobStateAccepted)
	if len(targets) != 4 {
		t.Errorf("accepted should have 4 valid targets, got %d: %v", len(targets), targets)
	}
}
