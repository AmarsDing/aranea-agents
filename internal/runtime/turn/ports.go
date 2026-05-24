package turn

// ActiveRunRegistry exposes the subset of RunRegistry admission needs.
type ActiveRunRegistry interface {
	HasActive(sessionID string) bool
	HasActiveRunner(sessionID string) bool
}

// SessionLocker acquires per-session mutual exclusion for admission checks.
type SessionLocker interface {
	LockSession(sessionID string) (unlock func())
}

// MessageEnqueuer accepts a follow-up user message while a turn is active.
type MessageEnqueuer interface {
	EnqueueUserMessage(sessionID, content string) (accepted bool, pendingID, rejectReason string, err error)
}
