package biz

import (
	"context"
	"errors"
	"time"
)

// ErrSIMergeConflict marks an ApplyCodeMerge failure caused by repository
// drift (the patch no longer applies on current HEAD). The apply usecase
// downgrades such runs to the manual approval channel (design D7:
// 冲突则转人工). Detect with errors.Is.
var ErrSIMergeConflict = errors.New("self-improvement merge conflict")

// ── Self-improvement run persistence ports (73-self-iteration-v3, design §4.1) ──

// SelfImprovementRunReader reads platform self-improvement runs.
// GetByID/GetBySuggestionID return (nil, nil) when absent.
// Stability:evolving
type SelfImprovementRunReader interface {
	GetByID(ctx context.Context, id string) (*SelfImprovementRun, error)
	GetBySuggestionID(ctx context.Context, suggestionID string) (*SelfImprovementRun, error)
	List(ctx context.Context, filter RunFilter) ([]SelfImprovementRun, error)
	// Count returns the number of runs matching the filter's status /
	// risk_level / trigger_source conditions; Limit/Offset are ignored
	// (console list total, P5).
	Count(ctx context.Context, filter RunFilter) (int, error)
	// ListTerminalPendingOutcome returns terminal runs (closed / rolled_back /
	// verify_failed / rejected / failed) that have no PatchOutcome yet, oldest
	// first (Outcome worker attribution scan, D8).
	ListTerminalPendingOutcome(ctx context.Context, limit int) ([]SelfImprovementRun, error)
}

// SelfImprovementRunWriter writes platform self-improvement runs.
// Stability:evolving
type SelfImprovementRunWriter interface {
	Create(ctx context.Context, run *SelfImprovementRun) error
	// Update persists the full mutable state of run (run.Status is the target
	// state) guarded by CAS: the row must currently be in `from` status.
	// Zero affected rows → CodeConflict.
	Update(ctx context.Context, run *SelfImprovementRun, from SelfImprovementRunStatus) error
	// RecordAttempt increments attempts (patch-verify retry counter).
	RecordAttempt(ctx context.Context, id string) error
}

// PatchOutcomeWriter writes terminal attribution records (Learn stage).
// Method names carry the Outcome prefix so one repo struct can implement both
// SelfImprovementRunWriter and PatchOutcomeWriter without method collisions.
// Stability:evolving
type PatchOutcomeWriter interface {
	CreateOutcome(ctx context.Context, outcome *PatchOutcome) error
	ListOutcomesByRun(ctx context.Context, runID string) ([]PatchOutcome, error)
	// ListRecentOutcomesByTrigger returns the newest outcomes for one trigger
	// source (join through runs), newest first (trigger feedback adaptation,
	// D8).
	ListRecentOutcomesByTrigger(ctx context.Context, triggerSource string, limit int) ([]PatchOutcome, error)
}

// SITriggerVerdictCount is one (trigger_source, verdict) aggregate row of
// patch_outcomes joined through runs (console outcome stats, P5).
type SITriggerVerdictCount struct {
	TriggerSource string
	Verdict       SelfImprovementVerdict
	Count         int
}

// PatchOutcomeStatsReader aggregates terminal attribution records for the
// console stats panel (P5, design §7 GetOutcomeStats).
// Stability:evolving
type PatchOutcomeStatsReader interface {
	AggregateOutcomeStats(ctx context.Context) ([]SITriggerVerdictCount, error)
}

// SIRiskRuleRepo persists the admin-configurable risk-classification rules
// (P5 console, design §7 UpdateRiskRules) on the system_settings singleton.
// Zero fields mean "inherit code defaults" (see SIRiskRules).
// Stability:evolving
type SIRiskRuleRepo interface {
	GetSIRiskRules(ctx context.Context) (SIRiskRules, error)
	UpdateSIRiskRules(ctx context.Context, rules SIRiskRules) (SIRiskRules, error)
}

// ── Sandbox port (73-self-iteration-v3, design §4.2 / D4) ────────────────────

