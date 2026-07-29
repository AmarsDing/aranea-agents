package monitor_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/types"
)

type runnerFlowStubRepo struct {
	rows []monitor.RunnerCompletionRow
	err  error
}

func (s *runnerFlowStubRepo) ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error) {
	return false, nil
}
func (s *runnerFlowStubRepo) PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error) {
	return false, nil
}
func (s *runnerFlowStubRepo) AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error) {
	return 0, nil
}
func (s *runnerFlowStubRepo) LatencyPercentilesSince(ctx context.Context, sinceRFC3339 string) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}
func (s *runnerFlowStubRepo) ListRecentRunnerCompletions(ctx context.Context, since time.Duration, limit int) ([]monitor.RunnerCompletionRow, error) {
	return s.rows, s.err
}

type runnerFlowStubProjector struct {
	started bool
	lastAt  time.Time
	hasEver bool
}

func (s *runnerFlowStubProjector) TraceCount() int        { return 0 }
func (s *runnerFlowStubProjector) Started() bool          { return s.started }
func (s *runnerFlowStubProjector) LastEventAt() time.Time { return s.lastAt }
func (s *runnerFlowStubProjector) HasEverProcessed() bool { return s.hasEver }

func TestRunnerCompletionFlowChecker_NilDeps(t *testing.T) {
	c := monitor.NewRunnerCompletionFlowChecker(nil, nil)
	got := c.Check(context.Background())
	if got.Status != types.SelfCheckStatusWarning {
		t.Fatalf("status=%v want warning", got.Status)
	}
}

func TestRunnerCompletionFlowChecker_Healthy(t *testing.T) {
	repo := &runnerFlowStubRepo{rows: []monitor.RunnerCompletionRow{{CreatedAt: time.Now().UTC().Format(time.RFC3339)}}}
	proj := &runnerFlowStubProjector{started: true, hasEver: true, lastAt: time.Now()}
	c := monitor.NewRunnerCompletionFlowChecker(repo, proj)
	got := c.Check(context.Background())
	if got.Status != types.SelfCheckStatusPassed {
		t.Fatalf("status=%v want passed, msg=%q", got.Status, got.Message)
	}
}

func TestRunnerCompletionFlowChecker_Stalled(t *testing.T) {
	// Recent flow activity but zero runner.completion records in window → failed.
	repo := &runnerFlowStubRepo{}
	proj := &runnerFlowStubProjector{started: true, hasEver: true, lastAt: time.Now()}
	c := monitor.NewRunnerCompletionFlowChecker(repo, proj)
	got := c.Check(context.Background())
	if got.Status != types.SelfCheckStatusFailed {
		t.Fatalf("status=%v want failed, msg=%q", got.Status, got.Message)
	}
}

func TestRunnerCompletionFlowChecker_Idle(t *testing.T) {
	// No recent flow activity and no completions → idle is healthy.
	repo := &runnerFlowStubRepo{}
	proj := &runnerFlowStubProjector{started: true, hasEver: true, lastAt: time.Now().Add(-2 * time.Hour)}
	c := monitor.NewRunnerCompletionFlowChecker(repo, proj)
	got := c.Check(context.Background())
	if got.Status != types.SelfCheckStatusPassed {
		t.Fatalf("status=%v want passed (idle), msg=%q", got.Status, got.Message)
	}
}

func TestRunnerCompletionFlowChecker_NeverProcessed(t *testing.T) {
	// Projector never received any envelope → no way to distinguish idle; pass.
	repo := &runnerFlowStubRepo{}
	proj := &runnerFlowStubProjector{started: true, hasEver: false}
	c := monitor.NewRunnerCompletionFlowChecker(repo, proj)
	got := c.Check(context.Background())
	if got.Status != types.SelfCheckStatusPassed {
		t.Fatalf("status=%v want passed, msg=%q", got.Status, got.Message)
	}
}

func TestRunnerCompletionFlowChecker_QueryError(t *testing.T) {
	repo := &runnerFlowStubRepo{err: errors.New("db down")}
	proj := &runnerFlowStubProjector{started: true, hasEver: true, lastAt: time.Now()}
	c := monitor.NewRunnerCompletionFlowChecker(repo, proj)
	got := c.Check(context.Background())
	if got.Status != types.SelfCheckStatusWarning {
		t.Fatalf("status=%v want warning, msg=%q", got.Status, got.Message)
	}
}
