package monitor

import (
	"context"
)

// NoopHealActionHandler is a HealActionHandler that logs the action but does
// not execute it. It is used when the runtime handles healing (the default
// since the migration to SelfHealObserver).
type NoopHealActionHandler struct{}

// HandleFixAction logs the fix action and returns nil (no-op).
func (h *NoopHealActionHandler) HandleFixAction(_ context.Context, _ FixAction, _ map[string]any) error {
	return nil
}
