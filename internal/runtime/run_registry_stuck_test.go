package runtime

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// stuckSweepRunner reports a controllable framework run status (heartbeat).
type stuckSweepRunner struct {
	registryRunner
	status   trpcrunner.RunStatus
	statusOK bool
}

func (r *stuckSweepRunner) RunStatus(string) (trpcrunner.RunStatus, bool) {
	return r.status, r.statusOK
}

func TestSweepStuckRuns_CancelsSilentRunner(t *testing.T) {
	reg := NewRunRegistry()
	now := time.Now()
	runner := &stuckSweepRunner{
		registryRunner: registryRunner{cancelOK: true},
		status:         trpcrunner.RunStatus{LastEventAt: now.Add(-time.Hour), EventCount: 3},
		statusOK:       true,
	}
	reg.StoreRunner("s1", "r1", runner)
	reg.SetStatus("s1", "r1", string(biz.RunStateRunning), "")

	stuck := reg.SweepStuckRuns(now, DefaultStuckRunTimeout)
	if len(stuck) != 1 {
		t.Fatalf("expected 1 stuck run, got %d", len(stuck))
	}
	if stuck[0].SessionID != "s1" || stuck[0].RunID != "r1" {
		t.Errorf("stuck entry = %+v", stuck[0])
	}
	if runner.cancelCalls != 1 {
		t.Errorf("runner Cancel calls = %d, want 1", runner.cancelCalls)
	}
	st, ok := reg.GetStatus("s1")
	if !ok || st.Status != string(biz.SessionRunPhaseCancelled) {
		t.Errorf("status = %+v ok=%v, want cancelled", st, ok)
	}

	// A second sweep must not re-cancel the already-cancelling entry.
	if again := reg.SweepStuckRuns(now, DefaultStuckRunTimeout); len(again) != 0 {
		t.Errorf("second sweep returned %d entries, want 0 (cancelling guard)", len(again))
	}
	if runner.cancelCalls != 1 {
		t.Errorf("runner Cancel calls after second sweep = %d, want 1", runner.cancelCalls)
	}
}

func TestSweepStuckRuns_SparesFreshRunner(t *testing.T) {
	reg := NewRunRegistry()
	now := time.Now()
	runner := &stuckSweepRunner{
		registryRunner: registryRunner{cancelOK: true},
		status:         trpcrunner.RunStatus{LastEventAt: now.Add(-5 * time.Minute), EventCount: 42},
		statusOK:       true,
	}
	reg.StoreRunner("s1", "r1", runner)
	reg.SetStatus("s1", "r1", string(biz.RunStateRunning), "")

	if stuck := reg.SweepStuckRuns(now, DefaultStuckRunTimeout); len(stuck) != 0 {
		t.Fatalf("fresh runner must be spared, got %+v", stuck)
	}
	if runner.cancelCalls != 0 {
		t.Errorf("Cancel calls = %d, want 0", runner.cancelCalls)
	}
}

func TestSweepStuckRuns_NeverTouchesAwaitingUser(t *testing.T) {
	reg := NewRunRegistry()
	now := time.Now()
	runner := &stuckSweepRunner{
		registryRunner: registryRunner{cancelOK: true},
		// Silent for a full day — a clarify card may legitimately wait this long.
		status:   trpcrunner.RunStatus{LastEventAt: now.Add(-24 * time.Hour), EventCount: 7},
		statusOK: true,
	}
	reg.StoreRunner("s1", "r1", runner)
	reg.SetStatus("s1", "r1", string(biz.RunStateAwaitingUser), "")

	if stuck := reg.SweepStuckRuns(now, DefaultStuckRunTimeout); len(stuck) != 0 {
		t.Fatalf("awaiting_user run must never be swept, got %+v", stuck)
	}
}

func TestSweepStuckRuns_SkipsPlaceholderAndRunnerless(t *testing.T) {
	reg := NewRunRegistry()
	now := time.Now()

	reg.StorePlaceholder("s-placeholder")
	reg.SetStatus("s-placeholder", "", string(biz.RunStateRunning), "")
	reg.StoreCancelable("s-runnerless", "r1", func() {})

	if stuck := reg.SweepStuckRuns(now, DefaultStuckRunTimeout); len(stuck) != 0 {
		t.Fatalf("placeholder/runner-less entries must be skipped, got %+v", stuck)
	}
	// Placeholder must NOT carry a cancel intent poisoned into the next runner.
	runner := &stuckSweepRunner{registryRunner: registryRunner{cancelOK: true}}
	reg.StoreRunner("s-placeholder", "r2", runner)
	if runner.cancelCalls != 0 {
		t.Errorf("next runner after placeholder sweep was cancelled — cancel intent leaked")
	}
}

func TestSweepStuckRuns_NonManagedRunnerFallsBackToStatusTimestamp(t *testing.T) {
	reg := NewRunRegistry()
	now := time.Now()
	// Default registryRunner declines RunStatus (statusOK=false path).
	runner := &registryRunner{cancelOK: true}
	reg.StoreRunner("s1", "r1", runner)
	// Backdate the registry status entry directly (same-package access).
	reg.runStatuses.m.Store("s1", &RunStatusEntry{
		RunID: "r1", Status: string(biz.RunStateRunning), UpdatedAt: now.Add(-time.Hour),
	})

	stuck := reg.SweepStuckRuns(now, DefaultStuckRunTimeout)
	if len(stuck) != 1 {
		t.Fatalf("status-timestamp fallback should catch the stale run, got %d entries", len(stuck))
	}
	if runner.cancelCalls != 1 {
		t.Errorf("Cancel calls = %d, want 1", runner.cancelCalls)
	}
}

func TestSweepStuckRuns_NeverKillsBlind(t *testing.T) {
	reg := NewRunRegistry()
	now := time.Now()
	// Runner without run-status reporting AND no registry status entry:
	// zero heartbeat signal — must not be touched.
	runner := &registryRunner{cancelOK: true}
	reg.StoreRunner("s1", "r1", runner)

	if stuck := reg.SweepStuckRuns(now, time.Hour); len(stuck) != 0 {
		t.Fatalf("no-signal run must never be killed blind, got %+v", stuck)
	}
	if runner.cancelCalls != 0 {
		t.Errorf("Cancel calls = %d, want 0", runner.cancelCalls)
	}
}

func TestSweepStuckRuns_ThresholdZeroDisabled(t *testing.T) {
	reg := NewRunRegistry()
	now := time.Now()
	runner := &stuckSweepRunner{
		registryRunner: registryRunner{cancelOK: true},
		status:         trpcrunner.RunStatus{LastEventAt: now.Add(-72 * time.Hour)},
		statusOK:       true,
	}
	reg.StoreRunner("s1", "r1", runner)
	reg.SetStatus("s1", "r1", string(biz.RunStateRunning), "")

	if stuck := reg.SweepStuckRuns(now, 0); len(stuck) != 0 {
		t.Fatalf("threshold<=0 must disable the sweep, got %+v", stuck)
	}
	if stuck := reg.SweepStuckRuns(now, -time.Minute); stuck != nil {
		t.Fatalf("negative threshold must return nil, got %+v", stuck)
	}
}
