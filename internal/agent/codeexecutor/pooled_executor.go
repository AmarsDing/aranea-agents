package codeexecutor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/sandbox"

	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// sandboxArtifactDir is the in-sandbox artifact mount (M32 contract parity:
// agent code writes output files here; they are collected after the run).
const sandboxArtifactDir = "/workspace/out"

// pooledRuntime executes code inside a pooled sandbox lease: Acquire → exec →
// collect artifacts → Release (destroy). It replaces the per-run
// create/start/rm cycle of DockerExecutor with warm-pool acquisition while
// keeping identical output semantics (Result + artifact dir).
type pooledRuntime struct {
	mgr     *sandbox.Manager
	tempDir string
}

func newPooledRuntime(mgr *sandbox.Manager) *pooledRuntime {
	return &pooledRuntime{mgr: mgr, tempDir: os.TempDir()}
}

// Run executes one code block in a fresh lease. Attribution fields are left
// empty at P0: the per-agent quota gate is keyed on agent identity which the
// factory API does not thread yet (P1-1 session stickiness will); the global
// gate (32) still bounds total concurrency.
func (e *pooledRuntime) Run(ctx context.Context, language, code string, timeout time.Duration) (Result, error) {
	ext, runner := languageRuntime(language)
	if ext == "" {
		return Result{}, fmt.Errorf("codeexecutor sandbox: unsupported language %q", language)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	// Prepare local artifact collection dir (same lifecycle as DockerExecutor).
	tmpDir, err := os.MkdirTemp(e.tempDir, "codeexec-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmpDir)
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Result{}, err
	}

	lease, err := e.mgr.Acquire(ctx, sandbox.AcquireReq{
		Profile: sandbox.DefaultProfileName,
		TTL:     timeout + time.Minute, // exec window + collection margin
	})
	if err != nil {
		return Result{}, fmt.Errorf("codeexecutor sandbox acquire: %w", err)
	}
	defer func() { _ = lease.Release(context.Background()) }()

	// Same streaming contract as the docker backend: stdin lands on tmpfs via
	// cat, then the interpreter replaces the shell (PID signal hygiene).
	script := fmt.Sprintf("cat > /tmp/main%s && exec %s /tmp/main%s", ext, runner, ext)
	res, err := lease.Exec(ctx, sandbox.ExecSpec{
		Argv:    []string{"sh", "-c", script},
		Stdin:   code,
		Timeout: timeout,
	})
	out := Result{
		Stdout:      res.Stdout,
		Stderr:      res.Stderr,
		ExitCode:    res.ExitCode,
		TimedOut:    res.TimedOut,
		OOM:         res.OOM,
		ArtifactDir: outDir,
	}
	if err != nil {
		return out, fmt.Errorf("codeexecutor sandbox exec: %w", err)
	}

	// Collect artifacts best-effort (detached ctx: the caller ctx may already
	// be cancelled by the execution timeout). Failures surface in Stderr so a
	// lost artifact is never silent — M32 parity.
	collectCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if cerr := collectSandboxArtifacts(collectCtx, lease, outDir); cerr != nil {
		out.Stderr += fmt.Sprintf("\n[artifact collect failed: %v]", cerr)
	}
	return out, nil
}

// collectSandboxArtifacts streams /workspace/out out of the sandbox and
// untars it into outDir. Path traversal entries are rejected.
func collectSandboxArtifacts(ctx context.Context, lease *sandbox.Lease, outDir string) error {
	rc, err := lease.CopyDirFrom(ctx, sandboxArtifactDir)
	if err != nil {
		return err
	}
	defer rc.Close()
	return sandbox.UntarFiles(rc, func(relPath string, content []byte) error {
		clean := filepath.Clean(relPath)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return nil // skip traversal attempts silently
		}
		dest := filepath.Join(outDir, clean)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, content, 0o644)
	})
}

// pooledAdapter adapts pooledRuntime to the framework CodeExecutor interface.
type pooledAdapter struct {
	runtime *pooledRuntime
	timeout time.Duration
}

func newPooledAdapter(mgr *sandbox.Manager, timeout time.Duration) *pooledAdapter {
	return &pooledAdapter{runtime: newPooledRuntime(mgr), timeout: timeout}
}

func (a *pooledAdapter) CodeBlockDelimiter() trpcagentcodeexec.CodeBlockDelimiter {
	return trpcagentcodeexec.CodeBlockDelimiter{Start: "```", End: "```"}
}

func (a *pooledAdapter) ExecuteCode(ctx context.Context, input trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	var sb strings.Builder
	var outFiles []trpcagentcodeexec.File
	for i, block := range input.CodeBlocks {
		res, err := a.runtime.Run(ctx, block.Language, block.Code, a.timeout)
		if err != nil {
			return trpcagentcodeexec.CodeExecutionResult{}, fmt.Errorf("sandbox executor block %d: %w", i, err)
		}
		sb.WriteString(formatBlockOutput(res))
		if res.ArtifactDir != "" {
			outFiles = append(outFiles, CollectOutputDirFiles(res.ArtifactDir, DefaultMaxOutputFileBytes)...)
		}
	}
	return trpcagentcodeexec.CodeExecutionResult{Output: sb.String(), OutputFiles: outFiles}, nil
}
