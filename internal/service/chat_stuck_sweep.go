package service

import (
	"time"

	rt "aranea-agents/internal/runtime"
)

// sweepStuckRuns runs the TD-T1 stuck-run heartbeat sweep (2026-10-19):
// active runs whose runner produced no framework events for longer than
// rt.DefaultStuckRunTimeout are cancelled through the regular Cancel path.
// Wired into the orchestrator sweep loop (5min period). Safety invariants
// (awaiting_user exclusion, placeholder/runner-less skip, no blind kills)
// live in runtime.RunRegistry.SweepStuckRuns.
func (o *ChatOrchestrator) sweepStuckRuns() {
	if o == nil || o.runs == nil {
		return
	}
	o.runs.SweepStuckRuns(time.Now(), rt.DefaultStuckRunTimeout)
}
