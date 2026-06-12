package service

import (
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestTurnError_withDetail(t *testing.T) {
	err := TurnError(TurnErrTurnTimeout, "5m")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatal("expected apierror")
	}
	if ae.Domain != "CHAT_AGENT" {
		t.Errorf("Domain = %q, want CHAT_AGENT", ae.Domain)
	}
}

func TestTurnError_withoutDetail(t *testing.T) {
	err := TurnError(TurnErrEmptyReply, "")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if _, ok := apierror.From(err); !ok {
		t.Fatal("expected apierror")
	}
}

func TestTurnError_forbiddenCode(t *testing.T) {
	err := TurnError(TurnErrAgentForbidden, "agent-1")
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatal("expected apierror")
	}
	if ae.Code != apierror.CodeForbidden {
		t.Errorf("Code = %v, want FORBIDDEN for forbidden", ae.Code)
	}
}

func TestTurnError_badRequestCode(t *testing.T) {
	tests := []struct {
		name string
		code TurnErrorCode
	}{
		{"attachment_failed", TurnErrAttachmentFailed},
		{"attachment_unsupported", TurnErrAttachmentUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := TurnError(tt.code, "")
			ae, ok := apierror.From(err)
			if !ok {
				t.Fatal("expected apierror")
			}
			if ae.Code != apierror.CodeBadRequest {
				t.Errorf("Code = %v, want BAD_REQUEST for bad request", ae.Code)
			}
		})
	}
}

func TestTurnError_internalServerCode(t *testing.T) {
	tests := []struct {
		name string
		code TurnErrorCode
	}{
		{"agent_build_failed", TurnErrAgentBuildFailed},
		{"llm_call_failed", TurnErrLLMCallFailed},
		{"turn_timeout", TurnErrTurnTimeout},
		{"empty_reply", TurnErrEmptyReply},
		{"first_byte_timeout", TurnErrFirstByteTimeout},
		{"stream_preview_failed", TurnErrStreamPreviewFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := TurnError(tt.code, "")
			ae, ok := apierror.From(err)
			if !ok {
				t.Fatal("expected apierror")
			}
			if ae.Code != apierror.CodeInternal {
				t.Errorf("Code = %v, want INTERNAL for internal server error", ae.Code)
			}
		})
	}
}

func TestTurnErrorCodeFromErr_nil(t *testing.T) {
	got := TurnErrorCodeFromErr(nil)
	if got != "" {
		t.Errorf("TurnErrorCodeFromErr(nil) = %q, want empty", got)
	}
}

func TestTurnErrorCodeFromErr_kratosError(t *testing.T) {
	err := TurnError(TurnErrTurnTimeout, "")
	got := TurnErrorCodeFromErr(err)
	if got != TurnErrTurnTimeout {
		t.Errorf("TurnErrorCodeFromErr() = %q, want %q", got, TurnErrTurnTimeout)
	}
}

func TestTurnErrorCodeFromErr_kratosErrorWithDetail(t *testing.T) {
	err := TurnError(TurnErrFirstByteTimeout, "30s")
	got := TurnErrorCodeFromErr(err)
	if got != TurnErrFirstByteTimeout {
		t.Errorf("TurnErrorCodeFromErr() = %q, want %q", got, TurnErrFirstByteTimeout)
	}
}

func TestTurnErrorCodeFromErr_nonKratosError(t *testing.T) {
	err := errors.New("some random error")
	got := TurnErrorCodeFromErr(err)
	if got != "" {
		t.Errorf("TurnErrorCodeFromErr(non-kratos) = %q, want empty", got)
	}
}

func TestTurnErrorCodeFromErr_unmatchedKratosError(t *testing.T) {
	err := apierror.Internal("OTHER", "unrelated error")
	got := TurnErrorCodeFromErr(err)
	if got != "" {
		t.Errorf("TurnErrorCodeFromErr(unmatched) = %q, want empty", got)
	}
}
