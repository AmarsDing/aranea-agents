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
// The container is always removed after execution (--rm).
func (e *DockerExecutor) Run(ctx context.Context, language, code string, timeout time.Duration) (Result, error) {
	ext, _ := languageRuntime(language)
	if ext == "" {
		return Result{}, fmt.Errorf("codeexecutor docker: unsupported language %q", language)
	}

	// Write code to a temp file that will be bind-mounted into the container.
	tmpDir, err := os.MkdirTemp(e.tempDir, "codeexec-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmpDir)

	codeFile := filepath.Join(tmpDir, "main"+ext)
	if err := os.WriteFile(codeFile, []byte(code), 0644); err != nil {
		return Result{}, err
	}

	// Prepare output directory for artifact collection.
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return Result{}, err
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := e.buildDockerArgs(codeFile, outDir, language, timeout)
	cmd := exec.CommandContext(ctx, "docker", args...)
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

func (e *DockerExecutor) buildDockerArgs(codeFile, outDir, language string, timeout time.Duration) []string {
	container := fmt.Sprintf("codeexec-%d", time.Now().UnixNano())
	stopTimeout := int(timeout.Seconds()) + 5
	if stopTimeout < 10 {
		stopTimeout = 10
	}

	args := []string{
		"run",
		"--rm",
		"--name", container,
		"--network", e.cfg.Network,
		fmt.Sprintf("--memory=%d", e.cfg.MemoryBytes),
		fmt.Sprintf("--memory-swap=%d", e.cfg.MemoryBytes), // disable swap
		fmt.Sprintf("--cpus=%g", e.cfg.CPUs),
		"--read-only",
		"--tmpfs", "/tmp:size=" + e.cfg.TmpSize,
		fmt.Sprintf("--stop-timeout=%d", stopTimeout),
		// Bind-mount code file read-only.
		"--volume", codeFile + ":/workspace/main" + filepath.Ext(codeFile) + ":ro",
		// Bind-mount output dir writable.
		"--volume", outDir + ":/workspace/out:rw",
	}

	// Optional read-only workspace mount.
	if e.cfg.WorkspaceMount != "" {
		args = append(args, "--volume", e.cfg.WorkspaceMount+":/data:ro")
	}

	args = append(args, e.cfg.Image)

	// Build command based on language.
	switch language {
	case "python", "python3":
		args = append(args, "python3", "/workspace/main.py")
	case "javascript", "js", "node":
		args = append(args, "node", "/workspace/main.js")
	case "bash", "sh", "shell":
		args = append(args, "bash", "/workspace/main.sh")
	case "ruby":
		args = append(args, "ruby", "/workspace/main.rb")
	default:
		args = append(args, "sh", "-c", "echo 'unsupported language'")
	}

	return args
}
