package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Compile-time check: SIRepoApplier implements the biz.SIApplier port.
var _ biz.SIApplier = (*SIRepoApplier)(nil)

const (
	siApplierDomain = "SELF_IMPROVEMENT"
	// siSnapshotRefPrefix prefixes rollbackRef values of the hot-reload
	// channel: "snapshot/<safeRunID>" (design D7, rollback_pointer column).
	siSnapshotRefPrefix = "snapshot/"
	// siSnapshotDirName holds pre-apply file snapshots under the worktree
	// root (gitignored, design D4).
	siSnapshotDirName = "snapshots"
)

// siApplierGitIdentity pins the author of applier-created commits via
// per-command -c flags (repo git config is never mutated).
var siApplierGitIdentity = []string{
	"-c", "user.name=aranea-self-improvement",
	"-c", "user.email=self-improvement@aranea.local",
}

// SIRepoApplier applies governed patches to the live repository (design D7).
//
// Channels:
//   - Hot-reload (config/prompt/docs): the diff lands on the main working
//     tree after a pre-apply file snapshot; Rollback restores the snapshot.
//   - Code merge (code/test): the diff is committed inside a fresh worktree
//     and fast-forward merged into the current branch; Rollback git-reverts
//     the merge commit.
//
// All git operations against the main repository are serialized (mu) so
// concurrent runs cannot interleave merges / working-tree patches.
type SIRepoApplier struct {
	sandbox *RepoSandboxRunner
	lg      loggateway.Logger
	mu      sync.Mutex
}

// NewSIRepoApplier binds the applier to the sandbox runner owning the
// repository worktree domain.
func NewSIRepoApplier(sandbox *RepoSandboxRunner, lg loggateway.Logger) (*SIRepoApplier, error) {
	if sandbox == nil {
		return nil, apierror.BadRequest(siApplierDomain, "sandbox runner is required")
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SIRepoApplier{
		sandbox: sandbox,
		lg:      lg.With(loggateway.Domain(siApplierDomain)),
	}, nil
}

// ApplyHotReload snapshots every diff target, then applies the diff to the
// main working tree. On success it returns the rollback pointer locating the
// snapshot; a failed apply leaves neither working-tree changes nor snapshots.
func (a *SIRepoApplier) ApplyHotReload(ctx context.Context, run *biz.SelfImprovementRun) (string, error) {
	paths, err := siApplyPrecheck(run)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	snapDir, ref, err := a.takeSnapshot(run.ID, paths)
	if err != nil {
		return "", err
	}
	if err := a.sandbox.runGit(ctx, a.sandbox.repoRoot, strings.NewReader(run.Diff), "apply", "--whitespace=nowarn", "-"); err != nil {
		_ = os.RemoveAll(snapDir)
		a.lg.Warn("self-improvement hot-reload apply failed",
			loggateway.StepID("si_apply.hot_reload"),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err))
		return "", err
	}
	a.lg.Info("self-improvement hot-reload applied",
		loggateway.StepID("si_apply.hot_reload"),
		loggateway.Str("run_id", run.ID),
		loggateway.Int("files", len(paths)))
	return ref, nil
}

// ApplyCodeMerge commits the diff on a fresh self-improve/<runID> worktree
// branch (based at current HEAD) and fast-forward merges it into the main
// branch. Repository drift (context mismatch / non-ff) yields an error
// wrapping biz.ErrSIMergeConflict; the repository is left untouched.
func (a *SIRepoApplier) ApplyCodeMerge(ctx context.Context, run *biz.SelfImprovementRun) (string, error) {
	if _, err := siApplyPrecheck(run); err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	wtPath, cleanup, err := a.sandbox.PrepareWorktree(ctx, run.ID, "")
	if err != nil {
		return "", err
	}
	defer cleanup()

	if err := a.sandbox.ApplyDiff(ctx, wtPath, run.Diff); err != nil {
		a.lg.Warn("self-improvement code merge: patch no longer applies (drift)",
			loggateway.StepID("si_apply.code_merge"),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err))
		return "", apierror.Wrap(fmt.Errorf("%w: %s", biz.ErrSIMergeConflict, err), apierror.CodeConflict, siApplierDomain)
	}
	if err := a.sandbox.runGit(ctx, wtPath, nil, "add", "-A"); err != nil {
		return "", err
	}
	commitArgs := append(append([]string{}, siApplierGitIdentity...), "commit", "-m", siCommitMessage(run))
	if err := a.sandbox.runGit(ctx, wtPath, nil, commitArgs...); err != nil {
		return "", err
	}
	sha, err := a.gitOutput(ctx, wtPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	branch := "self-improve/" + sanitizeSandboxRunID(run.ID)
	if err := a.sandbox.runGit(ctx, a.sandbox.repoRoot, nil, "merge", "--ff-only", branch); err != nil {
		a.lg.Warn("self-improvement code merge: fast-forward failed",
			loggateway.StepID("si_apply.code_merge"),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err))
		return "", apierror.Wrap(fmt.Errorf("%w: %s", biz.ErrSIMergeConflict, err), apierror.CodeConflict, siApplierDomain)
	}
	a.lg.Info("self-improvement code patch merged",
		loggateway.StepID("si_apply.code_merge"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("commit", sha))
	return sha, nil
}

