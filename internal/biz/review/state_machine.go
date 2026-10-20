package review

import "fmt"

var transitions = map[string]map[string]bool{
	StatusPending:   {StatusApproved: true, StatusRejected: true, StatusCancelled: true},
	StatusApproved:  {},
	StatusRejected:  {},
	StatusCancelled: {},
}

func CanTransition(from, to string) bool {
	next, ok := transitions[from]
	return ok && next[to]
}

func Transition(from, to string) (string, error) {
	if !CanTransition(from, to) {
		return from, fmt.Errorf("review: illegal transition %s → %s", from, to)
	}
	return to, nil
}
