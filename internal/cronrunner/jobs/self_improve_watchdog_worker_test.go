package jobs

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestNewSelfImproveWatchdogWorker_Defaults(t *testing.T) {
	w := NewSelfImproveWatchdogWorker(0, nil, loggateway.NewNoop())
	if w.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m（design §5）", w.interval)
	}
	w2 := NewSelfImproveWatchdogWorker(2*time.Minute, nil, loggateway.NewNoop())
	if w2.interval != 2*time.Minute {
		t.Errorf("interval = %v, want 2m", w2.interval)
	}
}

func TestSelfImproveWatchdogWorker_StartNilUsecaseNoop(t *testing.T) {
	w := NewSelfImproveWatchdogWorker(time.Minute, nil, loggateway.NewNoop())
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

// Start 首 tick 即执行一次扫描（与 SelfImproveObserveWorker 一致）。
// 扫描在 async safego goroutine 中执行：测试轮询等待首次扫描完成后取消。
func TestSelfImproveWatchdogWorker_RunsOnceImmediately(t *testing.T) {
	store := &siDriveRunStoreStub{}
	uc, err := biz.NewSelfImprovementWatchdogUsecase(biz.SelfImprovementWatchdogDeps{
		RunReader: store, RunWriter: store,
		Metrics: &siWatchdogMetricsStub{}, Applier: &siWatchdogApplierStub{},
		Lg: loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewSelfImprovementWatchdogUsecase: %v", err)
	}
	w := NewSelfImproveWatchdogWorker(10*time.Millisecond, uc, loggateway.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for store.getListCalls() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("worker 至少执行一次 ScanOnce（List）")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
}

type siWatchdogMetricsStub struct{}

func (siWatchdogMetricsStub) Snapshot(context.Context, time.Duration) (*biz.MetricsSnapshot, error) {
	return &biz.MetricsSnapshot{}, nil
}

type siWatchdogApplierStub struct{}

func (siWatchdogApplierStub) ApplyHotReload(context.Context, *biz.SelfImprovementRun) (string, error) {
	return "", nil
}
func (siWatchdogApplierStub) ApplyCodeMerge(context.Context, *biz.SelfImprovementRun) (string, error) {
	return "", nil
}
func (siWatchdogApplierStub) Rollback(context.Context, *biz.SelfImprovementRun, string) error {
	return nil
}
