package agent

import (
	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"
)

const (
	// defaultToolOutputSizeLimit is the default maximum character count for
	// tool results. Results exceeding this limit are truncated with a marker.
	// This prevents oversized tool outputs from overflowing the LLM context
	// window while preserving the most relevant content.
	defaultToolOutputSizeLimit = 50000
)

// newOutputSizeLimiterAfterHook creates an AfterTool hook that truncates
// oversized tool results. It runs at priority 60, after the tool recorder
// (priority 50) so the original size is logged before truncation.
func newOutputSizeLimiterAfterHook(lg loggateway.Logger) callbacks.AfterToolHook {
	hook := tools.NewOutputSizeLimiterHook(defaultToolOutputSizeLimit, lg)
	return callbacks.NewAfterToolHook(60, hook)
}
