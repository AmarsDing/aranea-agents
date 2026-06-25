package service

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

func TestClassifyChannelTurnError_taxonomy(t *testing.T) {
	if classifyChannelTurnError(turnBusyError()) != biz.ChannelTurnErrBusy {
		t.Fatal("busy")
	}
	if classifyChannelTurnError(errors.New("429 Too Many Requests")) != biz.ChannelTurnErrRateLimit {
		t.Fatal("rate limit")
	}
	if classifyChannelTurnError(errors.New("maximum context length exceeded")) != biz.ChannelTurnErrContextOverflow {
		t.Fatal("overflow")
	}
	if formatChannelTurnErrorMessage(errors.New("429 Too Many Requests")) != biz.ChannelTurnErrorRateLimitMsg {
		t.Fatal("rate limit message")
	}
	if formatChannelTurnErrorMessage(errors.New("maximum context length exceeded")) != biz.ChannelTurnErrorContextOverflowMsg {
		t.Fatal("overflow message")
	}
}

// Phase 1c-5: TestTurnPreviewCoordinator_runIDFilter removed — tested deleted
// EnvelopeType TextDelta behavior in consume() (now a no-op).
