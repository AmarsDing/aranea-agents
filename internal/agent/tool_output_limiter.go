package agent

import (
	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"
)

const (
	// defaultToolOutputSizeLimit is the fallback AfterTool cap for
	// undecorated string results (deferred / MCP). Decorator-governed
	// tools keep their own ResultBudget / offload envelope and are skipped.
	defaultToolOutputSizeLimit = 50000
)

// newOutputSizeLimiterAfterHook creates an AfterTool hook that truncates
// oversized tool results. It runs at priority 60, after the tool recorder
// (priority 50) so the original size is logged before truncation.
func newOutputSizeLimiterAfterHook(lg loggateway.Logger) callbacks.AfterToolHook {
	hook := tools.NewOutputSizeLimiterHook(defaultToolOutputSizeLimit, lg)
	return callbacks.NewAfterToolHook(60, hook)
}
