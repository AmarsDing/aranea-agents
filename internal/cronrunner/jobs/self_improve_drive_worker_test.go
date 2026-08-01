package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// siDriveRunStoreStub is a minimal empty-run store for worker smoke tests.
type siDriveRunStoreStub struct {
	mu        sync.Mutex
	listCalls int
}

// getListCalls returns the List call count under the store mutex. Worker
// scans run in async safego goroutines, so tests must not read listCalls
// directly.
func (s *siDriveRunStoreStub) getListCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listCalls
}

func (s *siDriveRunStoreStub) GetByID(context.Context, string) (*biz.SelfImprovementRun, error) {
	return nil, nil
}
func (s *siDriveRunStoreStub) GetBySuggestionID(context.Context, string) (*biz.SelfImprovementRun, error) {
	return nil, nil
}
func (s *siDriveRunStoreStub) List(context.Context, biz.RunFilter) ([]biz.SelfImprovementRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	return nil, nil
}
func (s *siDriveRunStoreStub) Count(context.Context, biz.RunFilter) (int, error) {
	return 0, nil
}
func (s *siDriveRunStoreStub) ListObservingDue(context.Context, time.Time) ([]biz.SelfImprovementRun, error) {
	return nil, nil
}
func (s *siDriveRunStoreStub) ListTerminalPendingOutcome(context.Context, int) ([]biz.SelfImprovementRun, error) {
	return nil, nil
}
func (s *siDriveRunStoreStub) Create(context.Context, *biz.SelfImprovementRun) error { return nil }
func (s *siDriveRunStoreStub) Update(context.Context, *biz.SelfImprovementRun, biz.SelfImprovementRunStatus) error {
	return nil
}
func (s *siDriveRunStoreStub) RecordAttempt(context.Context, string) error { return nil }

type siDriveApplyStub struct{}

func (siDriveApplyStub) Apply(context.Context, string) error   { return nil }
func (siDriveApplyStub) PromoteEligible(context.Context) error { return nil }

func TestNewSelfImproveDriveWorker_Defaults(t *testing.T) {
	w := NewSelfImproveDriveWorker(0, nil, loggateway.NewNoop())
	if w.interval != time.Minute {
		t.Errorf("interval = %v, want 1m（drive 全链驱动默认）", w.interval)
	}
	w2 := NewSelfImproveDriveWorker(3*time.Second, nil, loggateway.NewNoop())
	if w2.interval != 3*time.Second {
		t.Errorf("interval = %v, want 3s", w2.interval)
	}
}

func TestSelfImproveDriveWorker_StartNilUsecaseNoop(t *testing.T) {
	w := NewSelfImproveDriveWorker(time.Minute, nil, loggateway.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start 应在 ctx 取消后返回")
	}
}

// Start 首 tick 即执行一次驱动（与 SelfImproveObserveWorker 一致）。
// 扫描在 async safego goroutine 中执行：测试轮询等待首次扫描完成后取消。
func TestSelfImproveDriveWorker_RunsOnceImmediately(t *testing.T) {
	store := &siDriveRunStoreStub{}
	uc, err := biz.NewSelfImprovementDriveUsecase(biz.SelfImprovementDriveDeps{
		RunReader: store, RunWriter: store, Applier: &siDriveApplyStub{},
		Lg: loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewSelfImprovementDriveUsecase: %v", err)
	}
	w := NewSelfImproveDriveWorker(10*time.Millisecond, uc, loggateway.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for store.getListCalls() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("worker 至少执行一次 DriveOnce（List）")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
}
