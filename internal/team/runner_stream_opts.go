package team

import (
	"aranea-agents/internal/agent"
	"aranea-agents/internal/chatactivity"
)

// newStreamConsumeOptions returns stream consume options via the injected factory
// if available, otherwise falls back to direct chatactivity construction.
func (r *Runner) newStreamConsumeOptions() *agent.StreamConsumeOptions {
	if r.streamOptsFactory != nil {
		return r.streamOptsFactory.NewStreamConsumeOptions()
	}
	// Fallback: direct construction (to be removed once all callers inject the factory)
	return chatactivity.NewStreamConsumeOptions(r.td.Catalog.ToolUC, r.td.Catalog.Agents, r.td.Sessions)
}
