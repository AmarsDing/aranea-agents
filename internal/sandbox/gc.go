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
		}
	}
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
