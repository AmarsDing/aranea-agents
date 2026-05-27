package cli_test

import (
	"testing"

	"aranea-agents/internal/cli"
)

func TestExitCodeOf_Nil(t *testing.T) {
	if code := cli.ExitCodeOf(nil); code != cli.ExitOK {
		t.Errorf("nil error: got %d, want %d", code, cli.ExitOK)
	}
}

func TestExitCodeOf_UserCanceled(t *testing.T) {
	err := &cli.CLIError{Code: "USER_CANCELED"}
	if code := cli.ExitCodeOf(err); code != cli.ExitUserCanceled {
		t.Errorf("USER_CANCELED: got %d, want %d", code, cli.ExitUserCanceled)
	}
}

func TestExitCodeOf_SkillBlocked(t *testing.T) {
	err := &cli.CLIError{Code: "SKILL_IMPORT_BLOCKED"}
	if code := cli.ExitCodeOf(err); code != cli.ExitConflictBlocked {
		t.Errorf("SKILL_IMPORT_BLOCKED: got %d, want %d", code, cli.ExitConflictBlocked)
	}
}

func TestExitCodeOf_AuthError(t *testing.T) {
	err := &cli.CLIError{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	if code := cli.ExitCodeOf(err); code != cli.ExitAuthError {
		t.Errorf("401 UNAUTHENTICATED: got %d, want %d", code, cli.ExitAuthError)
	}
}

func TestExitCodeOf_NetworkError(t *testing.T) {
	err := &cli.CLIError{Code: "NETWORK_ERROR"}
	if code := cli.ExitCodeOf(err); code != cli.ExitNetworkError {
		t.Errorf("NETWORK_ERROR: got %d, want %d", code, cli.ExitNetworkError)
	}
}

func TestExitCodeOf_4xx(t *testing.T) {
	err := &cli.CLIError{Code: "NOT_FOUND", HTTPStatus: 404}
	if code := cli.ExitCodeOf(err); code != cli.ExitBackendBizError {
		t.Errorf("404: got %d, want %d", code, cli.ExitBackendBizError)
	}
}

func TestExitCodeOf_5xx(t *testing.T) {
	err := &cli.CLIError{Code: "INTERNAL", HTTPStatus: 500}
	if code := cli.ExitCodeOf(err); code != cli.ExitNetworkError {
		t.Errorf("500: got %d, want %d", code, cli.ExitNetworkError)
	}
}

func TestCLIError_Error(t *testing.T) {
	err := &cli.CLIError{Code: "NOT_FOUND", HTTPStatus: 404, Message: "resource not found"}
	s := err.Error()
	if s == "" {
		t.Error("Error() returned empty string")
	}
}
