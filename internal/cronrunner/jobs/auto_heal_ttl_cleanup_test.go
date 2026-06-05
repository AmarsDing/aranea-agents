package jobs_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/log"
)

// ---------------------------------------------------------------------------
// mock: monitor.HealRecordRepo
// ---------------------------------------------------------------------------

type mockHealRecordRepo struct {
	insertFn func(ctx context.Context, record monitor.HealRecord) error
	listFn   func(ctx context.Context, query monitor.HealRecordQuery) (monitor.HealRecordListResult, error)
	deleteFn func(ctx context.Context, olderThan time.Time) (int, error)
}

func (m *mockHealRecordRepo) InsertHealRecord(ctx context.Context, record monitor.HealRecord) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, record)
	}
	return nil
}

func (m *mockHealRecordRepo) ListHealRecords(ctx context.Context, query monitor.HealRecordQuery) (monitor.HealRecordListResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, query)
	}
	return monitor.HealRecordListResult{}, nil
}

func (m *mockHealRecordRepo) DeleteHealRecordsOlderThan(ctx context.Context, olderThan time.Time) (int, error) {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, olderThan)
	}
	return 0, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestCleanup(t *testing.T, interval, maxAge time.Duration, repo monitor.HealRecordRepo) *jobs.AutoHealTTLCleanup {
	t.Helper()
	lg := loggateway.NewNoop()
	return jobs.NewAutoHealTTLCleanup(interval, maxAge, repo, lg, log.DefaultLogger)
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestNewAutoHealTTLCleanup_Defaults(t *testing.T) {
	t.Setenv("AUTO_HEAL_TTL_INTERVAL", "")
	t.Setenv("AUTO_HEAL_TTL_MAX_AGE", "")

	c := newTestCleanup(t, 0, 0, &mockHealRecordRepo{})
	if c == nil {
		t.Fatal("expected non-nil cleanup")
	}
}

func TestNewAutoHealTTLCleanup_CustomValues(t *testing.T) {
	interval := 30 * time.Minute
	maxAge := 168 * time.Hour

	c := newTestCleanup(t, interval, maxAge, &mockHealRecordRepo{})
	if c == nil {
		t.Fatal("expected non-nil cleanup")
	}
}

func TestAutoHealTTLCleanup_RunOnce_DeletesOldRecords(t *testing.T) {
	maxAge := 72 * time.Hour
	var capturedCutoff time.Time

	repo := &mockHealRecordRepo{
		deleteFn: func(_ context.Context, olderThan time.Time) (int, error) {
			capturedCutoff = olderThan
			return 5, nil
		},
	}

	c := newTestCleanup(t, 0, maxAge, repo)
	c.RunOnceExposed(context.Background())

	expectedCutoff := time.Now().UTC().Add(-maxAge)
	diff := capturedCutoff.Sub(expectedCutoff)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("cutoff mismatch: got %v, want approximately %v (diff=%v)", capturedCutoff, expectedCutoff, diff)
	}
}

func TestAutoHealTTLCleanup_RunOnce_DeleteError(t *testing.T) {
	repo := &mockHealRecordRepo{
		deleteFn: func(_ context.Context, _ time.Time) (int, error) {
			return 0, context.DeadlineExceeded
		},
	}

	c := newTestCleanup(t, 0, 72*time.Hour, repo)

	// Should not panic on error.
	c.RunOnceExposed(context.Background())
}

func TestAutoHealTTLCleanup_RunOnce_NoRecordsDeleted(t *testing.T) {
	repo := &mockHealRecordRepo{
		deleteFn: func(_ context.Context, _ time.Time) (int, error) {
			return 0, nil
		},
	}

	c := newTestCleanup(t, 0, 72*time.Hour, repo)

	// Should not panic when no records are deleted.
	c.RunOnceExposed(context.Background())
}

func TestAutoHealTTLCleanup_Start_ContextCancel(t *testing.T) {
	repo := &mockHealRecordRepo{
		deleteFn: func(_ context.Context, _ time.Time) (int, error) {
			return 0, nil
		},
	}

	// Use a very short interval so the ticker fires quickly.
	c := newTestCleanup(t, 10*time.Millisecond, 72*time.Hour, repo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)

	// Let the goroutine start and potentially tick once.
	time.Sleep(50 * time.Millisecond)

	// Cancel the context — the goroutine should exit.
	cancel()

	// Give the goroutine time to observe cancellation.
	time.Sleep(50 * time.Millisecond)

	// If we reach here without hanging, the test passes.
}
