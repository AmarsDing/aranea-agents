package service

// TurnAdmissionDecision is the outcome of turn admission while holding session lock.
type TurnAdmissionDecision int

const (
	TurnAdmitNewTurn TurnAdmissionDecision = iota
	TurnAdmitEnqueue
	TurnRejectBusy
)

// DecideTurnAdmission maps active-run registry state to an admission decision.
func DecideTurnAdmission(hasActive, hasActiveRunner bool) TurnAdmissionDecision {
	if !hasActive {
		return TurnAdmitNewTurn
	}
	if hasActiveRunner {
		return TurnAdmitEnqueue
	}
	return TurnRejectBusy
}

// classifyEnqueueOutcome maps ChatUsecase enqueue result to service-level errors.
func classifyEnqueueOutcome(accepted bool, rejectReason string, err error) error {
	if err != nil {
		return err
	}
	if !accepted {
		return enqueueRejectError(rejectReason)
	}
	return ErrTurnMessageQueued
}
