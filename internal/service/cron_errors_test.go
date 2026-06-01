package service

import (
	"database/sql"
	"errors"
	"testing"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestMapCronError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantNil    bool
		wantCode   int32
		wantReason string
	}{
		{"nil", nil, true, 0, ""},
		{"sql_no_rows", sql.ErrNoRows, false, 404, "CRON"},
		{"runner_disabled", biz.ErrCronRunnerDisabled, false, 503, "CRON"},
		{"task_deleted", biz.ErrCronTaskDeleted, false, 404, "CRON"},
		{"session_busy", biz.ErrCronSessionBusy, false, 409, "CRON_SESSION_BUSY"},
		{"required_string", errors.New("field is required"), false, 400, "CRON"},
		{"invalid_string", errors.New("invalid parameter"), false, 400, "CRON"},
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
			ke, ok := got.(*kerrors.Error)
			if !ok {
				t.Fatalf("expected *kerrors.Error, got %T: %v", got, got)
			}
			if ke.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", ke.Code, tt.wantCode)
			}
			if ke.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", ke.Reason, tt.wantReason)
			}
		})
	}
}

func TestMapCronError_KratosErrorPassthrough(t *testing.T) {
	original := kerrors.BadRequest("CRON", "bad input")
	got := mapCronError(original)
	if got != original {
		t.Errorf("kerrors should pass through, got %v", got)
	}
}

func TestMapCronError_GenericPassthrough(t *testing.T) {
	original := errors.New("something unexpected")
	got := mapCronError(original)
	if got != original {
		t.Errorf("generic error should pass through, got %v", got)
	}
}
