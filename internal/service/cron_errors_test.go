package service

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func TestMapCronError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantNil    bool
		wantCode   apierror.Code
		wantDomain string
	}{
		{"nil", nil, true, "", ""},
		{"cron_not_found", biz.ErrCronNotFound, false, apierror.CodeNotFound, "CRON"},
		{"runner_disabled", biz.ErrCronRunnerDisabled, false, apierror.CodeUnavailable, "CRON"},
		{"task_deleted", biz.ErrCronTaskDeleted, false, apierror.CodeNotFound, "CRON"},
		{"session_busy", biz.ErrCronSessionBusy, false, apierror.CodeConflict, "CRON_SESSION_BUSY"},
		{"required_string", errors.New("field is required"), false, apierror.CodeBadRequest, "CRON"},
		{"invalid_string", errors.New("invalid parameter"), false, apierror.CodeBadRequest, "CRON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCronError(tt.err)
			if tt.wantNil {
				if got != nil {
					t.Errorf("mapCronError() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil error")
			}
			ae, ok := apierror.From(got)
			if !ok {
				t.Fatalf("expected *apierror.Error, got %T: %v", got, got)
			}
			if ae.Code != tt.wantCode {
				t.Errorf("code = %v, want %v", ae.Code, tt.wantCode)
			}
			if ae.Domain != tt.wantDomain {
				t.Errorf("domain = %q, want %q", ae.Domain, tt.wantDomain)
			}
		})
	}
}

func TestMapCronError_ApiErrorPassthrough(t *testing.T) {
	original := apierror.BadRequest("CRON", "bad input")
	got := mapCronError(original)
	if got != original {
		t.Errorf("apierror should pass through, got %v", got)
	}
}

func TestMapCronError_GenericPassthrough(t *testing.T) {
	original := errors.New("something unexpected")
	got := mapCronError(original)
	if got != original {
		t.Errorf("generic error should pass through, got %v", got)
	}
}