// Rollback undoes whatever the matching Apply* call did: git revert for a
// merged code commit (run.AppliedCommit), snapshot restore for a hot-reload
// patch (run.RollbackPointer with the snapshot/ prefix).
func (a *SIRepoApplier) Rollback(ctx context.Context, run *biz.SelfImprovementRun, reason string) error {
	if run == nil || strings.TrimSpace(run.ID) == "" {
		return apierror.BadRequest(siApplierDomain, "run with id is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	switch {
	case strings.TrimSpace(run.AppliedCommit) != "":
		args := append(append([]string{}, siApplierGitIdentity...), "revert", "--no-edit", run.AppliedCommit)
		if err := a.sandbox.runGit(ctx, a.sandbox.repoRoot, nil, args...); err != nil {
			a.lg.Warn("self-improvement code revert failed",
				loggateway.StepID("si_apply.rollback"),
				loggateway.Str("run_id", run.ID),
				loggateway.Str("commit", run.AppliedCommit),
				loggateway.Err(err))
			return err
		}
		a.lg.Info("self-improvement code patch reverted",
			loggateway.StepID("si_apply.rollback"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("commit", run.AppliedCommit),
			loggateway.Str("reason", reason))
		return nil
	case strings.HasPrefix(run.RollbackPointer, siSnapshotRefPrefix):
		if err := a.restoreSnapshot(run.RollbackPointer); err != nil {
			a.lg.Warn("self-improvement hot-reload rollback failed",
				loggateway.StepID("si_apply.rollback"),
				loggateway.Str("run_id", run.ID),
				loggateway.Err(err))
			return err
		}
		a.lg.Info("self-improvement hot-reload patch rolled back",
			loggateway.StepID("si_apply.rollback"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("reason", reason))
		return nil
	default:
		return apierror.BadRequest(siApplierDomain,
			"run %s has neither applied_commit nor a snapshot rollback pointer", run.ID)
	}
}

// ── snapshot machinery (hot-reload channel) ──────────────────────────────────

// siSnapshotManifest records the pre-apply state of every diff target.
type siSnapshotManifest struct {
	RunID  string   `json:"run_id"`
	Stored []string `json:"stored"` // pre-apply content saved under files/
	Absent []string `json:"absent"` // did not exist pre-apply → removed on rollback
}

func (a *SIRepoApplier) snapshotRoot() string {
	return filepath.Join(a.sandbox.repoRoot, a.sandbox.worktreeRoot, siSnapshotDirName)
}

// takeSnapshot copies the pre-apply content of every target path into the
// per-run snapshot dir and writes the manifest. It returns the snapshot dir
// (for failure cleanup) and the rollback pointer.
func (a *SIRepoApplier) takeSnapshot(runID string, paths []string) (string, string, error) {
	safe := sanitizeSandboxRunID(runID)
	if safe == "" {
		return "", "", apierror.BadRequest(siApplierDomain, "run id %q is not usable as a snapshot key", runID)
	}
	dir := filepath.Join(a.snapshotRoot(), safe)
	m := siSnapshotManifest{RunID: runID}
	for _, p := range paths {
		content, err := os.ReadFile(filepath.Join(a.sandbox.repoRoot, filepath.FromSlash(p)))
		switch {
		case err == nil:
			dst := filepath.Join(dir, "files", filepath.FromSlash(p))
			if werr := os.MkdirAll(filepath.Dir(dst), 0o755); werr != nil {
				_ = os.RemoveAll(dir)
				return "", "", apierror.Wrap(werr, apierror.CodeInternal, siApplierDomain)
			}
			if werr := os.WriteFile(dst, content, 0o644); werr != nil {
				_ = os.RemoveAll(dir)
				return "", "", apierror.Wrap(werr, apierror.CodeInternal, siApplierDomain)
			}
			m.Stored = append(m.Stored, p)
		case errors.Is(err, os.ErrNotExist):
			m.Absent = append(m.Absent, p)
		default:
			_ = os.RemoveAll(dir)
			return "", "", apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", "", apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return "", "", apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
	}
	return dir, siSnapshotRefPrefix + safe, nil
}

// restoreSnapshot writes back pre-apply content, removes patch-created files,
// and consumes the snapshot directory.
func (a *SIRepoApplier) restoreSnapshot(ref string) error {
	safe := strings.TrimPrefix(ref, siSnapshotRefPrefix)
	if safe == "" || sanitizeSandboxRunID(safe) != safe {
		return apierror.BadRequest(siApplierDomain, "invalid rollback pointer %q", ref)
	}
	dir := filepath.Join(a.snapshotRoot(), safe)
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return apierror.NotFound(siApplierDomain, "snapshot %q not found (already rolled back?)", ref)
	}
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
	}
	var m siSnapshotManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
	}
	for _, p := range m.Stored {
		if err := validateRepoRelativePath(p); err != nil {
			return err
		}
		content, err := os.ReadFile(filepath.Join(dir, "files", filepath.FromSlash(p)))
		if err != nil {
			return apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
		}
		abs := filepath.Join(a.sandbox.repoRoot, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			return apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
		}
	}
	for _, p := range m.Absent {
		if err := validateRepoRelativePath(p); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(a.sandbox.repoRoot, filepath.FromSlash(p))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
		}
	}
	return os.RemoveAll(dir)
}

