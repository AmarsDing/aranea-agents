package session

import (
	"context"
	"errors"
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
		sessionReader:      repo,
		sessionWriter:      repo,
		metricsUsecase:     mu,
		compressionUsecase: NewSessionCompressionUsecase(repo, repo, repo, repo),
		timelineUsecase:    NewSessionTimelineUsecase(repo, repo, repo, repo),
		messageUsecase:     NewSessionMessageUsecase(repo, repo, repo, repo, nil, repo, repo, nil, mu, repo, repo, nil, nil),
	}
	return uc, repo
}

func TestAccumulateMetricsDelta_BasicAccumulation(t *testing.T) {
	uc, _ := newTestUsecaseWithMock()

	uc.AccumulateMetricsDelta(SessionMetricsDelta{
		SessionID:    "s1",
		MessageCount: 1,
		RunCount:     1,
	})
	uc.AccumulateMetricsDelta(SessionMetricsDelta{
		SessionID:    "s1",
		MessageCount: 2,
		RunCount:     1,
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
	if d.RunCount != 2 {
		t.Errorf("expected RunCount=2, got %d", d.RunCount)
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

func TestNilMetricsUsecase_GracefulDegradation(t *testing.T) {
	uc := &SessionUsecase{} // metricsUsecase is nil

	// None of these should panic — graceful degradation.
	uc.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: "s1", MessageCount: 1})
	uc.StartMetricsFlusher(context.Background())
	uc.flushAllMetrics(context.Background())
	uc.forceFlushSingle("s1")
}

// mockFailMetricsRepo always fails ApplyMetricsDelta (simulates DB outage).
type mockFailMetricsRepo struct {
	mockSessionRepo
	failCount atomic.Int32
}

func (m *mockFailMetricsRepo) ApplyMetricsDelta(_ context.Context, _ *SessionMetricsDelta) error {
	m.failCount.Add(1)
	return errors.New("db down")
}

// SP-1c 回归：flush 失败重试有上限，超限后 delta 丢弃（不再无限回炉）。
func TestFlushAllMetrics_RetryLimitDropsDelta(t *testing.T) {
	repo := &mockFailMetricsRepo{}
	mu := NewSessionMetricsUsecase(repo, nil, nil)

	mu.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: "s1", MessageCount: 3})

	// First failure re-accumulates with incremented fail count.
	mu.flushAllMetrics(context.Background())
	mu.metricsDeltaMu.Lock()
	d, ok := mu.metricsDeltas["s1"]
	mu.metricsDeltaMu.Unlock()
	if !ok {
		t.Fatal("expected delta re-accumulated after first failure")
	}
	if d.FlushFailCount != 1 {
		t.Errorf("expected FlushFailCount=1, got %d", d.FlushFailCount)
	}

	// Keep flushing well past the limit: delta must be dropped at the cap,
	// with no further ApplyMetricsDelta attempts (no infinite retry).
	for i := 0; i < MaxFlushFailCount+3; i++ {
		mu.flushAllMetrics(context.Background())
	}
	mu.metricsDeltaMu.Lock()
	_, ok = mu.metricsDeltas["s1"]
	mu.metricsDeltaMu.Unlock()
	if ok {
		t.Error("expected delta dropped after reaching MaxFlushFailCount")
	}
	if got := repo.failCount.Load(); got != int32(MaxFlushFailCount) {
		t.Errorf("expected exactly %d flush attempts, got %d", MaxFlushFailCount, got)
	}
}

// SP-1c 回归：失败回炉的 delta 与窗口内新 delta 合并时，失败计数取 max。
func TestAccumulateMetricsDelta_MergeKeepsMaxFlushFailCount(t *testing.T) {
	mu := NewSessionMetricsUsecase(&mockMetricsRepo{}, nil, nil)

	mu.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: "s1", MessageCount: 1, FlushFailCount: 3})
	mu.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: "s1", MessageCount: 1, FlushFailCount: 1})

	mu.metricsDeltaMu.Lock()
	d := mu.metricsDeltas["s1"]
	mu.metricsDeltaMu.Unlock()
	if d.FlushFailCount != 3 {
		t.Errorf("expected merged FlushFailCount=3, got %d", d.FlushFailCount)
	}
	if d.MessageCount != 2 {
		t.Errorf("expected merged MessageCount=2, got %d", d.MessageCount)
	}
}
