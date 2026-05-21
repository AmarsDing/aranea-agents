package agent

import (
	"strings"
	"time"

	"aranea-agents/internal/biz"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

const (
	defaultRalphPromiseTagOpen  = "<promise>"
	defaultRalphPromiseTagClose = "</promise>"
)

// RalphLoopTurnResult holds resolved Ralph Loop config for one turn.
type RalphLoopTurnResult struct {
	Config  *trpcrunner.RalphLoopConfig
	SkipErr error // set when settings are partially configured but invalid
}

// ResolveRalphLoopTurn maps runtime settings to framework config for chat/team/A2A turns.
// Callers should log and skip when SkipErr is non-nil.
func ResolveRalphLoopTurn(s *biz.AgentRuntimeSettings) RalphLoopTurnResult {
	cfg, err := RalphLoopConfigFromSettings(s)
	if err != nil {
		return RalphLoopTurnResult{SkipErr: err}
	}
	return RalphLoopTurnResult{Config: cfg}
}

// RalphLoopConfigFromSettings maps agent_runtime_settings Ralph fields to a
// framework RalphLoopConfig. Returns nil when Ralph Loop is not configured.
func RalphLoopConfigFromSettings(s *biz.AgentRuntimeSettings) (*trpcrunner.RalphLoopConfig, error) {
	if s == nil || !biz.RalphLoopConfigured(s) {
		return nil, nil
	}
	if err := biz.ValidateRalphLoopSettings(s); err != nil {
		return nil, err
	}
	promise := strings.TrimSpace(s.RalphLoopCompletionPromise)
	verifyCmd := strings.TrimSpace(s.RalphLoopVerifyCommand)
	timeout := time.Duration(s.RalphLoopVerifyTimeoutSeconds) * time.Second
	tagOpen := strings.TrimSpace(s.RalphLoopPromiseTagOpen)
	if tagOpen == "" {
		tagOpen = defaultRalphPromiseTagOpen
	}
	tagClose := strings.TrimSpace(s.RalphLoopPromiseTagClose)
	if tagClose == "" {
		tagClose = defaultRalphPromiseTagClose
	}
	return &trpcrunner.RalphLoopConfig{
		MaxIterations:     s.RalphLoopMaxIterations,
		CompletionPromise: promise,
		PromiseTagOpen:    tagOpen,
		PromiseTagClose:   tagClose,
		VerifyCommand:     verifyCmd,
		VerifyWorkDir:     strings.TrimSpace(s.RalphLoopVerifyWorkDir),
		VerifyTimeout:     timeout,
	}, nil
}