// ── shared helpers ───────────────────────────────────────────────────────────

// siApplyPrecheck validates the run and extracts the validated, deduped
// repo-relative target path list from the diff.
func siApplyPrecheck(run *biz.SelfImprovementRun) ([]string, error) {
	if run == nil || strings.TrimSpace(run.ID) == "" {
		return nil, apierror.BadRequest(siApplierDomain, "run with id is required")
	}
	if strings.TrimSpace(run.Diff) == "" {
		return nil, apierror.BadRequest(siApplierDomain, "run.diff is empty")
	}
	changes := biz.ParseUnifiedDiffFiles(run.Diff)
	if len(changes) == 0 {
		return nil, apierror.BadRequest(siApplierDomain, "diff touches no files")
	}
	seen := map[string]bool{}
	var paths []string
	add := func(p string) error {
		if err := validateRepoRelativePath(p); err != nil {
			return err
		}
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
		return nil
	}
	for _, c := range changes {
		if c.Kind == biz.PatchChangeRenamed {
			if err := add(c.OldPath); err != nil {
				return nil, err
			}
		}
		if err := add(c.Path); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// validateRepoRelativePath rejects absolute paths, backslashes and parent
// traversal in diff/manifest paths (git-style forward-slash paths only).
func validateRepoRelativePath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsRune(p, '\\') || filepath.IsAbs(p) {
		return apierror.BadRequest(siApplierDomain, "unsafe diff path %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return apierror.BadRequest(siApplierDomain, "diff path %q escapes the repository", p)
		}
	}
	return nil
}

// siCommitMessage builds the merge commit message with the design-D7 markers
// (self-improvement trailer + run id).
func siCommitMessage(run *biz.SelfImprovementRun) string {
	subject := ""
	if run.Diagnosis != nil {
		subject = strings.TrimSpace(strings.SplitN(run.Diagnosis.RootCause, "\n", 2)[0])
	}
	if subject == "" {
		subject = "run " + run.ID
	}
	const maxSubject = 72
	if len(subject) > maxSubject {
		subject = subject[:maxSubject-3] + "..."
	}
	return fmt.Sprintf("fix(self-improvement): %s\n\nself-improvement: true\nrun-id: %s\n", subject, run.ID)
}

// gitOutput runs a git command and returns trimmed stdout (rev-parse etc.).
func (a *SIRepoApplier) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", apierror.Wrap(err, apierror.CodeInternal, siApplierDomain)
	}
	return strings.TrimSpace(string(out)), nil
}
