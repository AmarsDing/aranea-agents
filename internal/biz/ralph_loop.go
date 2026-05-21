package biz

import (
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// RalphLoopConfigured reports whether any Ralph Loop field is set on runtime settings.
func RalphLoopConfigured(s *AgentRuntimeSettings) bool {
	if s == nil {
		return false
	}
	promise := strings.TrimSpace(s.RalphLoopCompletionPromise)
	verify := strings.TrimSpace(s.RalphLoopVerifyCommand)
	return s.RalphLoopMaxIterations > 0 || promise != "" || verify != ""
}

// ValidateRalphLoopSettings rejects invalid Ralph Loop combinations at the persistence boundary.
// When not configured, validation is a no-op.
func ValidateRalphLoopSettings(s *AgentRuntimeSettings) error {
	if s == nil || !RalphLoopConfigured(s) {
		return nil
	}
	promise := strings.TrimSpace(s.RalphLoopCompletionPromise)
	verify := strings.TrimSpace(s.RalphLoopVerifyCommand)
	if promise == "" && verify == "" {
		return errors.BadRequest("AGENT", "ralph loop requires completion_promise or verify_command when enabled")
	}
	if s.RalphLoopMaxIterations < 0 {
		return errors.BadRequest("AGENT", "ralph_loop_max_iterations must be >= 0")
	}
	if s.RalphLoopVerifyTimeoutSeconds < 0 {
		return errors.BadRequest("AGENT", "ralph_loop_verify_timeout_seconds must be >= 0")
	}
	return nil
}
