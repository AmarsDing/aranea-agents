package turn

import "aranea-agents/internal/biz"

// DecideAdmission maps active-run registry state to an admission decision.
// When a session has an active run but the runner is not yet ready (starting
// phase), callers should reject new turns to avoid duplicate execution.
func DecideAdmission(hasActive, hasActiveRunner bool) biz.TurnAdmissionDecision {
	if !hasActive {
		return biz.AdmitRun
	}
	if hasActiveRunner {
		return biz.AdmitEnqueue
	}
	return biz.AdmitReject
}

// EvaluateAdmission applies entry-point queue policy on top of registry state.
func EvaluateAdmission(hasActive, hasActiveRunner, allowQueue bool) biz.TurnAdmissionDecision {
	if !hasActive {
		return biz.AdmitRun
	}
	if !allowQueue {
		return biz.AdmitReject
	}
	return DecideAdmission(hasActive, hasActiveRunner)
}
