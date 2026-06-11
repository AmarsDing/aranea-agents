// Package shared provides cross-aggregate value objects, error sentinels, and
// generic helpers used throughout the biz layer.
//
// State Machine (AS-FSM-01):
//
//	This file implements the unified generic StateMachine interface that all
//	entity state machines (Run, Session, TeamRun, Team, GraphExecution,
//	SessionRunPhase) must implement. Each entity state machine file is named
//	*_state_machine.go and co-locates with its entity package.
//
//	Mermaid state diagram generation:
//	Each entity state machine should include a Mermaid diagram comment block
//	documenting its states and transitions, e.g.:
//
//	  /*
//	  ```mermaid
//	  stateDiagram-v2
//	    [*] --> Idle
//	    Idle --> Running : Start
//	    Running --> Completed : Finish
//	    Running --> Failed : Error
//	  ```
//	  */
package shared

import (
	"context"
	"fmt"
	"sort"
)

// ── Generic State Machine ────────────────────────────────────────────────────

// StateMachine is the unified interface that all entity state machines must implement.
// S is the state type (must be a string-like type), E is the event type (must be a string-like type).
//
// Stability:stable
type StateMachine[S ~string, E ~string] interface {
	// Transition validates and executes a state transition triggered by the given event.
	// Returns the target state on success, or an error if the transition is invalid
	// or a guard condition rejects it.
	Transition(from S, event E) (S, error)

	// CanTransition reports whether a direct transition from one state to another is valid.
	CanTransition(from S, to S) bool

	// ValidTargets returns the sorted list of states reachable from the given state.
	ValidTargets(from S) []S
}

// Guard is an optional predicate that must return true for a transition to fire.
// The context argument allows transition guards to respect cancellation or deadlines.
type Guard func(ctx context.Context) bool

// TransitionRule defines a single allowed state transition.
type TransitionRule[S ~string, E ~string] struct {
	From  S     // source state
	Event E     // trigger event
	To    S     // target state
	Guard Guard // optional guard condition; nil means always allowed
}

// GenericStateMachine implements StateMachine using a rule-based transition table.
// It pre-indexes rules for O(1) transition lookups and O(1) CanTransition checks.
// The machine is safe for concurrent use after construction — all internal state is immutable.
type GenericStateMachine[S ~string, E ~string] struct {
	fromEventIndex map[S]map[E]TransitionRule[S, E] // from → event → rule
	fromToIndex    map[S]map[S]bool                  // from → to → allowed
}

// NewGenericStateMachine builds a GenericStateMachine from the given transition rules.
// Duplicate (from, event) pairs panic — each source state must have at most one rule per event.
func NewGenericStateMachine[S, E ~string](rules []TransitionRule[S, E]) *GenericStateMachine[S, E] {
	fei := make(map[S]map[E]TransitionRule[S, E], len(rules))
	fti := make(map[S]map[S]bool, len(rules))

	for _, r := range rules {
		// Build fromEventIndex
		events, ok := fei[r.From]
		if !ok {
			events = make(map[E]TransitionRule[S, E])
			fei[r.From] = events
		}
		if _, exists := events[r.Event]; exists {
			panic(fmt.Sprintf("duplicate transition rule: from=%s event=%s", r.From, r.Event))
		}
		events[r.Event] = r

		// Build fromToIndex
		targets, ok := fti[r.From]
		if !ok {
			targets = make(map[S]bool)
			fti[r.From] = targets
		}
		targets[r.To] = true
	}

	return &GenericStateMachine[S, E]{
		fromEventIndex: fei,
		fromToIndex:    fti,
	}
}

// Transition validates and executes a state transition triggered by the given event.
// If a guard is defined on the matching rule and it returns false, an error is returned.
func (m *GenericStateMachine[S, E]) Transition(from S, event E) (S, error) {
	events, ok := m.fromEventIndex[from]
	if !ok {
		var zero S
		return zero, fmt.Errorf("invalid state transition: from=%s event=%s", from, event)
	}
	rule, ok := events[event]
	if !ok {
		var zero S
		return zero, fmt.Errorf("invalid state transition: from=%s event=%s", from, event)
	}
	if rule.Guard != nil && !rule.Guard(context.Background()) {
		var zero S
		return zero, fmt.Errorf("invalid state transition: from=%s event=%s: guard rejected", from, event)
	}
	return rule.To, nil
}

// CanTransition reports whether a direct transition from one state to another is valid.
func (m *GenericStateMachine[S, E]) CanTransition(from S, to S) bool {
	targets, ok := m.fromToIndex[from]
	if !ok {
		return false
	}
	return targets[to]
}

// ValidTargets returns the sorted list of states reachable from the given state.
func (m *GenericStateMachine[S, E]) ValidTargets(from S) []S {
	targets, ok := m.fromToIndex[from]
	if !ok {
		return nil
	}
	result := make([]S, 0, len(targets))
	for t := range targets {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}
