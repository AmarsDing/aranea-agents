package service

import (
	"testing"

	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/testutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestPublishTurnFailure_usesEnvelopeErrorFromTurn(t *testing.T) {
	bus := testutil.NewRecordingBus()
	orch := &ChatOrchestrator{
		td: rt.TurnDeps{Pipeline: rt.EventPipeline{Bus: bus}},
	}
	te := TurnError(TurnErrLLMCallFailed, "connection reset")
	orch.publishTurnFailure("sess-1", "run-1", "chat-service", te, "")

	errs := bus.EventsOfType(event.EnvelopeTypeError)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error envelope, got %d", len(errs))
	}
	env := errs[0]
	if env.Error == nil || env.Error.Code != string(TurnErrLLMCallFailed) {
		t.Fatalf("unexpected error envelope: %+v", env.Error)
	}
	if env.InvocationID != "run-1" {
		t.Fatalf("expected invocation_id run-1, got %q", env.InvocationID)
	}
	if env.Error.Hint == "" {
		t.Fatal("expected non-empty hint for LLM_CALL_FAILED")
	}
}

func TestPublishTurnFailure_pendingID(t *testing.T) {
	bus := testutil.NewRecordingBus()
	orch := &ChatOrchestrator{
		td: rt.TurnDeps{Pipeline: rt.EventPipeline{Bus: bus}},
	}
	orch.publishTurnFailure("sess-1", "", "pending-queue", TurnError(TurnErrTurnTimeout, "5m"), "pend-1")

	errs := bus.EventsOfType(event.EnvelopeTypeError)
	if len(errs) != 1 || errs[0].Error.PendingID != "pend-1" {
		t.Fatalf("expected pending_id pend-1, got %+v", errs[0].Error)
	}
}

func TestEnvelopeErrorFromTurn_redactsUnknownDetail(t *testing.T) {
	envErr := envelopeErrorFromTurn("", `POST "https://api.deepseek.com/v1/chat/completions": 400 Bad Request`)
	if envErr == nil {
		t.Fatal("expected error payload")
	}
	if envErr.Message == "" || envErr.Message == `POST "https://api.deepseek.com/v1/chat/completions": 400 Bad Request` {
		t.Fatalf("expected redacted user-facing message, got %q", envErr.Message)
	}
	if envErr.Hint == "" {
		t.Fatal("expected recovery hint")
	}
}

func TestTurnErrorCodeFromErr_kratos(t *testing.T) {
	err := TurnError(TurnErrTurnTimeout, "5m")
	if TurnErrorCodeFromErr(err) != TurnErrTurnTimeout {
		t.Fatalf("expected TURN_TIMEOUT")
	}
	generic := kerrors.InternalServer("X", "something else")
	if TurnErrorCodeFromErr(generic) != "" {
		t.Fatal("expected empty code for unmapped error")
	}
}
