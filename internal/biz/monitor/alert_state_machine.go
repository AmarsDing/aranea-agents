package monitor

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

// AlertFiringEvent represents events that trigger state transitions in the alert firing lifecycle.
type AlertFiringEvent string

const (
	AlertEventThresholdExceeded AlertFiringEvent = "threshold_exceeded"
	AlertEventRecovered         AlertFiringEvent = "recovered"
	AlertEventReset             AlertFiringEvent = "reset"
)

// alertFiringTransitions defines the legal state transitions for the alert firing state machine.
// Key: current state → event → next state.
var alertFiringTransitions = map[AlertFiringState]map[AlertFiringEvent]AlertFiringState{
	AlertFiringStateIdle: {
		AlertEventThresholdExceeded: AlertFiringStateFiring,
	},
	AlertFiringStateFiring: {
		AlertEventRecovered: AlertFiringStateRecovered,
		AlertEventReset:     AlertFiringStateIdle,
	},
	AlertFiringStateRecovered: {
		AlertEventThresholdExceeded: AlertFiringStateFiring,
		AlertEventReset:             AlertFiringStateIdle,
	},
}

// TransitionAlertFiringState validates and returns the next state for a given (current, event) pair.
// Returns an error if the transition is not allowed.
// An empty current state is treated as idle (zero-value default for rules not yet persisted).
func TransitionAlertFiringState(current AlertFiringState, event AlertFiringEvent) (AlertFiringState, error) {
	// Normalize empty string to idle (rules loaded from DB before MON-OPT-02 migration)
	if current == "" {
		current = AlertFiringStateIdle
	}
	events, ok := alertFiringTransitions[current]
	if !ok {
		return current, apierror.BadRequest("ALERT_STATE", "unknown alert firing state: %s", current)
	}
	next, ok := events[event]
	if !ok {
		return current, apierror.BadRequest("ALERT_TRANSITION", "invalid transition: %s + %s", current, event)
	}
	return next, nil
}

// MustTransitionAlertFiringState is like TransitionAlertFiringState but panics on invalid transition.
// Use only in tests.
func MustTransitionAlertFiringState(current AlertFiringState, event AlertFiringEvent) AlertFiringState {
	next, err := TransitionAlertFiringState(current, event)
	if err != nil {
		panic(fmt.Sprintf("invalid alert state transition: %v", err))
	}
	return next
}
