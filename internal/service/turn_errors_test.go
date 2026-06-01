package service

import (
	"errors"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestTurnError_withDetail(t *testing.T) {
	err := TurnError(TurnErrTurnTimeout, "5m")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	ke := kerrors.FromError(err)
	if ke == nil {
		t.Fatal("expected kratos error")
	}
	if ke.Reason != "CHAT_AGENT" {
		t.Errorf("Reason = %q, want CHAT_AGENT", ke.Reason)
	}
}

func TestTurnError_withoutDetail(t *testing.T) {
	err := TurnError(TurnErrEmptyReply, "")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	ke := kerrors.FromError(err)
	if ke == nil {
		t.Fatal("expected kratos error")
	}
}

func TestTurnError_forbiddenCode(t *testing.T) {
	err := TurnError(TurnErrAgentForbidden, "agent-1")
	ke := kerrors.FromError(err)
	if ke == nil {
		t.Fatal("expected kratos error")
	}
	if ke.Code != 403 {
		t.Errorf("Code = %d, want 403 for forbidden", ke.Code)
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
			ke := kerrors.FromError(err)
			if ke == nil {
				t.Fatal("expected kratos error")
			}
			if ke.Code != 400 {
				t.Errorf("Code = %d, want 400 for bad request", ke.Code)
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
			ke := kerrors.FromError(err)
			if ke == nil {
				t.Fatal("expected kratos error")
			}
			if ke.Code != 500 {
				t.Errorf("Code = %d, want 500 for internal server error", ke.Code)
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
	err := kerrors.InternalServer("OTHER", "unrelated error")
	got := TurnErrorCodeFromErr(err)
	if got != "" {
		t.Errorf("TurnErrorCodeFromErr(unmatched) = %q, want empty", got)
	}
}
