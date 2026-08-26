package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestDockerEngineIntegration exercises the full lifecycle against the real
// daemon: cold acquire -> exec -> release(destroy) and warm replenish -> warm
// acquire. Skipped when no daemon is reachable. Uses the default codeexec
// profile image (python:3.11-slim); pull it beforehand or the test skips.
func TestDockerEngineIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if !DockerDaemonAvailable() {
		t.Skip("docker daemon unavailable")
	}
	cfg := DefaultConfig()
	cfg.Pool.MinReady = 1 // normalize keeps >=1; pool is driven explicitly
	m := NewManager(cfg, NewDockerEngine(), nil)
	ctx := context.Background()
	// Drain any test-created instances so nothing lingers on the shared daemon.
	t.Cleanup(func() {
		for _, v := range m.registry.list() {
			m.destroy(v.SandboxID, ReasonPoolEvict)
		}
		m.Close()
	})

	// --- Cold acquire + exec + release/destroy ---
	lease, err := m.Acquire(ctx, AcquireReq{AgentKey: "itest"})
	if err != nil {
		t.Fatalf("cold Acquire: %v", err)
	}
	res, err := lease.Exec(ctx, ExecSpec{Argv: []string{"python3", "-c", "print('sandbox-ok')"}, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "sandbox-ok" {
		t.Fatalf("stdout=%q, want sandbox-ok (stderr=%q)", res.Stdout, res.Stderr)
	}
	sbxID := lease.SandboxID()
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if containerExists(ctx, t, sbxID) {
		t.Fatalf("container %s still exists after release", sbxID)
	}

	// --- Warm replenish + warm acquire hit ---
	m.replenish(ctx)
	if got := m.registry.countReady(DefaultProfileName); got != 1 {
		t.Fatalf("ready=%d after replenish, want 1", got)
	}
	before := m.st.acquireWarm.Load()
	lease2, err := m.Acquire(ctx, AcquireReq{AgentKey: "itest"})
	if err != nil {
		t.Fatalf("warm Acquire: %v", err)
	}
	if got := m.st.acquireWarm.Load(); got != before+1 {
		t.Fatalf("acquireWarm=%d, want %d (warm hit)", got, before+1)
	}
	res2, err := lease2.Exec(ctx, ExecSpec{Argv: []string{"sh", "-c", "head -1 /etc/os-release"}, Timeout: 20 * time.Second})
	if err != nil || res2.ExitCode != 0 {
		t.Fatalf("warm Exec: err=%v exit=%d stderr=%q", err, res2.ExitCode, res2.Stderr)
	}
	if err := lease2.Release(ctx); err != nil {
		t.Fatalf("Release warm: %v", err)
	}
	// Pool must not regain the destroyed instance; replenish replaces it.
	m.replenish(ctx)
	if got := m.registry.countReady(DefaultProfileName); got != 1 {
		t.Fatalf("ready=%d after re-replenish, want 1", got)
	}
}

func containerExists(ctx context.Context, t *testing.T, id string) bool {
	t.Helper()
	eng := NewDockerEngine()
	out, err := eng.cmd(ctx, "ps", "-aq", "--filter", "name=^/"+id+"$").CombinedOutput()
	if err != nil {
		t.Fatalf("docker ps: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)) != ""
}
