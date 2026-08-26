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
// profile image (aranea-sandbox-base:local, P1-3); build it beforehand via
// docker/sandbox-base/build-sandbox-base.ps1 or the create step fails.
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
	// r2 #2: WriteFile lands via exec stdin with the container user's
	// ownership — code exec (same uid) must be able to overwrite AND delete
	// it (the old tar/docker-cp path wrote root-owned files).
	if err := lease.WriteFile(ctx, "/tmp/owner.txt", []byte("v1")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	resOwn, err := lease.Exec(ctx, ExecSpec{Argv: []string{"sh", "-c", "echo v2 > /tmp/owner.txt && rm /tmp/owner.txt && echo rw-ok"}, Timeout: 20 * time.Second})
	if err != nil || resOwn.ExitCode != 0 || strings.TrimSpace(resOwn.Stdout) != "rw-ok" {
		t.Fatalf("WriteFile ownership: err=%v exit=%d stdout=%q stderr=%q", err, resOwn.ExitCode, resOwn.Stdout, resOwn.Stderr)
	}
	// r2 #6: ReadFile round-trip + truncation + directory error.
	if err := lease.WriteFile(ctx, "/tmp/read.txt", []byte("0123456789")); err != nil {
		t.Fatalf("WriteFile read.txt: %v", err)
	}
	data, truncated, err := lease.ReadFile(ctx, "/tmp/read.txt", 4)
	if err != nil || string(data) != "0123" || !truncated {
		t.Fatalf("ReadFile truncate: data=%q truncated=%v err=%v", data, truncated, err)
	}
	if _, _, err := lease.ReadFile(ctx, "/tmp", 0); err != ErrNotRegular {
		t.Fatalf("ReadFile dir: err=%v, want ErrNotRegular", err)
	}
	// Missing file on tmpfs must surface ErrNotFound (r3 review #1: the
	// exec-based read's sentinel exit 16 gets real-daemon regression cover).
	if _, _, err := lease.ReadFile(ctx, "/tmp/nope.txt", 0); err != ErrNotFound {
		t.Fatalf("ReadFile missing: err=%v, want ErrNotFound", err)
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

// TestDockerEgressNetworkLifecycle exercises the per-sandbox egress lane
// (review 2026-08-26 #3): Acquire provisions a dedicated internal network
// shared only with the proxy, the whitelist CONNECT path works through it,
// and Release reclaims the network. Requires the compose egress-proxy
// container (docker/dev-up.ps1); skipped otherwise.
func TestDockerEgressNetworkLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if !DockerDaemonAvailable() {
		t.Skip("docker daemon unavailable")
	}
	ctx := context.Background()
	eng := NewDockerEngine()
	if out, _ := eng.cmd(ctx, "ps", "-q", "--filter", "name=^/aranea-egress-proxy$").CombinedOutput(); strings.TrimSpace(string(out)) == "" {
		t.Skip("egress proxy not running (start the stack via docker/dev-up.ps1)")
	}

	cfg := DefaultConfig()
	cfg.Pool.MinReady = 0
	cfg.Profiles["eg"] = Profile{Name: "eg", Image: cfg.Profiles[DefaultProfileName].Image, Network: NetworkEgress}
	m := NewManager(cfg, eng, nil)
	t.Cleanup(func() {
		for _, v := range m.registry.list() {
			m.destroy(v.SandboxID, ReasonPoolEvict)
		}
		m.Close()
	})

	lease, err := m.Acquire(ctx, AcquireReq{AgentKey: "itest", Profile: "eg"})
	if err != nil {
		t.Fatalf("egress Acquire: %v", err)
	}
	e, ok := m.registry.get(lease.SandboxID())
	if !ok || e.handle.EgressNet == "" {
		t.Fatal("egress network not recorded on the handle")
	}
	netName := e.handle.EgressNet
	if !networkExists(ctx, t, netName) {
		t.Fatalf("egress network %s missing while leased", netName)
	}
	// The lane's only way out is the CONNECT proxy: whitelisted pypi.org
	// must succeed (proxy env is injected by Create).
	res, err := lease.Exec(ctx, ExecSpec{
		Argv:    []string{"python3", "-c", "import urllib.request; print(urllib.request.urlopen('https://pypi.org/simple/', timeout=25).status)"},
		Timeout: 40 * time.Second,
	})
	if err != nil {
		t.Fatalf("egress exec: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "200" {
		t.Fatalf("whitelist via proxy=%q, want 200 (stderr=%q)", res.Stdout, res.Stderr)
	}

	if err := lease.Release(ctx); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if networkExists(ctx, t, netName) {
		t.Fatalf("egress network %s still exists after release", netName)
	}
}

func networkExists(ctx context.Context, t *testing.T, name string) bool {
	t.Helper()
	eng := NewDockerEngine()
	// NB: network names carry no leading "/" (unlike containers), and the
	// name filter is a substring match — compare the output for equality.
	out, err := eng.cmd(ctx, "network", "ls", "--filter", "name="+name, "--format", "{{.Name}}").CombinedOutput()
	if err != nil {
		t.Fatalf("docker network ls: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)) == name
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
