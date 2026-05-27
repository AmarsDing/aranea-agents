// Package cli re-exports the CLIError type and exit code constants from clierr
// for use in the public API.
package cli

import "aranea-agents/internal/cli/clierr"

// Re-exported exit code constants.
const (
	ExitOK              = clierr.ExitOK
	ExitUsage           = clierr.ExitUsage
	ExitBackendBizError = clierr.ExitBackendBizError
	ExitNetworkError    = clierr.ExitNetworkError
	ExitUserCanceled    = clierr.ExitUserCanceled
	ExitConflictBlocked = clierr.ExitConflictBlocked
	ExitAuthError       = clierr.ExitAuthError
	ExitInterrupted     = clierr.ExitInterrupted
)

// CLIError is the standard error type (alias to clierr.CLIError).
type CLIError = clierr.CLIError

// ExitCodeOf maps an error to an OS exit code.
func ExitCodeOf(err error) int {
	return clierr.ExitCodeOf(err)
}
