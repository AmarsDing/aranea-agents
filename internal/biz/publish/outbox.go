package publish

import "fmt"

var outboxTransitions = map[string]map[string]bool{
	StatusPending:    {StatusPackaging: true, StatusFailed: true},
	StatusPackaging:  {StatusPackaged: true, StatusFailed: true},
	StatusFailed:     {StatusPending: true, StatusDeadLetter: true},
	StatusPackaged:   {},
	StatusDeadLetter: {},
}

func CanOutboxTransition(from, to string) bool {
	next, ok := outboxTransitions[from]
	return ok && next[to]
}

func OutboxTransition(from, to string) (string, error) {
	if !CanOutboxTransition(from, to) {
		return from, fmt.Errorf("publish outbox: illegal %s → %s", from, to)
	}
	return to, nil
}
