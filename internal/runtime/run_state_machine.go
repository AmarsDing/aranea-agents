package runtime

import (
	"fmt"
)

// Run status constants for the runtime Run lifecycle state machine.
// These replace the previous inline comment list on RunStatusEntry.Status.
// Stability:internal
const (
	RunStatusIdle         = "idle"
	RunStatusPending      = "pending"
	RunStatusRunning      = "running"
	RunStatusAwaitingUser = "awaiting_user"
	RunStatusPaused       = "paused"
	RunStatusCompleted    = "completed"
	RunStatusFailed       = "failed"
	RunStatusCancelled    = "cancelled"
)

// RunStatusEvent represents an event that can trigger a Run status transition.
// Stability:internal
type RunStatusEvent string

const (
	RunEventStart     RunStatusEvent = "start"
	RunEventRun       RunStatusEvent = "run"
	RunEventAwaitUser RunStatusEvent = "await_user"
	RunEventPause     RunStatusEvent = "pause"
	RunEventComplete  RunStatusEvent = "complete"
	RunEventFail      RunStatusEvent = "fail"
	RunEventCancel    RunStatusEvent = "cancel"
	RunEventReset     RunStatusEvent = "reset"
	RunEventResume    RunStatusEvent = "resume"
)

// runTransitions defines the legal Run status transitions.
// Each (from, event) maps to exactly one target status.
// Stability:internal
var runTransitions = map[string]map[RunStatusEvent]string{
	RunStatusIdle: {
		RunEventStart: RunStatusPending,
	},
	RunStatusPending: {
		RunEventRun:    RunStatusRunning,
		RunEventFail:   RunStatusFailed,
		RunEventCancel: RunStatusCancelled,
	},
	RunStatusRunning: {
		RunEventAwaitUser: RunStatusAwaitingUser,
		RunEventPause:     RunStatusPaused,
		RunEventComplete:  RunStatusCompleted,
		RunEventFail:      RunStatusFailed,
		RunEventCancel:    RunStatusCancelled,
	},
	RunStatusAwaitingUser: {
		RunEventResume:   RunStatusRunning,
		RunEventComplete: RunStatusCompleted,
		RunEventFail:     RunStatusFailed,
		RunEventCancel:   RunStatusCancelled,
	},
	RunStatusPaused: {
		RunEventResume:   RunStatusRunning,
		RunEventCancel:   RunStatusCancelled,
		RunEventFail:     RunStatusFailed,
		RunEventComplete: RunStatusCompleted,
	},
	RunStatusCompleted: {
		RunEventReset: RunStatusIdle,
	},
	RunStatusFailed: {
		RunEventReset: RunStatusIdle,
	},
	RunStatusCancelled: {
		RunEventReset: RunStatusIdle,
	},
}

// runTerminalStatuses lists statuses that cannot be left without an explicit reset.
var runTerminalStatuses = map[string]bool{
	RunStatusCompleted: true,
	RunStatusFailed:    true,
	RunStatusCancelled: true,
}

// IsRunStatusTerminal reports whether the given Run status is terminal.
func IsRunStatusTerminal(status string) bool {
	return runTerminalStatuses[status]
}

// TransitionRunStatus returns the target status for a (from, event) pair.
// It returns an error if the transition is not defined.
// Stability:internal
func TransitionRunStatus(from string, event RunStatusEvent) (string, error) {
	if from == "" {
		from = RunStatusIdle
	}
	events, ok := runTransitions[from]
	if !ok {
		return "", fmt.Errorf("unknown run status %q", from)
	}
	to, ok := events[event]
	if !ok {
		return "", fmt.Errorf("invalid run transition from %q on event %q", from, event)
	}
	return to, nil
}

// ValidateRunStatusTransition returns nil if the transition is legal.
// It is a convenience wrapper for callers that already know the target status.
// Stability:internal
func ValidateRunStatusTransition(from, to string) error {
	if from == "" {
		from = RunStatusIdle
	}
	events, ok := runTransitions[from]
	if !ok {
		return fmt.Errorf("unknown run status %q", from)
	}
	for event, target := range events {
		if target == to {
			_ = event
			return nil
		}
	}
	return fmt.Errorf("invalid run transition from %q to %q", from, to)
}

// RunStatusFromPhase maps a biz.SessionRunPhase-like status string to the
// canonical RunStatus constant when the input is already known to be valid.
// If the input is not a recognized canonical status, it is returned as-is.
// Stability:internal
func RunStatusFromPhase(phase string) string {
	switch phase {
	case RunStatusIdle, RunStatusPending, RunStatusRunning, RunStatusAwaitingUser,
		RunStatusPaused, RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return phase
	default:
		// Preserve unknown values so callers can store framework or legacy statuses.
		return phase
	}
}
