package sandbox

import (
	"context"
	"time"
)

// gcLoop is the lifecycle reaper (design §3.3): every GCInterval it destroys
// leased sandboxes past their TTL deadline (reason=ttl), leased sandboxes
// idle past IdleTimeout (reason=idle), and rotates aged pool instances
// (reason=pool_evict, via rotatePoolAges).
func (m *Manager) gcLoop(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.GCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.gcOnce()
			// 周期 light 对账折叠进既有 GC tick（review §12.1-⑤：不新增
			// goroutine、默认 60s、env 可关）。
			m.maybeLightReconcile(ctx)
		}
	}
}

// maybeLightReconcile runs the runtime light reconcile at most once per
// reconcileInterval (0/negative = disabled). It piggybacks on the GC ticker
// so no extra goroutine exists; destruction of registry-tracked entries never
// happens here — only orphaned daemon instances are reaped directly, and
// registry entries stay on the destroy() CAS single-fire path.
func (m *Manager) maybeLightReconcile(ctx context.Context) {
	if m.reconcileInterval <= 0 {
		return
	}
	now := m.now()
	if !m.lastReconcile.IsZero() && now.Sub(m.lastReconcile) < m.reconcileInterval {
		return
	}
	m.lastReconcile = now
	m.lightReconcile(ctx)
}

func (m *Manager) gcOnce() {
	now := m.now()
	for _, e := range m.registry.listEntries() {
		switch e.entryState {
		case StateLeased:
			if now.After(e.view.Deadline) {
				m.destroy(e.view.SandboxID, ReasonTTL)
				continue
			}
			last := e.view.LastExecAt
			if last.IsZero() {
				last = e.view.CreatedAt
			}
			if m.cfg.TTL.IdleTimeout > 0 && now.Sub(last) > m.cfg.TTL.IdleTimeout {
				m.destroy(e.view.SandboxID, ReasonIdle)
			}
		}
	}
	m.rotatePoolAges()
}
