package codeexecutor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/sandbox"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// sandboxArtifactDir is the in-sandbox artifact mount (M32 contract parity:
// agent code writes output files here; they are collected after the run).
const sandboxArtifactDir = "/workspace/out"

// sandboxArtifactMaxBytes caps the cumulative artifact payload pulled out of
// one sandbox run (r2 #5, 64 MiB — generous vs the 10 MiB per-file ceiling
// applied later by CollectOutputDirFiles).
const sandboxArtifactMaxBytes = 64 << 20

// sessionRenewExtend slides a session-pinned lease forward after each
// successful ExecuteCode (capped by the manager's max TTL).
const sessionRenewExtend = 30 * time.Minute

// pooledRuntime executes code inside a pooled sandbox lease supplied by the
// caller (pooledAdapter owns lease lifecycle: ephemeral per-call or
// session-pinned via the shared sandbox.SessionLeases). It replaces the per-run
// create/start/rm cycle of DockerExecutor with warm-pool acquisition while
// keeping identical output semantics (Result + artifact dir).
type pooledRuntime struct {
	mgr     *sandbox.Manager
	tempDir string
}

func newPooledRuntime(mgr *sandbox.Manager) *pooledRuntime {
	return &pooledRuntime{mgr: mgr, tempDir: os.TempDir()}
}

