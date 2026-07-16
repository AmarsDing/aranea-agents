// Package lifecycle defines the MCP server health/lifecycle state machine (TPM-D-M3).
// Callers should route probe/reconnect/alert status changes through Transition
// instead of writing metadata health_status strings ad hoc.
package lifecycle

import (
	"fmt"
	"strings"
)

// State is a normalized MCP server health lifecycle state.
type State string

const (
	StateUnknown      State = "unknown"
	StateOK           State = "ok"
	StateError        State = "error"
	StateAuthRequired State = "auth_required"
	StateDegraded     State = "degraded"
)

// Event drives a lifecycle transition.
type Event string

const (
	EventProbeOK           Event = "probe_ok"
	EventProbeFail         Event = "probe_fail"
	EventProbeAuthRequired Event = "probe_auth_required"
	EventStale             Event = "stale" // enabled but probe overdue
	EventReset             Event = "reset" // clear to unknown (e.g. config change)
)

// Normalize maps free-form health_status strings to a State.
func Normalize(raw string) State {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "unknown":
		return StateUnknown
	case "ok", "healthy", "active":
		return StateOK
	case "error", "fail", "failed":
		return StateError
	case "auth_required":
		return StateAuthRequired
	case "degraded":
		return StateDegraded
	default:
		return StateUnknown
	}
}

var transitions = map[State]map[Event]State{
	StateUnknown: {
		EventProbeOK:           StateOK,
		EventProbeFail:         StateError,
		EventProbeAuthRequired: StateAuthRequired,
		EventStale:             StateDegraded,
		EventReset:             StateUnknown,
	},
	StateOK: {
		EventProbeOK:           StateOK,
		EventProbeFail:         StateError,
		EventProbeAuthRequired: StateAuthRequired,
		EventStale:             StateDegraded,
		EventReset:             StateUnknown,
	},
	StateError: {
		EventProbeOK:           StateOK,
		EventProbeFail:         StateError,
		EventProbeAuthRequired: StateAuthRequired,
		EventStale:             StateDegraded,
		EventReset:             StateUnknown,
	},
	StateAuthRequired: {
		EventProbeOK:           StateOK,
		EventProbeFail:         StateError,
		EventProbeAuthRequired: StateAuthRequired,
		EventStale:             StateDegraded,
		EventReset:             StateUnknown,
	},
	StateDegraded: {
		EventProbeOK:           StateOK,
		EventProbeFail:         StateError,
		EventProbeAuthRequired: StateAuthRequired,
		EventStale:             StateDegraded,
		EventReset:             StateUnknown,
	},
}

// Transition applies event to from and returns the next state.
// Illegal transitions return an error and leave from unchanged.
func Transition(from State, event Event) (State, error) {
	from = Normalize(string(from))
	table, ok := transitions[from]
	if !ok {
		return from, fmt.Errorf("lifecycle: unknown state %q", from)
	}
	next, ok := table[event]
	if !ok {
		return from, fmt.Errorf("lifecycle: illegal transition %s --%s-->", from, event)
	}
	return next, nil
}

// EventFromProbeStatus maps probe result status strings to lifecycle events.
func EventFromProbeStatus(status string) Event {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "healthy":
		return EventProbeOK
	case "auth_required":
		return EventProbeAuthRequired
	case "degraded":
		return EventStale
	default:
		return EventProbeFail
	}
}
