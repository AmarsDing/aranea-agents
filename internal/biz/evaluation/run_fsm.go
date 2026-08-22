package evaluation

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

// runTransitions lists legal outbound statuses for each run state (AS-FSM-01).
// Same-status writes are always allowed (progress persistence while running).
var runTransitions = map[string][]string{
	"":                 {RunStatusPending, RunStatusRunning, RunStatusCompleted, RunStatusFailed, RunStatusCancelled},
	RunStatusPending:   {RunStatusRunning, RunStatusFailed, RunStatusCancelled},
	RunStatusRunning:   {RunStatusCompleted, RunStatusFailed, RunStatusCancelled},
	RunStatusCompleted: {},
	RunStatusFailed:    {},
	RunStatusCancelled: {},
}

// CanTransition reports whether from→to is a legal run-status change.
func CanTransition(from, to string) bool {
	if from == to {
		return true
	}
	allowed, ok := runTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// ValidateTransition returns a Conflict when from→to is illegal.
func ValidateTransition(from, to string) error {
	if to == "" {
		return apierror.BadRequest("EVAL", "run status is required")
	}
	if CanTransition(from, to) {
		return nil
	}
	return apierror.Conflict("EVAL", fmt.Sprintf("illegal run transition %s → %s", from, to))
}
