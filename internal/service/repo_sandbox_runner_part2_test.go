package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRepoSandboxRunner_G3Vet(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-g3", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	res, err := r.RunGate(context.Background(), path, biz.SandboxGateLint, []string{"./..."})
	if err != nil {
		t.Fatalf("RunGate G3: %v", err)
	}
	if !res.Passed {
		t.Errorf("G3 vet should pass on clean fixture, output: %s", res.Output)
	}
}

func TestRepoSandboxRunner_G2G3EmptyPkgsSkipped(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-empty-pkgs", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	for _, gate := range []biz.SandboxGateKind{biz.SandboxGateTest, biz.SandboxGateLint} {
		res, err := r.RunGate(context.Background(), path, gate, nil)
		if err != nil {
			t.Fatalf("RunGate %s: %v", gate, err)
		}
		if !res.Skipped || res.Passed {
			t.Errorf("%s empty pkgs = %+v, want skipped", gate, res)
		}
		if !strings.Contains(res.Output, "refusing repo-wide") {
			t.Errorf("%s skip reason = %q", gate, res.Output)
		}
	}
}

func TestSandboxGateEnv_StripsProductionSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/aranea")
	t.Setenv("PGPASSWORD", "secret")
	t.Setenv("MY_API_KEY", "abc")
	t.Setenv("CUSTOM_DSN", "postgres://x")
	t.Setenv("PATH", os.Getenv("PATH"))
	t.Setenv("CGO_ENABLED", "1")
	t.Setenv("CC", "gcc")

	env := sandboxGateEnv()
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "ARANEA_SI_SANDBOX=1") {
		t.Error("sandbox marker missing")
	}
	if !strings.Contains(joined, "CGO_ENABLED=1") || !strings.Contains(joined, "CC=gcc") {
		t.Error("CGO toolchain env must pass through (go-sqlite3 / cgo packages)")
	}
	for _, leak := range []string{"DATABASE_URL=", "PGPASSWORD=", "MY_API_KEY=", "CUSTOM_DSN=", "postgres://"} {
		if strings.Contains(joined, leak) {
			t.Errorf("sandbox env leaked %q", leak)
		}
	}
}

func TestRepoSandboxRunner_UnsupportedGate(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r := newTestRunner(t, repo)
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-g4", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	if _, err := r.RunGate(context.Background(), path, biz.SandboxGateCritic, nil); err == nil {
		t.Error("G4/G5 are not runner gates — want explicit error")
	}
}

func TestRepoSandboxRunner_GateTimeout(t *testing.T) {
	repo := initFixtureGoRepo(t)
	r, err := NewRepoSandboxRunner(repo, loggateway.NewNoop(),
		WithGateTimeout(biz.SandboxGateTest, time.Nanosecond))
	if err != nil {
		t.Fatalf("NewRepoSandboxRunner: %v", err)
	}
	path, cleanup, err := r.PrepareWorktree(context.Background(), "run-timeout", "")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	defer cleanup()

	res, err := r.RunGate(context.Background(), path, biz.SandboxGateTest, []string{"./..."})
	if err != nil {
		t.Fatalf("RunGate timeout: %v", err)
	}
	if res.Passed {
		t.Error("1ns timeout must fail the gate")
	}
}

func TestTruncateGateOutput(t *testing.T) {
	big := strings.Repeat("x", maxGateOutputBytes+100)
	got := truncateGateOutput(big)
	if len(got) > maxGateOutputBytes+64 {
		t.Errorf("truncateGateOutput len = %d, want <= %d", len(got), maxGateOutputBytes+64)
	}
	if !strings.HasPrefix(got, strings.Repeat("x", 100)) {
		t.Error("truncateGateOutput should keep the head of the output")
	}
	if got == big {
		t.Error("no truncation happened")
	}
}

// interface guard
var _ biz.RepoSandbox = (*RepoSandboxRunner)(nil)
