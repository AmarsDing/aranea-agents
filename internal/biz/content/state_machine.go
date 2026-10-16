package content

import "fmt"

var contentTransitions = map[string]map[string]bool{
	StatusCandidate: {StatusSelected: true, StatusArchived: true, StatusRejected: true},
	StatusSelected:  {StatusDrafting: true, StatusArchived: true},
	StatusDrafting:  {StatusInReview: true, StatusSelected: true},
	StatusInReview:  {StatusApproved: true, StatusRejected: true, StatusDrafting: true},
	StatusApproved:  {StatusArchived: true},
	StatusRejected:  {StatusArchived: true, StatusDrafting: true},
	StatusArchived:  {},
}

// CanTransition reports whether from→to is legal (AS-FSM-01).
func CanTransition(from, to string) bool {
	next, ok := contentTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

// Transition validates and returns the new status.
func Transition(from, to string) (string, error) {
	if !CanTransition(from, to) {
		return from, fmt.Errorf("content: illegal transition %s → %s", from, to)
	}
	return to, nil
}
