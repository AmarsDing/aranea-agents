package jobs

import "context"

// RunOnceExposed exports runOnce for external test packages.
func (c *AutoHealTTLCleanup) RunOnceExposed(ctx context.Context) {
	c.runOnce(ctx)
}

// RunOnceExposed exports runOnce for external test packages.
func (w *MonitorTraceBackfillWorker) RunOnceExposed(ctx context.Context) {
	w.runOnce(ctx)
}
