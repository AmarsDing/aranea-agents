package sandbox

import (
	"context"

	"aranea-agents/pkg/loggateway"
)

// reconcileOnce is the startup对账 (design §3.3): every container carrying
// the aranea.sandbox=1 label that is not in the (post-restart, always empty)
// registry is an orphan from a previous process lifetime and is destroyed
// (reason=reconcile). Label scan is the only source of truth (ADR-82-4).
func (m *Manager) reconcileOnce(ctx context.Context) {
	handles, err := m.engine.ListByLabels(ctx, map[string]string{LabelSandbox: "1"})
	if err != nil {
		// A failed scan must not block startup; the GC loop still bounds
		// registry-tracked leases and the next restart retries.
		m.lg.Warn("sandbox reconcile scan failed — orphans may linger until next restart",
			loggateway.StepID("sandbox.reconcile"),
			loggateway.Err(err))
		return
	}
	reaped := 0
	for _, h := range handles {
		if _, ok := m.registry.get(h.SandboxID); ok {
			continue
		}
		cleanCtx, cancel := context.WithTimeout(ctx, destroyTimeout)
		if err := m.engine.Destroy(cleanCtx, h); err != nil {
			// Keep counting the reap attempt (the summary log stays the
			// headline), but surface daemon failures individually (r2 #4).
			m.lg.Warn("sandbox reconcile destroy failed",
				loggateway.StepID("sandbox.reconcile"),
				loggateway.Str("container", h.ID),
				loggateway.Err(err))
		}
		cancel()
		destroyTotal.WithLabelValues(ReasonReconcile).Inc()
		m.st.destroy.inc(ReasonReconcile)
		reaped++
	}
	// Per-sandbox egress networks are engine-side resources outside the
	// registry (review 2026-08-26 #3): sweep labeled orphans left by a
	// previous process lifetime.
	if nr, ok := m.engine.(NetworkReaper); ok {
		if n, err := nr.ReapOrphanNetworks(ctx, map[string]string{LabelSandbox: "1"}); err != nil {
			m.lg.Warn("sandbox egress network sweep failed — orphan networks may linger until next restart",
				loggateway.StepID("sandbox.reconcile"),
				loggateway.Err(err))
		} else if n > 0 {
			m.lg.Warn("sandbox reconcile reaped orphaned egress networks",
				loggateway.StepID("sandbox.reconcile"),
				loggateway.Int("reaped", n))
		}
	}
	if reaped > 0 {
		m.lg.Warn("sandbox reconcile reaped orphaned instances",
			loggateway.StepID("sandbox.reconcile"),
			loggateway.Int("reaped", reaped))
	} else {
		m.lg.Info("sandbox reconcile clean (no orphans)",
			loggateway.StepID("sandbox.reconcile"))
	}
}
