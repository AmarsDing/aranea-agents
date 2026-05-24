package turn

import (
	"strings"

	"aranea-agents/internal/biz"
)

// AdmissionAction classifies the admission gate outcome before turn body execution.
type AdmissionAction int

const (
	AdmissionProceed AdmissionAction = iota
	AdmissionQueued
	AdmissionRejectBusy
	AdmissionRejectEnqueue
)

// AdmissionVerdict is the structured result of a pre-turn admission check.
type AdmissionVerdict struct {
	Action       AdmissionAction
	PendingID    string
	RejectReason string
	Err          error
}

// AdmissionGateDeps wires registry, lock, and enqueue dependencies.
type AdmissionGateDeps struct {
	Runs    ActiveRunRegistry
	Lock    SessionLocker
	Enqueue MessageEnqueuer
}

// AdmissionGate performs session-locked admission before turn body execution.
type AdmissionGate struct {
	runs    ActiveRunRegistry
	lock    SessionLocker
	enqueue MessageEnqueuer
}

// NewAdmissionGate constructs an admission gate from dependencies.
func NewAdmissionGate(deps AdmissionGateDeps) *AdmissionGate {
	return &AdmissionGate{
		runs:    deps.Runs,
		lock:    deps.Lock,
		enqueue: deps.Enqueue,
	}
}

// Check evaluates whether a turn should proceed, enqueue, or reject.
// It mirrors the pre-body admission block previously inlined in ChatOrchestrator.
func (g *AdmissionGate) Check(input biz.TurnInput) AdmissionVerdict {
	if g == nil {
		return AdmissionVerdict{Action: AdmissionProceed}
	}

	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)
	if sessionID == "" || content == "" {
		return AdmissionVerdict{Action: AdmissionProceed}
	}

	unlock := g.lockSession(sessionID)
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	if g.runs == nil || !g.runs.HasActive(sessionID) {
		return AdmissionVerdict{Action: AdmissionProceed}
	}

	hasRunner := g.runs.HasActiveRunner(sessionID)
	switch EvaluateAdmission(true, hasRunner, input.AllowPendingQueue()) {
	case biz.AdmitEnqueue:
		return g.tryEnqueue(sessionID, content)
	case biz.AdmitReject:
		return AdmissionVerdict{Action: AdmissionRejectBusy}
	default:
		return AdmissionVerdict{Action: AdmissionProceed}
	}
}

func (g *AdmissionGate) lockSession(sessionID string) func() {
	if g.lock == nil {
		return func() {}
	}
	return g.lock.LockSession(sessionID)
}

func (g *AdmissionGate) tryEnqueue(sessionID, content string) AdmissionVerdict {
	if g.enqueue == nil {
		return AdmissionVerdict{Action: AdmissionRejectEnqueue, RejectReason: "enqueue unavailable"}
	}
	accepted, pendingID, rejectReason, err := g.enqueue.EnqueueUserMessage(sessionID, content)
	if err != nil {
		return AdmissionVerdict{Action: AdmissionRejectEnqueue, Err: err}
	}
	if !accepted {
		return AdmissionVerdict{
			Action:       AdmissionRejectEnqueue,
			RejectReason: rejectReason,
		}
	}
	return AdmissionVerdict{
		Action:    AdmissionQueued,
		PendingID: pendingID,
	}
}
