package service

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// checkSandboxPath resolves path and ensures it lives under the configured
// worktree root, blocking gate/apply calls against the main repo or arbitrary
// directories.
func (r *RepoSandboxRunner) checkSandboxPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", apierror.BadRequest(apierror.DomainTool, "worktree path is required")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", apierror.Wrap(err, apierror.CodeInternal, apierror.DomainTool)
	}
	sandboxRoot := filepath.Join(r.repoRoot, r.worktreeRoot)
	if abs != sandboxRoot && !strings.HasPrefix(abs, sandboxRoot+string(os.PathSeparator)) {
		return "", apierror.BadRequest(apierror.DomainTool,
			"path %q is outside the sandbox worktree root %q", p, sandboxRoot)
	}
	return abs, nil
}

// worktreeRegistered reports whether wtPath is a currently registered git
// worktree. Conservative: returns true when the listing itself fails, so an
// active worktree is never mistaken for a crash orphan and purged.
func (r *RepoSandboxRunner) worktreeRegistered(ctx context.Context, wtPath string) bool {
	cmd := exec.CommandContext(ctx, "git", "-c", "core.longpaths=true", "worktree", "list", "--porcelain")
	cmd.Dir = r.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	target := filepath.ToSlash(wtPath)
	for line := range strings.Lines(string(out)) {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			if strings.EqualFold(strings.TrimRight(p, "\r\n"), target) {
				return true
			}
		}
	}
	return false
}

// purgeWorktreeOrphan best-effort removes a crash-orphaned branch + leftover
// directory (no registered worktree). Errors are logged, never fatal: the
// subsequent add retry is the real verdict.
func (r *RepoSandboxRunner) purgeWorktreeOrphan(ctx context.Context, branch, wtPath string) {
	if err := r.runGit(ctx, r.repoRoot, nil, "branch", "-D", branch); err != nil {
		r.lg.Debug("self-improve orphan branch delete skipped",
			loggateway.StepID("sandbox.worktree.purge"),
			loggateway.Str("branch", branch),
			loggateway.Err(err))
	}
	if err := os.RemoveAll(wtPath); err != nil {
		r.lg.Warn("self-improve orphan dir removal failed",
			loggateway.StepID("sandbox.worktree.purge"),
			loggateway.Str("path", wtPath),
			loggateway.Err(err))
	}
}

// purgeRegisteredWorktree removes a half-created worktree that IS registered
// (git died after registration, e.g. 2026-08-19 exit 255): worktree remove
// unregisters + deletes the dir, then the branch is dropped. Best-effort like
// purgeWorktreeOrphan; the error return to the caller is the real verdict.
func (r *RepoSandboxRunner) purgeRegisteredWorktree(ctx context.Context, branch, wtPath string) {
	if err := r.runGit(ctx, r.repoRoot, nil, "worktree", "remove", "--force", wtPath); err != nil {
		r.lg.Warn("self-improve half-created worktree removal failed",
			loggateway.StepID("sandbox.worktree.purge"),
			loggateway.Str("path", wtPath),
			loggateway.Err(err))
	}
	r.purgeWorktreeOrphan(ctx, branch, wtPath)
}

// runGit executes a git command with optional stdin and wraps failures with
// the captured output for diagnostics.
func (r *RepoSandboxRunner) runGit(ctx context.Context, dir string, stdin *strings.Reader, args ...string) error {

	args = append([]string{"-c", "core.longpaths=true"}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = stdin
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainTool).
			WithMeta("git_args", strings.Join(args, " ")).
			WithMeta("git_output", truncateGateOutput(string(out)))
	}
	return nil
}

// sanitizeSandboxRunID maps a run ID to a safe path/branch component. Dots
// are folded too: ".." is illegal inside git ref names, and run IDs are
// UUIDs that never carry meaningful dots. Consecutive separators collapse
// into one so hostile input cannot smuggle ref sequences or path traversal.
func sanitizeSandboxRunID(runID string) string {
	mapped := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, strings.TrimSpace(runID))
	for strings.Contains(mapped, "--") {
		mapped = strings.ReplaceAll(mapped, "--", "-")
	}
	return strings.Trim(mapped, "-")
}

// truncateGateOutput keeps the head of gate output within the 64KB report
// budget, appending a truncation marker.
func truncateGateOutput(out string) string {
	if len(out) <= maxGateOutputBytes {
		return out
	}
	return out[:maxGateOutputBytes] + "\n...[truncated]"
}

// appendGateOutputNote appends a status note within the output budget.
func appendGateOutputNote(out, note string) string {
	if len(out)+len(note)+1 > maxGateOutputBytes+64 {
		out = truncateGateOutput(out)
	}
	if out == "" {
		return note
	}
	return out + "\n" + note
}

// exitCodeString extracts a human-readable exit description.
func exitCodeString(err error) string {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return fmt.Sprintf("%d", exitErr.ExitCode())
	}
	return "error: " + err.Error()
}
