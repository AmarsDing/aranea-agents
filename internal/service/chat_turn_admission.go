package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime/turn"
)

type chatUCSessionLocker struct {
	uc *biz.ChatUsecase
}

func (l chatUCSessionLocker) LockSession(sessionID string) func() {
	if l.uc == nil {
		return func() {}
	}
	return l.uc.LockSession(sessionID)
}

func newTurnAdmissionGate(reg turn.RunRegistryAdapter, uc *biz.ChatUsecase, mergeFn func(sessionID string) bool) *turn.AdmissionGate {
	return turn.NewAdmissionGate(turn.AdmissionGateDeps{
		Runs:    reg,
		Lock:    chatUCSessionLocker{uc: uc},
		Enqueue: chatUCMessageEnqueuer{uc: uc, mergeFollowup: mergeFn},
	})
}

type chatUCMessageEnqueuer struct {
	uc            *biz.ChatUsecase
	mergeFollowup func(sessionID string) bool
}

func (e chatUCMessageEnqueuer) EnqueueUserMessage(sessionID, content string) (bool, string, string, error) {
	if e.uc == nil {
		return false, "", "", nil
	}
	merge := false
	if e.mergeFollowup != nil {
		merge = e.mergeFollowup(sessionID)
	}
	accepted, _, pendingID, rejectReason, err := e.uc.EnqueueUserMessage(sessionID, content, merge)
	return accepted, pendingID, rejectReason, err
}

func nativeResultFromAdmissionVerdict(v turn.AdmissionVerdict) (biz.NativeTurnResult, error) {
	switch v.Action {
	case turn.AdmissionQueued:
		return biz.NativeTurnResult{
			Outcome:   biz.NativeTurnOutcomeQueued,
			PendingID: v.PendingID,
		}, ErrTurnMessageQueued
	case turn.AdmissionRejectBusy:
		return biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed}, turnBusyError()
	case turn.AdmissionRejectEnqueue:
		if v.Err != nil {
			return biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed}, v.Err
		}
		return biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed}, classifyEnqueueOutcome(false, v.RejectReason, nil)
	default:
		return biz.NativeTurnResult{}, nil
	}
}
