package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// mockMetricsRepo tracks ApplyMetricsDelta calls for testing.
type mockMetricsRepo struct {
	mockSessionRepo
	flushCount atomic.Int32
	lastDelta  atomic.Pointer[SessionMetricsDelta]
}

func (m *mockMetricsRepo) ApplyMetricsDelta(_ context.Context, d *SessionMetricsDelta) error {
	m.flushCount.Add(1)
	m.lastDelta.Store(d)
	return nil
}

func newTestUsecaseWithMock() (*SessionUsecase, *mockMetricsRepo) {
	repo := &mockMetricsRepo{}
	mu := NewSessionMetricsUsecase(repo, nil, nil)
	mu.flushInterval = 1 * time.Hour // disable periodic flush for test
	uc := &SessionUsecase{
		sessionReader:  repo,
		sessionWriter:  repo,
		contextUpdater: repo,
		metricsUsecase: mu,
	}
	return uc, repo
}

func TestAccumulateMetricsDelta_BasicAccumulation(t *testing.T) {
	uc, _ := newTestUsecaseWithMock()

	uc.AccumulateMetricsDelta(SessionMetricsDelta{
		SessionID:    "s1",
		MessageCount: 1,
	})
	uc.AccumulateMetricsDelta(SessionMetricsDelta{
		SessionID:    "s1",
		MessageCount: 2,
	})

	uc.metricsUsecase.metricsDeltaMu.Lock()
	d := uc.metricsUsecase.metricsDeltas["s1"]
	uc.metricsUsecase.metricsDeltaMu.Unlock()

	if d == nil {
		t.Fatal("expected delta to exist")
	}
	if d.MessageCount != 3 {
		t.Errorf("expected MessageCount=3, got %d", d.MessageCount)
	}
	if d.AccumulatedCount != 2 {
		t.Errorf("expected AccumulatedCount=2, got %d", d.AccumulatedCount)
	}
}

func TestAccumulateMetricsDelta_ForceFlushOnCountOverflow(t *testing.T) {
	uc, repo := newTestUsecaseWithMock()

	// Accumulate MaxDeltaCount - 1 times (should not flush)
	for i := 0; i < MaxDeltaCount-1; i++ {
		uc.AccumulateMetricsDelta(SessionMetricsDelta{
			SessionID:    "s1",
			MessageCount: 1,
		})
	}
	// Give async flush time
	time.Sleep(50 * time.Millisecond)
	if repo.flushCount.Load() > 0 {
		t.Errorf("expected no flush before MaxDeltaCount, got %d", repo.flushCount.Load())
	}

	// One more accumulation triggers force flush
	uc.AccumulateMetricsDelta(SessionMetricsDelta{
		SessionID:    "s1",
		MessageCount: 1,
	})

	// Wait for async force flush
	time.Sleep(100 * time.Millisecond)
	if repo.flushCount.Load() == 0 {
		t.Error("expected force flush on MaxDeltaCount overflow")
	}
}

func TestAccumulateMetricsDelta_ForceFlushOnAgeOverflow(t *testing.T) {
	uc, repo := newTestUsecaseWithMock()

	uc.AccumulateMetricsDelta(SessionMetricsDelta{
		SessionID:    "s1",
		MessageCount: 1,
	})

	// Manually age the delta to exceed MaxDeltaAge
	uc.metricsUsecase.metricsDeltaMu.Lock()
	d := uc.metricsUsecase.metricsDeltas["s1"]
	d.FirstAccumulatedAt = time.Now().Add(-MaxDeltaAge - time.Second)
	uc.metricsUsecase.metricsDeltaMu.Unlock()

	// Next accumulation should trigger force flush
	uc.AccumulateMetricsDelta(SessionMetricsDelta{
		SessionID:    "s1",
		MessageCount: 1,
	})

	// Wait for async force flush
	time.Sleep(100 * time.Millisecond)
	if repo.flushCount.Load() == 0 {
		t.Error("expected force flush on MaxDeltaAge overflow")
	}
}

func TestFlushAllMetrics(t *testing.T) {
	uc, repo := newTestUsecaseWithMock()

	uc.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: "s1", MessageCount: 1})
	uc.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: "s2", MessageCount: 2})

	uc.flushAllMetrics(context.Background())

	if repo.flushCount.Load() != 2 {
		t.Errorf("expected 2 flushes, got %d", repo.flushCount.Load())
	}
}

func TestForceFlushSingle(t *testing.T) {
	uc, repo := newTestUsecaseWithMock()

	uc.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: "s1", MessageCount: 5})

	uc.forceFlushSingle("s1")

	if repo.flushCount.Load() != 1 {
		t.Errorf("expected 1 flush, got %d", repo.flushCount.Load())
	}
	last := repo.lastDelta.Load()
	if last == nil || last.MessageCount != 5 {
		t.Errorf("expected MessageCount=5 in flushed delta, got %+v", last)
	}
}
