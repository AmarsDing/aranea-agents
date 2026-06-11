package service

import (
	"errors"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ErrTurnMessageQueued indicates the user message was accepted into steer or pending
// queue while another turn is active; no assistant message is produced yet.
var ErrTurnMessageQueued = errors.New("turn message queued")

func isTurnMessageQueued(err error) bool {
	return errors.Is(err, ErrTurnMessageQueued)
}

func turnBusyError() error {
	return apierror.Conflict("CHAT_TURN_BUSY", "session turn is starting; retry in a moment or use enqueue")
}

// isTurnBusyError reports admission conflict while a run is starting (no runner yet).
func isTurnBusyError(err error) bool {
	if err == nil {
		return false
	}
	if ae, ok := apierror.From(err); ok {
		return ae.Code == apierror.CodeConflict && ae.Domain == "CHAT_TURN_BUSY"
	}
	return false
}

func turnResultToNative(tr biz.TurnResult, err error) (biz.NativeTurnResult, error) {
	if err != nil {
		if isTurnMessageQueued(err) {
			return biz.NativeTurnResult{
				Outcome:   biz.NativeTurnOutcomeQueued,
				PendingID: tr.PendingID,
			}, err
		}
		return biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed, UserMsg: tr.UserMsg}, err
	}
	switch tr.Outcome {
	case biz.TurnOutcomeCompleted:
		return biz.NativeTurnResult{
			Outcome:      biz.NativeTurnOutcomeCompleted,
			UserMsg:      tr.UserMsg,
			AssistantMsg: tr.AssistantMsg,
		}, nil
	case biz.TurnOutcomeQueued:
		return biz.NativeTurnResult{
			Outcome:   biz.NativeTurnOutcomeQueued,
			PendingID: tr.PendingID,
		}, ErrTurnMessageQueued
	default:
		return biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed}, nil
	}
}
