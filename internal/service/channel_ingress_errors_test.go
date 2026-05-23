package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFormatChannelTurnErrorMessageTimeout(t *testing.T) {
	msg := formatChannelTurnErrorMessage(context.DeadlineExceeded)
	if msg != channelTurnErrorTimeoutMsg {
		t.Fatalf("deadline message = %q, want %q", msg, channelTurnErrorTimeoutMsg)
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

func TestFormatChannelTurnErrorMessageGeneric(t *testing.T) {
	msg := formatChannelTurnErrorMessage(errors.New("internal sql: connection refused"))
	if msg != channelTurnErrorGenericMsg {
		t.Fatalf("generic message = %q, want fixed template", msg)
	}
	if strings.Contains(msg, "sql") {
		t.Fatal("must not leak internal error text to IM")
	}
}

func TestTurnErrorIsTimeoutFalse(t *testing.T) {
	if turnErrorIsTimeout(errors.New("validation failed")) {
		t.Fatal("non-timeout error")
	}
}
