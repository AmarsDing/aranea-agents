package sandbox

import (
	"context"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/loggateway"
)

// poolLoop is the warm-pool replenisher (design §3.4): every
// ReplenishInterval it tops each profile up to min_ready, never exceeding
// max_ready, and rotates ready instances that aged past MaxPoolAge.
// Ready instances are brand-new every time — the pool never re-admits a
// released sandbox (ADR-82-2).
func (m *Manager) poolLoop(ctx context.Context) {
	m.replenish(ctx) // first fill immediately so Acquire can hit warm
	ticker := time.NewTicker(m.cfg.Pool.ReplenishInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.replenish(ctx)
		}
	}
}

func (m *Manager) replenish(ctx context.Context) {
	for name := range m.cfg.Profiles {
		ready := m.registry.countReady(name)
		deficit := m.cfg.Pool.MinReady - ready
		if deficit <= 0 {
			continue
		}
		// max_ready 防爆 cap.
		if room := m.cfg.Pool.MaxReady - ready; deficit > room {
			deficit = room
		}
		for i := 0; i < deficit; i++ {
			if ctx.Err() != nil {
				return
			}
			if err := m.createPooled(ctx, name); err != nil {
				// Image missing / daemon hiccup: log and retry next tick;
				// cold-create on demand remains as the fallback path.
				m.lg.Warn("sandbox pool replenish failed",
					loggateway.StepID("sandbox.pool_replenish"),
					loggateway.Str("profile", name),
					loggateway.Err(err))
				break
			}
		}
	}
	m.refreshGauges()
}

// createPooled builds one unattributed ready instance for the pool.
func (m *Manager) createPooled(ctx context.Context, profileName string) error {
	profile := m.cfg.Profiles[profileName]
	sandboxID := "sbx-" + uuid.NewString()
	labels := map[string]string{
		LabelSandbox: "1",
		LabelID:      sandboxID,
		LabelProfile: profileName,
	}
	h, err := m.engine.Create(ctx, sandboxID, profile, labels)
	if err != nil {
		return err
	}
	m.registry.register(&entry{
		view: LeaseView{
			SandboxID: sandboxID,
			Profile:   profileName,
			State:     StateReady,
			CreatedAt: m.now(),
		},
		handle:     h,
		entryState: StateReady,
	})
	return nil
}

// rotatePoolAges destroys ready instances older than MaxPoolAge so the pool
// does not保温 drifting environments (design §3.4). Called from the GC loop.
func (m *Manager) rotatePoolAges() {
	now := m.now()
	for _, e := range m.registry.listEntries() {
		if e.entryState != StateReady {
			continue
		}
		if now.Sub(e.view.CreatedAt) > m.cfg.Pool.MaxPoolAge {
			m.destroy(e.view.SandboxID, ReasonPoolEvict)
		}
	}
}
