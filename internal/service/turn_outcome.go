package service

import (
	"errors"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ErrTurnMessageQueued indicates the user message was accepted into steer or pending
// queue while another turn is active; no assistant message is produced yet.
var ErrTurnMessageQueued = errors.New("turn message queued")

func IsTurnMessageQueued(err error) bool {
	return errors.Is(err, ErrTurnMessageQueued)
}

func turnBusyError() error {
	return kerrors.Conflict("CHAT_TURN_BUSY", "session turn is starting; retry in a moment or use enqueue")
}

// IsTurnBusyError reports admission conflict while a run is starting (no runner yet).
func IsTurnBusyError(err error) bool {
	if err == nil {
		return false
	}
	if ke := kerrors.FromError(err); ke != nil {
		return ke.Reason == "CHAT_TURN_BUSY"
	}
	return false
}
