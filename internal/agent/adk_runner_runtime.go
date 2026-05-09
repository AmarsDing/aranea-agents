package agent

import (
	"aranea-agents/internal/adkdeps"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
)

// RunnerMemoryForRuntime returns the ADK memory.Service that matches [NewADKRunnerForRuntime]
// (in-memory when rt or SessionMemory is nil).
func RunnerMemoryForRuntime(rt *adkdeps.Runtime) memory.Service {
	if rt != nil && rt.SessionMemory != nil {
		return RunnerMemoryService(rt.SessionMemory)
	}
	return RunnerMemoryService(nil)
}

// NewADKRunnerForRuntime builds a production Runner with memory derived from the Wire-injected runtime bundle.
// When rt is nil or rt.SessionMemory is nil, ADK uses in-process memory only.
func NewADKRunnerForRuntime(root adkagent.Agent, sessSvc session.Service, rt *adkdeps.Runtime) (*runner.Runner, error) {
	return NewADKRunner(root, sessSvc, RunnerMemoryForRuntime(rt))
}
