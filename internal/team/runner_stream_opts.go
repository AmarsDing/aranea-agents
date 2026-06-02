package team

import (
	"aranea-agents/internal/agent"
)

// newStreamConsumeOptions returns stream consume options via the injected factory.
// RunnerConfig.StreamOptsFactory must be set before use; the factory is always wired
// in service/chat_orchestrator.go via chatactivity.StreamOptsFactoryAdapter.
func (r *Runner) newStreamConsumeOptions() *agent.StreamConsumeOptions {
	if r.cfg.StreamOptsFactory == nil {
		return nil
	}
	return r.cfg.StreamOptsFactory.NewStreamConsumeOptions()
}
