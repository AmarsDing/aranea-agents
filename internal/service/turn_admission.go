package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime/turn"
)

// TurnAdmissionDecision is the outcome of turn admission while holding session lock.
type TurnAdmissionDecision int

const (
	TurnAdmitNewTurn TurnAdmissionDecision = iota
	TurnAdmitEnqueue
	TurnRejectBusy
)

func admissionFromBiz(d biz.TurnAdmissionDecision) TurnAdmissionDecision {
	switch d {
	case biz.AdmitEnqueue:
		return TurnAdmitEnqueue
	case biz.AdmitReject:
		return TurnRejectBusy
	default:
		return TurnAdmitNewTurn
	}
}

// DecideTurnAdmission maps active-run registry state to an admission decision.
func DecideTurnAdmission(hasActive, hasActiveRunner bool) TurnAdmissionDecision {
	return admissionFromBiz(turn.DecideAdmission(hasActive, hasActiveRunner))
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
