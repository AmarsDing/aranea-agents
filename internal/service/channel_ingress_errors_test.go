package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFormatChannelTurnErrorMessageTimeout(t *testing.T) {
	msg := formatChannelTurnErrorMessage(context.DeadlineExceeded)
	if msg != channelTurnErrorSyncCapMsg {
		t.Fatalf("deadline message = %q, want %q", msg, channelTurnErrorSyncCapMsg)
	}
	if !turnErrorIsTimeout(context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded should be timeout")
	}
	if !turnErrorIsTimeout(errors.New("context deadline exceeded")) {
		t.Fatal("deadline string should be timeout")
	}
}

func TestFormatChannelTurnErrorMessageTurnTimeout(t *testing.T) {
	msg := formatChannelTurnErrorMessage(TurnError(TurnErrTurnTimeout, "5m"))
	if msg != channelTurnErrorSyncCapMsg {
		t.Fatalf("turn timeout message = %q, want sync cap hint", msg)
	}
}

func TestFormatChannelTurnErrorMessageBusy(t *testing.T) {
	msg := formatChannelTurnErrorMessage(turnBusyError())
	if msg != channelTurnErrorBusyMsg {
		t.Fatalf("busy message = %q, want %q", msg, channelTurnErrorBusyMsg)
	}
}

func TestFormatChannelTurnErrorMessageGeneric(t *testing.T) {
	msg := formatChannelTurnErrorMessage(errors.New("internal sql: connection refused"))
	if msg != channelTurnErrorGenericMsg {
		t.Fatalf("generic message = %q, want fixed template", msg)
	}
	if strings.Contains(msg, "sql") {
		t.Fatal("must not leak internal error text to IM")
	}
}

func TestTurnErrorIsCanceled(t *testing.T) {
	if formatChannelTurnErrorMessage(context.Canceled) != "" {
		t.Fatal("canceled should not produce IM error text")
	}
	if turnErrorIsTimeout(context.Canceled) {
		t.Fatal("canceled must not be classified as timeout")
	}
	if !turnErrorIsCanceled(context.Canceled) {
		t.Fatal("expected canceled")
	}
}

func TestTurnErrorIsTimeoutFalse(t *testing.T) {
	if turnErrorIsTimeout(errors.New("validation failed")) {
		t.Fatal("non-timeout error")
	}
}
