package codeexecutor_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/agent/codeexecutor"
)

// Integration tests run against the real docker daemon on the host.
// They guard the contract that must hold both on the host and inside the
// aranea-admin container (where the CLI talks to the host daemon over
// /var/run/docker.sock and client-side paths are invisible to the daemon).

func requireDocker(t *testing.T) {
	t.Helper()
	codeexecutor.ResetDockerProbe()
	if !codeexecutor.DockerAvailable() {
		t.Skip("docker daemon unavailable")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not in PATH")
	}
}

func TestDockerExecutorRunPythonIntegration(t *testing.T) {
	requireDocker(t)

	execr := codeexecutor.NewDockerExecutor(codeexecutor.DefaultDockerConfig())
	code := "import os\nprint('hello-from-sandbox')\nos.makedirs('/workspace/out', exist_ok=True)\nopen('/workspace/out/result.txt','w').write('artifact-42')\n"
	res, err := execr.Run(context.Background(), "python", code, 60*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Stdout, "hello-from-sandbox") {
		t.Fatalf("expected stdout to contain greeting, got %q (stderr %q)", res.Stdout, res.Stderr)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", res.ExitCode)
	}
	data, err := os.ReadFile(filepath.Join(res.ArtifactDir, "result.txt"))
	if err != nil {
		t.Fatalf("expected collected artifact under ArtifactDir: %v (stderr %q)", err, res.Stderr)
	}
	if string(data) != "artifact-42" {
		t.Fatalf("artifact content mismatch: %q", data)
	}
}

func TestDockerExecutorNonZeroExitIntegration(t *testing.T) {
	requireDocker(t)

	execr := codeexecutor.NewDockerExecutor(codeexecutor.DefaultDockerConfig())
	res, err := execr.Run(context.Background(), "python", "import sys; sys.stderr.write('boom\\n'); sys.exit(3)", 60*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 3 {
		t.Fatalf("expected exit 3, got %d", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "boom") {
		t.Fatalf("expected stderr to contain boom, got %q", res.Stderr)
	}
}

func TestDockerExecutorLeavesNoContainerIntegration(t *testing.T) {
	requireDocker(t)

	execr := codeexecutor.NewDockerExecutor(codeexecutor.DefaultDockerConfig())
	_, err := execr.Run(context.Background(), "python", "print('cleanup-check')", 60*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, err := exec.Command("docker", "ps", "-a", "--filter", "name=codeexec-", "--format", "{{.Names}}").Output()
	if err != nil {
		t.Fatalf("docker ps: %v", err)
	}
	if names := strings.TrimSpace(string(out)); names != "" {
		t.Fatalf("expected no leftover codeexec containers, got %q", names)
	}
}
