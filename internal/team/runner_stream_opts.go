package team

import (
	"aranea-agents/internal/agent"
)

// newStreamConsumeOptions returns stream consume options via the injected factory.
// SetStreamOptsFactory must be called before use; the factory is always wired
// in service/chat_orchestrator.go via chatactivity.StreamOptsFactoryAdapter.
func (r *Runner) newStreamConsumeOptions() *agent.StreamConsumeOptions {
	if r.streamOptsFactory == nil {
		return nil
	}
	return r.streamOptsFactory.NewStreamConsumeOptions()
}
