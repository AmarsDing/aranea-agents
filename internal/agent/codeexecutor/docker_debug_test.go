package codeexecutor_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Mirrors executor.go Run exactly (CommandContext + timeout ctx + collectCtx)
// to isolate the divergence from the passing manual sequence.
func TestDockerExecutorDebugVariantCtx(t *testing.T) {
	requireDocker(t)

	container := fmt.Sprintf("cpdbg-ctx-%d", time.Now().UnixNano())
	defer func() {
		_ = exec.Command("docker", "rm", "-fv", container).Run()
	}()

	timeout := 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "codeexec-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatal(err)
	}

	createArgs := []string{
		"create", "--interactive", "--name", container,
		"--network", "none",
		"--memory=268435456", "--memory-swap=268435456", "--cpus=0.5",
		"--read-only", "--tmpfs", "/tmp:size=128m", "--stop-timeout=65",
		"--volume", "/workspace/out",
		"python:3.11-slim", "sh", "-c", "cat > /tmp/main.py && exec python3 /tmp/main.py",
	}
	if out, err := exec.CommandContext(ctx, "docker", createArgs...).CombinedOutput(); err != nil {
		t.Fatalf("create: %v (%s)", err, out)
	}

	code := "import os\nprint('hello-from-sandbox')\nos.makedirs('/workspace/out', exist_ok=True)\nopen('/workspace/out/result.txt','w').write('artifact-42')\n"
	cmd := exec.CommandContext(ctx, "docker", "start", "-ai", container)
	cmd.Stdin = strings.NewReader(code)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	t.Logf("start: err=%v stdout=%q stderr=%q", runErr, stdout.String(), stderr.String())

	collectCtx, collectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer collectCancel()
	out, err := exec.CommandContext(collectCtx, "docker", "cp", container+":/workspace/out/.", outDir).CombinedOutput()
	t.Logf("cp out: err=%v out=%s dest=%s", err, strings.TrimSpace(string(out)), outDir)

	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		t.Logf("collected: %s", e.Name())
	}
	if len(entries) == 0 {
		t.Fatal("no artifacts collected")
	}
}
