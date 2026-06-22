package service

import (
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
)

// runRegistryAdapter adapts *rt.RunRegistry to the biz.RunRegistryPort interface.
// This is necessary because rt.RunRegistry.GetStatus returns rt.RunStatusEntry
// while biz.RunRegistryPort.GetStatus returns biz.RunStatusEntry.
type runRegistryAdapter struct {
	inner *rt.RunRegistry
}

// ProvideRunRegistryPort wraps a concrete *rt.RunRegistry as a biz.RunRegistryPort.
func ProvideRunRegistryPort(reg *rt.RunRegistry) biz.RunRegistryPort {
	if reg == nil {
		return nil
	}
	return &runRegistryAdapter{inner: reg}
}

func (a *runRegistryAdapter) Cancel(sessionID, reason string) (bool, string) {
	return a.inner.Cancel(sessionID, reason)
}

func (a *runRegistryAdapter) GetStatus(sessionID string) (biz.RunStatusEntry, bool) {
	entry, ok := a.inner.GetStatus(sessionID)
	if !ok {
		return biz.RunStatusEntry{}, false
	}
	return biz.RunStatusEntry{
		RunID:     entry.RunID,
		Status:    entry.Status,
		ErrMsg:    entry.ErrMsg,
		UpdatedAt: entry.UpdatedAt,
	}, true
}