// RepoSandbox prepares an isolated git worktree for one self-improvement run,
// applies the candidate patch, and executes verification gates G1-G3 inside
// it. Implemented by service.RepoSandboxRunner.
//
// Lifecycle: PrepareWorktree → ResetWorktree → ProbeTestFailures (G2 HEAD
// baseline) → ApplyDiff → RunGate(...) → cleanup(). The cleanup func
// returned by PrepareWorktree must be idempotent
// and must release the worktree even when the caller's ctx is already
// cancelled.
// Stability:evolving
type RepoSandbox interface {
	// PrepareWorktree creates worktree <worktree_root>/<runID> on branch
	// self-improve/<runID> based at baseRef. baseRef empty means HEAD.
	PrepareWorktree(ctx context.Context, runID, baseRef string) (path string, cleanup func(), err error)
	// ResetWorktree restores the worktree to its pristine base-ref state
	// (git reset --hard + clean -fd). The pipeline calls it before every
	// Patcher attempt so a retry never sees the previous attempt's applied
	// diff — otherwise the LLM reads polluted file contents, the new diff
	// stacks on top of the old one, and verification passes a code state
	// that run.Diff alone does not reproduce on the live repo.
	ResetWorktree(ctx context.Context, path string) error
	// ApplyDiff applies a unified diff inside the worktree at path.
	ApplyDiff(ctx context.Context, path, diff string) error
	// RunGate executes one verification gate inside the worktree.
	// pkgs carries the affected go package patterns (DeriveAffectedScopes).
	// Empty pkgs: G1 still builds ./... (caller must omit G1 when there is no
	// Go scope); G2/G3 return Skipped and must NOT fall back to ./....
	// G3 prefers golangci-lint when on PATH, otherwise go vet.
	// SandboxGateWebLint runs `pnpm lint` in web/ (skipped if missing).
	// Gate processes receive a filtered environment (no production DSN).
	// Output is truncated (64KB) inside the returned SandboxGateResult.
	RunGate(ctx context.Context, path string, gate SandboxGateKind, pkgs []string) (SandboxGateResult, error)
	// ProbeTestFailures snapshots failing test names on the current worktree
	// (intended: clean HEAD, before ApplyDiff). Compile/setup failures yield
	// an empty list so G2 cannot exempt a broken package. A probe exec error
	// is returned; the pipeline then applies no exemptions (fail-closed).
	ProbeTestFailures(ctx context.Context, path string, pkgs []string) (failedTests []string, err error)
}

// ── Apply & metrics ports (73-self-iteration-v3, design §4.4 / D7) ──────────

// SIApplier applies a governed patch to the live repository and rolls it
// back on demand. Implemented by service.SIRepoApplier (git-backed).
//
// Channel semantics (D7):
//   - ApplyHotReload — config/prompt/docs kinds: patch lands on the working
//     tree (effective on next file read). Optional SIRuntimeReloader may
//     refresh in-process caches; without it this is NOT a live runtime
//     reload. The returned rollbackRef locates the pre-apply snapshot.
//   - ApplyCodeMerge — code/test kinds: patch is committed on a
//     self-improve/<runID> branch and left there. The current working
//     branch is not fast-forwarded. Effective only after an explicit
//     operator merge (and then a process restart). Drift while applying
//     the diff returns an error wrapping ErrSIMergeConflict (→ 转人工,
//     design D7).
//   - Rollback — undoes whatever the matching Apply* call did (delete
//     the self-improve branch for code, snapshot restore for working-tree
//     apply). The running HEAD is never reverted.
//
// Stability:evolving
type SIApplier interface {
	ApplyHotReload(ctx context.Context, run *SelfImprovementRun) (rollbackRef string, err error)
	ApplyCodeMerge(ctx context.Context, run *SelfImprovementRun) (commitSHA string, err error)
	Rollback(ctx context.Context, run *SelfImprovementRun, reason string) error
}

// SIRuntimeReloader best-effort refreshes in-process caches after a
// working-tree apply. Nil is allowed — apply then only mutates files.
// Stability:evolving
type SIRuntimeReloader interface {
	ReloadAfterWorkingTreeApply(ctx context.Context, run *SelfImprovementRun) error
}

// SIMetricsReader captures a sliding-window platform metrics snapshot for the
// observing window (Watchdog before/after comparison, D7).
// Stability:evolving
type SIMetricsReader interface {
	// Snapshot aggregates platform metrics over [now-window, now).
	Snapshot(ctx context.Context, window time.Duration) (*MetricsSnapshot, error)
}
