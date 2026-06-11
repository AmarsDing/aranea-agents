package session

import (
	"time"

	"aranea-agents/pkg/apierror"
)

var validTransitions = map[SessionStatus][]SessionStatus{
	SessionStatusIdle:                 {SessionStatusRunning},
	SessionStatusRunning:              {SessionStatusCompleted, SessionStatusInterrupted, SessionStatusAwaitingConfirmation},
	SessionStatusCompleted:            {SessionStatusRunning},
	SessionStatusInterrupted:          {SessionStatusRunning},
	SessionStatusAwaitingConfirmation: {SessionStatusRunning, SessionStatusInterrupted},
}

type SessionStatusMachine struct {
	status       SessionStatus
	statusReason SessionStatusReason
	changedAt    string
}

func NewSessionStatusMachine(status SessionStatus, reason SessionStatusReason, changedAt string) *SessionStatusMachine {
	return &SessionStatusMachine{
		status:       status,
		statusReason: reason,
		changedAt:    changedAt,
	}
}

func (m *SessionStatusMachine) TransitionTo(target SessionStatus, reason SessionStatusReason) error {
	if !m.CanTransitionTo(target) {
		return apierror.Conflict("SESSION", "cannot transition session status from %s to %s", m.status, target)
	}
	m.status = target
	m.statusReason = reason
	m.changedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

func (m *SessionStatusMachine) CanTransitionTo(target SessionStatus) bool {
	allowed, ok := validTransitions[m.status]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == target {
			return true
		}
	}
	return false
}

func (m *SessionStatusMachine) Status() SessionStatus            { return m.status }
func (m *SessionStatusMachine) StatusReason() SessionStatusReason { return m.statusReason }
func (m *SessionStatusMachine) ChangedAt() string                { return m.changedAt }
