package biz

import (
	"context"
	"errors"
	"time"
)

// ErrSIMergeConflict marks an ApplyCodeMerge failure caused by repository
// drift (the patch no longer applies, or the main branch is not
// fast-forwardable). The apply usecase downgrades such runs to the manual
// approval channel (design D7: 冲突则转人工). Detect with errors.Is.
var ErrSIMergeConflict = errors.New("self-improvement merge conflict")

// ── Self-improvement run persistence ports (73-self-iteration-v3, design §4.1) ──

// SelfImprovementRunReader reads platform self-improvement runs.
// GetByID/GetBySuggestionID return (nil, nil) when absent.
// Stability:evolving
// TECH-DEBT(DB-DEBT-02): 6 methods after Count (P5 console pagination),
// exceeding the ≤5 narrow-interface guideline; split deferred with the
// other oversize ports.
type SelfImprovementRunReader interface {
	GetByID(ctx context.Context, id string) (*SelfImprovementRun, error)
	GetBySuggestionID(ctx context.Context, suggestionID string) (*SelfImprovementRun, error)
	List(ctx context.Context, filter RunFilter) ([]SelfImprovementRun, error)
	// Count returns the number of runs matching the filter's status /
	// risk_level / trigger_source conditions; Limit/Offset are ignored
	// (console list total, P5).
	Count(ctx context.Context, filter RunFilter) (int, error)
	// ListObservingDue returns runs in observing status whose observe_until <= now
	// (Watchdog scan).
	ListObservingDue(ctx context.Context, now time.Time) ([]SelfImprovementRun, error)
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
// Lifecycle: PrepareWorktree → ApplyDiff → RunGate(...) → cleanup().
// The cleanup func returned by PrepareWorktree must be idempotent and must
// release the worktree even when the caller's ctx is already cancelled.
// Stability:evolving
type RepoSandbox interface {
	// PrepareWorktree creates worktree <worktree_root>/<runID> on branch
	// self-improve/<runID> based at baseRef. baseRef empty means HEAD.
	PrepareWorktree(ctx context.Context, runID, baseRef string) (path string, cleanup func(), err error)
	// ApplyDiff applies a unified diff inside the worktree at path.
	ApplyDiff(ctx context.Context, path, diff string) error
	// RunGate executes one verification gate (G1 build / G2 test / G3 lint)
	// inside the worktree. pkgs carries the affected go package patterns
	// (DeriveAffectedScopes output); an empty slice means repo-wide default.
	// Output is truncated (64KB) inside the returned SandboxGateResult.
	RunGate(ctx context.Context, path string, gate SandboxGateKind, pkgs []string) (SandboxGateResult, error)
}

// ── Apply & metrics ports (73-self-iteration-v3, design §4.4 / D7) ──────────

// SIApplier applies a governed patch to the live repository and rolls it
// back on demand. Implemented by service.SIRepoApplier (git-backed).
//
// Channel semantics (D7):
//   - ApplyHotReload — config/prompt/docs kinds: patch lands on the working
//     tree (file-level effective on next read); the returned rollbackRef
//     locates the pre-apply snapshot for Rollback.
//   - ApplyCodeMerge — code/test kinds: patch is committed on a
//     self-improve/<runID> branch and fast-forward merged into the current
//     branch; the returned commit SHA is recorded as run.AppliedCommit.
//     A non-fast-forward / drift situation returns an error wrapping
//     ErrSIMergeConflict (→ 转人工, design D7).
//   - Rollback — reverts whatever the matching Apply* call did (git revert
//     for code, snapshot restore for hot-reload).
//
// Stability:evolving
type SIApplier interface {
	ApplyHotReload(ctx context.Context, run *SelfImprovementRun) (rollbackRef string, err error)
	ApplyCodeMerge(ctx context.Context, run *SelfImprovementRun) (commitSHA string, err error)
	Rollback(ctx context.Context, run *SelfImprovementRun, reason string) error
}

// SIMetricsReader captures a sliding-window platform metrics snapshot for the
// observing window (Watchdog before/after comparison, D7).
// Stability:evolving
type SIMetricsReader interface {
	// Snapshot aggregates platform metrics over [now-window, now).
	Snapshot(ctx context.Context, window time.Duration) (*MetricsSnapshot, error)
}
