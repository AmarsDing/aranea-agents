// Package codeexecutor provides sandboxed code execution backends.
// The local backend runs code as a child process; the docker backend runs code
// inside a disposable container with strict resource limits.
package codeexecutor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	codeExecRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_codeexec_runs_total",
		Help: "Total code execution runs.",
	}, []string{"kind", "status"})

	codeExecDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_codeexec_duration_seconds",
		Help:    "Code execution duration.",
		Buckets: prometheus.DefBuckets,
	}, []string{"kind"})

	codeExecOOMTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_codeexec_oom_total",
		Help: "OOM kills during code execution.",
	}, []string{"kind"})

	codeExecBlocksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_codeexec_blocks_total",
		Help: "Code blocks executed (one ExecuteCode may include multiple blocks).",
	}, []string{"kind", "status"})
)

// Result holds the outcome of one code execution.
type Result struct {
	Stdout      string
	Stderr      string
	ExitCode    int
	TimedOut    bool
	OOM         bool
	ArtifactDir string // path to output directory (may be empty)
}

// Executor is the interface for running code.
type Executor interface {
	Run(ctx context.Context, language, code string, timeout time.Duration) (Result, error)
}

// LocalConfig holds settings for the local subprocess executor.
type LocalConfig struct {
	TempDir string // scratch directory for code files
}

// LocalExecutor runs code as a local subprocess (no isolation).
type LocalExecutor struct {
	cfg LocalConfig
}

// NewLocalExecutor creates a LocalExecutor.
func NewLocalExecutor(cfg LocalConfig) *LocalExecutor {
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}
	return &LocalExecutor{cfg: cfg}
}

// Run executes code in a temporary file using the host language runtime.
func (e *LocalExecutor) Run(ctx context.Context, language, code string, timeout time.Duration) (Result, error) {
	ext, runner := languageRuntime(language)
	if runner == "" {
		return Result{}, fmt.Errorf("codeexecutor: unsupported language %q", language)
	}

	tmpFile, err := os.CreateTemp(e.cfg.TempDir, "exec-*"+ext)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(tmpFile.Name())
	if _, err := io.WriteString(tmpFile, code); err != nil {
		_ = tmpFile.Close()
		return Result{}, err
	}
	_ = tmpFile.Close()

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, runner, tmpFile.Name())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	res := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		return res, nil
	}
	if err != nil {
		return res, nil
	}
	return res, nil
}

// DockerConfig holds settings for the Docker-based executor.
type DockerConfig struct {
	Image          string
	Network        string  // "none" by default for security
	CPUs           float64 // docker --cpus (default 0.5)
	CPUQuota       int64   // microseconds per period (legacy, unused when CPUs > 0)
	MemoryBytes    int64   // container memory limit in bytes (default 256 MiB)
	TmpSize        string  // tmpfs size for /tmp (default 128m)
	PullPolicy     string  // "never" | "missing" | "always"
	WorkspaceMount string  // host path to mount read-only at /workspace
}

// DefaultDockerConfig returns safe production defaults.
func DefaultDockerConfig() DockerConfig {
	return DockerConfig{
		Image:       "python:3.11-slim",
		Network:     "none",
		CPUs:        0.5,
		CPUQuota:    50000,
		MemoryBytes: 256 * 1024 * 1024, // 256 MiB
		TmpSize:     "128m",
		PullPolicy:  "missing",
	}
}

// DockerExecutor runs code in a disposable Docker container.
type DockerExecutor struct {
	cfg     DockerConfig
	tempDir string
}

// NewDockerExecutor creates a DockerExecutor.
func NewDockerExecutor(cfg DockerConfig) *DockerExecutor {
	if cfg.Image == "" {
		cfg.Image = DefaultDockerConfig().Image
	}
	if cfg.Network == "" {
		cfg.Network = "none"
	}
	if cfg.MemoryBytes <= 0 {
		cfg.MemoryBytes = DefaultDockerConfig().MemoryBytes
	}
	if cfg.CPUs <= 0 {
		cfg.CPUs = DefaultDockerConfig().CPUs
	}
	if cfg.TmpSize == "" {
		cfg.TmpSize = "128m"
	}
	return &DockerExecutor{cfg: cfg, tempDir: os.TempDir()}
}

