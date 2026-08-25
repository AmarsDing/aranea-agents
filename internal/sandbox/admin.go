package sandbox

import (
	"context"
	"sort"
	"time"

	"aranea-agents/internal/biz"
)

// Admin surface: implements biz.SandboxAdminPort (design §5.3). Read-only
// plus ForceKill; exec/files are intentionally NOT exposed (ADR-82-3).

// AdminSandboxList returns all live sandboxes (ready + leased).
func (m *Manager) AdminSandboxList(ctx context.Context) ([]biz.SandboxView, error) {
	views := m.registry.list()
	out := make([]biz.SandboxView, 0, len(views))
	for _, v := range views {
		out = append(out, biz.SandboxView{
			ID:         v.SandboxID,
			Profile:    v.Profile,
			AgentKey:   v.AgentKey,
			SessionID:  v.SessionID,
			RunID:      v.RunID,
			State:      string(v.State),
			CreatedAt:  rfc3339(v.CreatedAt),
			Deadline:   rfc3339(v.Deadline),
			LastExecAt: rfc3339(v.LastExecAt),
			ExecCount:  v.ExecCount,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// AdminSandboxForceKill destroys one sandbox with mandatory operator/reason
// audit (P2 wires the HTTP endpoint; the port is complete from P0).
func (m *Manager) AdminSandboxForceKill(ctx context.Context, id, reason, operator string) error {
	return m.ForceKill(id, reason, operator)
}

// AdminSandboxMetrics returns the pool water levels + counter snapshot.
func (m *Manager) AdminSandboxMetrics(ctx context.Context) (*biz.SandboxMetrics, error) {
	snap := m.st.take()
	leased := m.registry.countLeasedByProfile()
	globalActive, _ := m.quota.Snapshot()

	profiles := make([]biz.SandboxProfileMetrics, 0, len(m.cfg.Profiles))
	for name := range m.cfg.Profiles {
		profiles = append(profiles, biz.SandboxProfileMetrics{
			Profile: name,
			Ready:   m.registry.countReady(name),
			Active:  leased[name],
		})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Profile < profiles[j].Profile })

	return &biz.SandboxMetrics{
		Profiles:     profiles,
		GlobalActive: globalActive,
		AcquireWarm:  snap.AcquireWarm,
		AcquireCold:  snap.AcquireCold,
		AcquireFail:  snap.AcquireFail,
		ExecOK:       snap.ExecOK,
		ExecError:    snap.ExecError,
		ExecTimeout:  snap.ExecTimeout,
		Destroy:      snap.Destroy,
		QuotaReject:  snap.QuotaReject,
	}, nil
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// compile-time assertion: Manager satisfies the biz admin port.
var _ biz.SandboxAdminPort = (*Manager)(nil)
