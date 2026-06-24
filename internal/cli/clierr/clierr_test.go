package clierr

import (
	"errors"
	"fmt"
	"testing"
)

func TestCLIError_Error_WithHTTP(t *testing.T) {
	e := &CLIError{Code: "BAD_REQ", HTTPStatus: 400, Message: "invalid input"}
	want := "BAD_REQ (HTTP 400): invalid input"
	if got := e.Error(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCLIError_Error_WithoutHTTP(t *testing.T) {
	e := &CLIError{Code: "NETWORK_ERROR", Message: "connection refused"}
	want := "NETWORK_ERROR: connection refused"
	if got := e.Error(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCLIError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner")
	e := &CLIError{Code: "X", Message: "m", Cause: inner}
	if !errors.Is(e, inner) {
		t.Fatal("Unwrap should return Cause")
	}
}

func TestExitCodeOf_Nil(t *testing.T) {
	if got := ExitCodeOf(nil); got != ExitOK {
		t.Fatalf("nil error should be ExitOK, got %d", got)
	}
}

func TestExitCodeOf_NonCLIError(t *testing.T) {
	if got := ExitCodeOf(fmt.Errorf("generic")); got != ExitNetworkError {
		t.Fatalf("non-CLIError should be ExitNetworkError, got %d", got)
	}
}

func TestExitCodeOf_HTTP401(t *testing.T) {
	e := &CLIError{Code: "AUTH", HTTPStatus: 401, Message: "unauthorized"}
	if got := ExitCodeOf(e); got != ExitAuthError {
		t.Fatalf("got %d, want ExitAuthError(%d)", got, ExitAuthError)
	}
}

func TestExitCodeOf_HTTP403(t *testing.T) {
	e := &CLIError{Code: "FORBIDDEN", HTTPStatus: 403, Message: "forbidden"}
	if got := ExitCodeOf(e); got != ExitAuthError {
		t.Fatalf("got %d, want ExitAuthError(%d)", got, ExitAuthError)
	}
}

func TestExitCodeOf_HTTP500(t *testing.T) {
	e := &CLIError{Code: "INTERNAL", HTTPStatus: 500, Message: "oops"}
	if got := ExitCodeOf(e); got != ExitNetworkError {
		t.Fatalf("got %d, want ExitNetworkError(%d)", got, ExitNetworkError)
	}
}

func TestExitCodeOf_HTTP400(t *testing.T) {
	e := &CLIError{Code: "BAD", HTTPStatus: 400, Message: "bad"}
	if got := ExitCodeOf(e); got != ExitBackendBizError {
		t.Fatalf("got %d, want ExitBackendBizError(%d)", got, ExitBackendBizError)
	}
}

func TestExitCodeOf_UserCanceled(t *testing.T) {
	e := &CLIError{Code: "USER_CANCELED", Message: "nope"}
	if got := ExitCodeOf(e); got != ExitUserCanceled {
		t.Fatalf("got %d, want ExitUserCanceled(%d)", got, ExitUserCanceled)
	}
}

func TestExitCodeOf_SkillImportBlocked(t *testing.T) {
	e := &CLIError{Code: "SKILL_IMPORT_BLOCKED", Message: "blocked"}
	if got := ExitCodeOf(e); got != ExitConflictBlocked {
		t.Fatalf("got %d, want ExitConflictBlocked(%d)", got, ExitConflictBlocked)
	}
}

func TestExitCodeOf_ConfirmationRequired(t *testing.T) {
	e := &CLIError{Code: "CONFIRMATION_REQUIRED", Message: "confirm"}
	if got := ExitCodeOf(e); got != ExitConflictBlocked {
		t.Fatalf("got %d, want ExitConflictBlocked(%d)", got, ExitConflictBlocked)
	}
}

func TestExitCodeOf_AuthCodes(t *testing.T) {
	codes := []string{"UNAUTHENTICATED", "UNAUTHORIZED", "FORBIDDEN", "LOGIN_NO_TOKEN"}
	for _, c := range codes {
		e := &CLIError{Code: c, Message: "auth"}
		if got := ExitCodeOf(e); got != ExitAuthError {
			t.Errorf("code=%q got %d, want ExitAuthError(%d)", c, got, ExitAuthError)
		}
	}
}

func TestExitCodeOf_UsageCodes(t *testing.T) {
	codes := []string{"INSECURE_CONFIG_PERM", "CONFIG_INVALID", "FILE_READ_ERROR", "FILE_PARSE_ERROR"}
	for _, c := range codes {
		e := &CLIError{Code: c, Message: "usage"}
		if got := ExitCodeOf(e); got != ExitUsage {
			t.Errorf("code=%q got %d, want ExitUsage(%d)", c, got, ExitUsage)
		}
	}
}

func TestExitCodeOf_NetworkCodes(t *testing.T) {
	codes := []string{"NETWORK_ERROR", "PKG_FETCH_ERROR", "PKG_INSTALL_ERROR"}
	for _, c := range codes {
		e := &CLIError{Code: c, Message: "net"}
		if got := ExitCodeOf(e); got != ExitNetworkError {
			t.Errorf("code=%q got %d, want ExitNetworkError(%d)", c, got, ExitNetworkError)
		}
	}
}

func TestExitCodeOf_WrappedCLIError(t *testing.T) {
	inner := &CLIError{Code: "USER_CANCELED", Message: "nope"}
	wrapped := fmt.Errorf("wrapper: %w", inner)
	if got := ExitCodeOf(wrapped); got != ExitUserCanceled {
		t.Fatalf("wrapped got %d, want ExitUserCanceled(%d)", got, ExitUserCanceled)
	}
}

func TestExitCodeOf_UnknownCode_DefaultsBackendBiz(t *testing.T) {
	e := &CLIError{Code: "UNKNOWN_CODE", Message: "mystery"}
	if got := ExitCodeOf(e); got != ExitBackendBizError {
		t.Fatalf("unknown code should default to ExitBackendBizError, got %d", got)
	}
}