// Run executes one code block on the given lease. Attribution (agent key /
// run id) is decided at Acquire time by pooledAdapter — this layer only
// consumes the already-attributed lease.
func (e *pooledRuntime) Run(ctx context.Context, lease *sandbox.Lease, language, code string, timeout time.Duration) (Result, error) {
	ext, runner := languageRuntime(language)
	if ext == "" {
		return Result{}, fmt.Errorf("codeexecutor sandbox: unsupported language %q", language)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	// Prepare local artifact collection dir. Ownership transfers to the caller
	// (pooledAdapter removes the temp tree after collection) — same contract
	// as the fixed DockerExecutor.
	tmpDir, err := os.MkdirTemp(e.tempDir, "codeexec-*")
	if err != nil {
		return Result{}, err
	}
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return Result{}, err
	}

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
		_ = os.RemoveAll(tmpDir) // no artifacts to hand out on exec failure
		return Result{}, fmt.Errorf("codeexecutor sandbox exec: %w", err)
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
// untars it into outDir. Path traversal entries are rejected. The cumulative
// payload is capped (r2 #5): a runaway sandbox writing unbounded output must
// not OOM the host during collection.
func collectSandboxArtifacts(ctx context.Context, lease *sandbox.Lease, outDir string) error {
	rc, err := lease.CopyDirFrom(ctx, sandboxArtifactDir)
	if err != nil {
		return err
	}
	defer rc.Close()
	return sandbox.UntarFiles(rc, sandboxArtifactMaxBytes, func(relPath string, content []byte) error {
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
// Lease lifecycle (P1-1): a non-empty session key pins one lease to the
// session for its whole lifetime (multi-turn fs/process state, A3); an empty
// key falls back to ephemeral per-call acquire/release.
type pooledAdapter struct {
	runtime *pooledRuntime
	timeout time.Duration
	store   *sandbox.SessionLeases
}

func newPooledAdapter(mgr *sandbox.Manager, timeout time.Duration, store *sandbox.SessionLeases) *pooledAdapter {
	return &pooledAdapter{runtime: newPooledRuntime(mgr), timeout: timeout, store: store}
}

func (a *pooledAdapter) CodeBlockDelimiter() trpcagentcodeexec.CodeBlockDelimiter {
	return trpcagentcodeexec.CodeBlockDelimiter{Start: "```", End: "```"}
}

// sessionKey resolves the lease key: the invocation session (app/user/session)
// always wins — the sandbox_fs tool family keys on it, so code exec must land
// on the SAME shared sandbox (review 2026-08-26 r2 #1: a model-controlled
// execution_id could otherwise collide across sessions or split the two
// consumers apart). The model-supplied ExecutionID is only a no-invocation
// compatibility fallback (tests / non-session callers).
func (a *pooledAdapter) sessionKey(ctx context.Context, executionID string) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if ok && inv != nil && inv.Session != nil && inv.Session.ID != "" {
		s := inv.Session
		return s.AppName + "/" + s.UserID + "/" + s.ID
	}
	return strings.TrimSpace(executionID)
}

// agentKeyFromCtx derives the per-agent quota attribution key (review
// 2026-08-26 #1: Acquire previously never set AgentKey, leaving the per-agent
// concurrency gate dead in production). "" without an invocation context —
// the global gate still bounds that case.
func agentKeyFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	return inv.AgentName
}

func (a *pooledAdapter) ExecuteCode(ctx context.Context, input trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	key := a.sessionKey(ctx, input.ExecutionID)
	if key == "" || a.store == nil {
		return a.executeEphemeral(ctx, input)
	}
	// Session-pinned path: one retry after evicting a stale (manager-GC'd)
	// lease; the fresh lease starts with empty state per the use-and-destroy
	// contract.
	for attempt := 0; attempt < 2; attempt++ {
		lease, err := a.store.Acquire(ctx, key, agentKeyFromCtx(ctx))
		if err != nil {
			return trpcagentcodeexec.CodeExecutionResult{}, fmt.Errorf("sandbox executor acquire: %w", err)
		}
		res, err := a.runBlocks(ctx, lease, input.CodeBlocks)
		if err == nil {
			a.store.Renew(ctx, key, sessionRenewExtend)
			return res, nil
		}
		if errors.Is(err, sandbox.ErrNotFound) && attempt == 0 {
			a.store.Evict(key, lease)
			continue
		}
		return trpcagentcodeexec.CodeExecutionResult{}, err
	}
	return trpcagentcodeexec.CodeExecutionResult{}, fmt.Errorf("sandbox executor: unreachable")
}

// executeEphemeral keeps the P0 one-shot semantics for callers without a
// session context: one lease per ExecuteCode, destroyed on return.
func (a *pooledAdapter) executeEphemeral(ctx context.Context, input trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	lease, err := a.runtime.mgr.Acquire(ctx, sandbox.AcquireReq{
		Profile:  sandbox.DefaultProfileName,
		TTL:      a.timeout + time.Minute, // exec window + collection margin
		AgentKey: agentKeyFromCtx(ctx),    // review 2026-08-26 #1: per-agent quota
		// review 2026-08-26 #4: team runs tag the ctx — ephemeral creations
		// must count against the run budget exactly like session-pinned ones.
		RunID: sandbox.RunIDFromContext(ctx),
	})
	if err != nil {
		return trpcagentcodeexec.CodeExecutionResult{}, fmt.Errorf("sandbox executor acquire: %w", err)
	}
	defer func() { _ = lease.Release(context.Background()) }()
	return a.runBlocks(ctx, lease, input.CodeBlocks)
}

func (a *pooledAdapter) runBlocks(ctx context.Context, lease *sandbox.Lease, blocks []trpcagentcodeexec.CodeBlock) (trpcagentcodeexec.CodeExecutionResult, error) {
	var sb strings.Builder
	var outFiles []trpcagentcodeexec.File
	for i, block := range blocks {
		res, err := a.runtime.Run(ctx, lease, block.Language, block.Code, a.timeout)
		if err != nil {
			return trpcagentcodeexec.CodeExecutionResult{}, fmt.Errorf("sandbox executor block %d: %w", i, err)
		}
		sb.WriteString(formatBlockOutput(res))
		if res.ArtifactDir != "" {
			outFiles = append(outFiles, CollectOutputDirFiles(res.ArtifactDir, DefaultMaxOutputFileBytes)...)
			// Caller-owned artifact temp tree (…/out parent) — remove after collection.
			_ = os.RemoveAll(filepath.Dir(res.ArtifactDir))
		}
	}
	return trpcagentcodeexec.CodeExecutionResult{Output: sb.String(), OutputFiles: outFiles}, nil
}
