package agent

import (
	"aranea-agents/internal/agent/adksvc"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
)

// NewADKRunner is the single helper for constructing a production Runner against biz session + optional memory + default plugins.
func NewADKRunner(root adkagent.Agent, sessSvc session.Service, mem memory.Service) (*runner.Runner, error) {
	return runner.New(runner.Config{
		AppName:           adksvc.DefaultAppName,
		Agent:             root,
		SessionService:    sessSvc,
		MemoryService:     mem,
		AutoCreateSession: false,
		PluginConfig: runner.PluginConfig{
			Plugins: DefaultRunnerPlugins(),
		},
	})
}
