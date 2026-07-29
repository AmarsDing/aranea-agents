// Package biz — SelfImprovementRun State Machine (AS-FSM-01)
//
// This file defines the state machine for SelfImprovementRun.Status
// (design D3, 73-self-iteration-v3.design.md). The suggestion lifecycle
// (pending/approved/applied/...) stays in UnifiedEvolutionStateMachine;
// this machine tracks the seven-stage execution loop.
//
// # SelfImprovementRun State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Detected
//	Detected --> Diagnosing : diagnose
//	Diagnosing --> Patching : patch
//	Diagnosing --> Closed : record_only (low confidence)
//	Patching --> Verifying : verify
//	Verifying --> Patching : verify_fail (retry, attempts<max)
//	Verifying --> VerifyFailed : verify_fail_final
//	Verifying --> AwaitingGovernance : verify_pass
//	AwaitingGovernance --> Applying : apply (auto / approved)
//	AwaitingGovernance --> Rejected : reject
//	Applying --> Applied : apply_done
//	Applied --> Observing : observe
//	Applied --> RolledBack : rollback
//	Observing --> Closed : close
//	Observing --> RolledBack : rollback
//	Detected/Diagnosing/Patching/Verifying/Applying --> Failed : error
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Run status & event types ─────────────────────────────────────────────────

// SelfImprovementRunStatus enumerates all legal states of a SelfImprovementRun.
// Stability:stable
type SelfImprovementRunStatus string

const (
	RunStatusDetected            SelfImprovementRunStatus = "detected"
	RunStatusDiagnosing          SelfImprovementRunStatus = "diagnosing"
	RunStatusPatching            SelfImprovementRunStatus = "patching"
	RunStatusVerifying           SelfImprovementRunStatus = "verifying"
	RunStatusAwaitingGovernance  SelfImprovementRunStatus = "awaiting_governance"
	RunStatusApplying            SelfImprovementRunStatus = "applying"
	RunStatusApplied             SelfImprovementRunStatus = "applied"
	RunStatusObserving           SelfImprovementRunStatus = "observing"
	RunStatusClosed              SelfImprovementRunStatus = "closed"
	RunStatusVerifyFailed        SelfImprovementRunStatus = "verify_failed"
	RunStatusRolledBack          SelfImprovementRunStatus = "rolled_back"
	RunStatusRejected            SelfImprovementRunStatus = "rejected"
	RunStatusFailed              SelfImprovementRunStatus = "failed"
)

// SelfImprovementRunEvent enumerates events triggering run state transitions.
// Stability:stable
type SelfImprovementRunEvent string

const (
	RunEventDiagnose        SelfImprovementRunEvent = "diagnose"
	RunEventPatch           SelfImprovementRunEvent = "patch"
	RunEventRecordOnly      SelfImprovementRunEvent = "record_only"
	RunEventVerify          SelfImprovementRunEvent = "verify"
	RunEventVerifyPass      SelfImprovementRunEvent = "verify_pass"
	RunEventVerifyFail      SelfImprovementRunEvent = "verify_fail"
	RunEventVerifyFailFinal SelfImprovementRunEvent = "verify_fail_final"
	RunEventApply           SelfImprovementRunEvent = "apply"
	RunEventReject          SelfImprovementRunEvent = "reject"
	RunEventApplyDone       SelfImprovementRunEvent = "apply_done"
	RunEventObserve         SelfImprovementRunEvent = "observe"
	RunEventClose           SelfImprovementRunEvent = "close"
	RunEventRollback        SelfImprovementRunEvent = "rollback"
	RunEventError           SelfImprovementRunEvent = "error"
)

// ── Transition rules ─────────────────────────────────────────────────────────

var selfImprovementRunTransitionRules = []shared.TransitionRule[SelfImprovementRunStatus, SelfImprovementRunEvent]{
	{From: RunStatusDetected, Event: RunEventDiagnose, To: RunStatusDiagnosing},
	{From: RunStatusDiagnosing, Event: RunEventPatch, To: RunStatusPatching},
	{From: RunStatusDiagnosing, Event: RunEventRecordOnly, To: RunStatusClosed},
	{From: RunStatusPatching, Event: RunEventVerify, To: RunStatusVerifying},
	{From: RunStatusVerifying, Event: RunEventVerifyPass, To: RunStatusAwaitingGovernance},
	{From: RunStatusVerifying, Event: RunEventVerifyFail, To: RunStatusPatching},
	{From: RunStatusVerifying, Event: RunEventVerifyFailFinal, To: RunStatusVerifyFailed},
	{From: RunStatusAwaitingGovernance, Event: RunEventApply, To: RunStatusApplying},
	{From: RunStatusAwaitingGovernance, Event: RunEventReject, To: RunStatusRejected},
	{From: RunStatusApplying, Event: RunEventApplyDone, To: RunStatusApplied},
	{From: RunStatusApplied, Event: RunEventObserve, To: RunStatusObserving},
	{From: RunStatusApplied, Event: RunEventRollback, To: RunStatusRolledBack},
	{From: RunStatusObserving, Event: RunEventClose, To: RunStatusClosed},
	{From: RunStatusObserving, Event: RunEventRollback, To: RunStatusRolledBack},
	{From: RunStatusDetected, Event: RunEventError, To: RunStatusFailed},
	{From: RunStatusDiagnosing, Event: RunEventError, To: RunStatusFailed},
	{From: RunStatusPatching, Event: RunEventError, To: RunStatusFailed},
	{From: RunStatusVerifying, Event: RunEventError, To: RunStatusFailed},
	{From: RunStatusApplying, Event: RunEventError, To: RunStatusFailed},
}

// ── SelfImprovementRunStateMachine ───────────────────────────────────────────

// SelfImprovementRunStateMachine wraps the generic state machine with
// SelfImprovementRun-specific types. Safe for concurrent use after construction.
// Stability:stable
type SelfImprovementRunStateMachine struct {
	inner *shared.GenericStateMachine[SelfImprovementRunStatus, SelfImprovementRunEvent]
}

// NewSelfImprovementRunStateMachine creates a SelfImprovementRunStateMachine with standard rules.
func NewSelfImprovementRunStateMachine() *SelfImprovementRunStateMachine {
	return &SelfImprovementRunStateMachine{
		inner: shared.NewGenericStateMachine[SelfImprovementRunStatus, SelfImprovementRunEvent](selfImprovementRunTransitionRules),
	}
}

// Transition validates and executes a state transition.
func (sm *SelfImprovementRunStateMachine) Transition(from SelfImprovementRunStatus, event SelfImprovementRunEvent) (SelfImprovementRunStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *SelfImprovementRunStateMachine) CanTransition(from, to SelfImprovementRunStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state.
func (sm *SelfImprovementRunStateMachine) ValidTargets(from SelfImprovementRunStatus) []SelfImprovementRunStatus {
	return sm.inner.ValidTargets(from)
}

// IsSelfImprovementRunTerminal returns true for terminal states with no outgoing transitions.
func IsSelfImprovementRunTerminal(state SelfImprovementRunStatus) bool {
	switch state {
	case RunStatusClosed, RunStatusVerifyFailed, RunStatusRolledBack, RunStatusRejected, RunStatusFailed:
		return true
	default:
		return false
	}
}
