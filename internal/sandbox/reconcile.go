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

// lightReconcile is the periodic runtime drift sweep folded into the GC tick
// (review 2026-10-21 §2.8 ⑤, 反作用 #4 缓解). It only handles ONE direction:
// daemon instances carrying the sandbox label with NO registry entry are
// orphans from outside this process lifetime (e.g. a half-failed cold create
// whose register step was lost) and are destroyed directly.
//
// The reverse direction — registry-leased entries whose daemon instance is
// gone — is NEVER destroyed here; those are lazily re-converged by
// Lease.Alive()/SessionLeases (§2.5 #4) and any registry-entry teardown stays
// on destroy()'s transitionDestroying CAS (single-fire, A2 对账/destroy CAS
// 并发约束). Reverse drift is surfaced only as the sandbox.reconcile_drift
// warn log + metric, an AS-EVT-01 Informational 级信号（丢失仅降低可观测性，
// 不上 bus、不 BlockUpTo）。
//
// Returns (reaped, drifted) for unit tests.
func (m *Manager) lightReconcile(ctx context.Context) (reaped, drifted int) {
	handles, err := m.engine.ListByLabels(ctx, map[string]string{LabelSandbox: "1"})
	if err != nil {
		m.lg.Warn("sandbox light reconcile scan failed",
			loggateway.StepID("sandbox.reconcile"),
			loggateway.Err(err))
		return 0, 0
	}
	seen := make(map[string]bool, len(handles))
	for _, h := range handles {
		seen[h.SandboxID] = true
		if _, ok := m.registry.get(h.SandboxID); ok {
			continue // registry 权威条目：销毁一律走 destroy() 的 CAS 单发
		}
		cleanCtx, cancel := context.WithTimeout(ctx, destroyTimeout)
		if err := m.engine.Destroy(cleanCtx, h); err != nil {
			m.lg.Warn("sandbox light reconcile destroy failed",
				loggateway.StepID("sandbox.reconcile"),
				loggateway.Str("container", h.ID),
				loggateway.Err(err))
		}
		cancel()
		destroyTotal.WithLabelValues(ReasonReconcile).Inc()
		m.st.destroy.inc(ReasonReconcile)
		reaped++
	}
	for _, e := range m.registry.listEntries() {
		if e.entryState != StateLeased || seen[e.view.SandboxID] {
			continue
		}
		drifted++
		reconcileDriftTotal.WithLabelValues("daemon_missing").Inc()
		// AS-EVT-01 Informational（§9.3）：仅日志+指标，不发 bus 事件。
		m.lg.Warn("sandbox.reconcile_drift: registry 有租约而 daemon 实例缺失（惰性重租兜底）",
			loggateway.StepID("sandbox.reconcile"),
			loggateway.Str("sandbox_id", e.view.SandboxID),
			loggateway.Str("profile", e.view.Profile),
			loggateway.Str("direction", "daemon_missing"))
	}
	if reaped > 0 {
		m.lg.Warn("sandbox light reconcile reaped orphaned instances",
			loggateway.StepID("sandbox.reconcile"),
			loggateway.Int("reaped", reaped))
	}
	return reaped, drifted
}
