package httpx

import (
	"arenea/backend/internal/kernel/errs"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrMapsKnownErrors(t *testing.T) {
	tests := []struct {
		name       string
		fallback   int
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "not found", fallback: http.StatusBadRequest, err: sql.ErrNoRows, wantStatus: http.StatusNotFound, wantBody: "resource not found"},
		{name: "conflict", fallback: http.StatusBadRequest, err: errs.ErrConflict, wantStatus: http.StatusConflict, wantBody: "conflict"},
		{name: "internal redacted", fallback: http.StatusInternalServerError, err: errors.New("database password leaked"), wantStatus: http.StatusInternalServerError, wantBody: "internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteErr(rec, tt.fallback, tt.err)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
