package turn

import (
	rt "aranea-agents/internal/runtime"
)

// RunRegistryAdapter adapts runtime.RunRegistry to ActiveRunRegistry.
type RunRegistryAdapter struct {
	Registry *rt.RunRegistry
}

func (a RunRegistryAdapter) HasActive(sessionID string) bool {
	if a.Registry == nil {
		return false
	}
	return a.Registry.HasActive(sessionID)
}

func (a RunRegistryAdapter) HasActiveRunner(sessionID string) bool {
	if a.Registry == nil {
		return false
	}
	_, _, ok := a.Registry.ActiveRunner(sessionID)
	return ok
}