// Run executes code inside a one-shot Docker container.
//
// The code is streamed to the container over stdin (`docker start -ai`) and
// lands on the tmpfs — never the rootfs — because `docker cp` is refused for
// --read-only containers. Output artifacts go to an anonymous volume so they
// survive container stop (tmpfs contents would be lost) and are collected via
// `docker cp` out, then removed with the volume via `docker rm -v`.
//
// No client-side path is ever bind-mounted: when the CLI talks to the host
// daemon over /var/run/docker.sock (e.g. from inside the aranea-admin
// container), client paths are invisible to the daemon, so bind mounts
// silently break. stdin/volume-cp work regardless of daemon location.
func (e *DockerExecutor) Run(ctx context.Context, language, code string, timeout time.Duration) (Result, error) {
	ext, runner := languageRuntime(language)
	if ext == "" {
		return Result{}, fmt.Errorf("codeexecutor docker: unsupported language %q", language)
	}

	// Prepare local output directory for artifact collection.
	tmpDir, err := os.MkdirTemp(e.tempDir, "codeexec-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmpDir)
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return Result{}, err
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	container := fmt.Sprintf("codeexec-%d", time.Now().UnixNano())
	// Always remove the sandbox (and its anonymous volume) afterwards, using a
	// detached context: ctx may already be cancelled by the execution timeout.
	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanCancel()
		_ = exec.CommandContext(cleanCtx, "docker", "rm", "-fv", container).Run()
	}()

	if out, err := exec.CommandContext(ctx, "docker", e.buildCreateArgs(container, runner, ext, timeout)...).CombinedOutput(); err != nil {
		return Result{}, fmt.Errorf("docker create %s: %w (%s)", container, err, strings.TrimSpace(string(out)))
	}

	cmd := exec.CommandContext(ctx, "docker", "start", "-ai", container)
	cmd.Stdin = strings.NewReader(code)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	res := Result{
		Stdout:      stdout.String(),
		Stderr:      stderr.String(),
		ArtifactDir: outDir,
	}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}

	// Collect artifacts best-effort from the anonymous volume (readable while
	// the container is stopped). Uses a detached context for the same reason
	// as the cleanup above. Failures surface in Stderr so a lost artifact is
	// never silent.
	collectCtx, collectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer collectCancel()
	if out, cerr := exec.CommandContext(collectCtx, "docker", "cp", container+":/workspace/out/.", outDir).CombinedOutput(); cerr != nil {
		res.Stderr += fmt.Sprintf("\n[artifact collect failed: %v (%s)]", cerr, strings.TrimSpace(string(out)))
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		return res, nil
	}

	if res.ExitCode == 137 {
		res.OOM = true
		return res, nil
	}

	if err != nil && res.ExitCode != 0 {
		return res, nil
	}
	return res, nil
}

// buildCreateArgs assembles `docker create` with the same isolation limits as
// the previous `docker run` form. The container command streams the code from
// stdin onto the tmpfs, then execs the interpreter; the container is started
// later via `docker start -ai`.
func (e *DockerExecutor) buildCreateArgs(container, runner, ext string, timeout time.Duration) []string {
	stopTimeout := int(timeout.Seconds()) + 5
	if stopTimeout < 10 {
		stopTimeout = 10
	}

	args := []string{
		"create",
		"--interactive",
		"--name", container,
		"--network", e.cfg.Network,
		fmt.Sprintf("--memory=%d", e.cfg.MemoryBytes),
		fmt.Sprintf("--memory-swap=%d", e.cfg.MemoryBytes), // disable swap
		fmt.Sprintf("--cpus=%g", e.cfg.CPUs),
		"--read-only",
		"--tmpfs", "/tmp:size=" + e.cfg.TmpSize,
		fmt.Sprintf("--stop-timeout=%d", stopTimeout),
		// Anonymous volume for artifact output: writable under --read-only and
		// preserved after stop for `docker cp` collection; removed by rm -v.
		"--volume", "/workspace/out",
	}

	// Optional read-only workspace mount (daemon-side path).
	if e.cfg.WorkspaceMount != "" {
		args = append(args, "--volume", e.cfg.WorkspaceMount+":/data:ro")
	}

	// cat reads the code from stdin (EOF when our side closes); exec replaces
	// the shell so the interpreter is PID 1 and receives stop signals.
	script := fmt.Sprintf("cat > /tmp/main%s && exec %s /tmp/main%s", ext, runner, ext)
	args = append(args, e.cfg.Image, "sh", "-c", script)
	return args
}
