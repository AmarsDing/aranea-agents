// Package clierr defines the shared CLIError type and exit code constants used
// throughout the CLI stack without introducing import cycles.
package clierr

import (
	"errors"
	"fmt"
)

// Exit code constants aligned with PRD §5.2.
const (
	ExitOK              = 0
	ExitUsage           = 1 // cobra parse error / bad args
	ExitBackendBizError = 2 // 4xx except 401/403
	ExitNetworkError    = 3 // DNS/TLS/5xx/unreachable
	ExitUserCanceled    = 4 // user typed n / Ctrl+C in prompt
	ExitConflictBlocked = 5 // SKILL_IMPORT_BLOCKED or warn+non-interactive
	ExitAuthError       = 6 // 401/403
	ExitInterrupted     = 130
)

// CLIError is the standard error type returned throughout the CLI stack.
type CLIError struct {
	Code       string // backend reason or CLI-defined code
	HTTPStatus int    // 0 = non-HTTP error
	Message    string
	Hint       string
	Metadata   map[string]any
	Cause      error
}

func (e *CLIError) Error() string {
	if e.HTTPStatus > 0 {
		return fmt.Sprintf("%s (HTTP %d): %s", e.Code, e.HTTPStatus, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *CLIError) Unwrap() error { return e.Cause }

// ExitCodeOf maps an error to an OS exit code.
func ExitCodeOf(err error) int {
	if err == nil {
		return ExitOK
	}
	var ce *CLIError
	if !errors.As(err, &ce) {
		// Non-CLIError: likely a transport/network error (DNS, TLS, connection refused).
		return ExitNetworkError
	}

	// HTTP-response errors: use status code first.
	if ce.HTTPStatus > 0 {
		switch {
		case ce.HTTPStatus == 401 || ce.HTTPStatus == 403:
			return ExitAuthError
		case ce.HTTPStatus >= 500:
			return ExitNetworkError
		case ce.HTTPStatus >= 400:
			return ExitBackendBizError
		}
	}

	// Non-HTTP (programmatic) errors: use code.
	switch ce.Code {
	case "USER_CANCELED":
		return ExitUserCanceled
	case "SKILL_IMPORT_BLOCKED", "CONFIRMATION_REQUIRED":
		return ExitConflictBlocked
	case "UNAUTHENTICATED", "UNAUTHORIZED", "FORBIDDEN", "LOGIN_NO_TOKEN":
		return ExitAuthError
	case "INSECURE_CONFIG_PERM", "CONFIG_INVALID",
		"FILE_READ_ERROR", "FILE_PARSE_ERROR", "MISSING_CONTENT", "MISSING_ARGS",
		"CONFIG_KEY_UNKNOWN", "CONFIG_VALUE_INVALID", "CONFIG_SAVE_ERROR",
		"PKG_MANIFEST_INVALID":
		return ExitUsage
	case "NETWORK_ERROR", "PKG_FETCH_ERROR", "PKG_MANIFEST_ERROR", "PKG_INSTALL_ERROR",
		"IMPORT_LOAD_ERROR", "IMPORT_WRITE_ERROR", "IMPORT_APPLY_ERROR":
		return ExitNetworkError
	}

	return ExitBackendBizError
}
