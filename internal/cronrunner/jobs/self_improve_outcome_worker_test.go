package jobs

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestNewSelfImproveOutcomeWorker_Defaults(t *testing.T) {
	w := NewSelfImproveOutcomeWorker(0, nil, loggateway.NewNoop())
	if w.interval != time.Hour {
		t.Errorf("interval = %v, want 1h（design §5）", w.interval)
	}
	w2 := NewSelfImproveOutcomeWorker(30*time.Minute, nil, loggateway.NewNoop())
	if w2.interval != 30*time.Minute {
		t.Errorf("interval = %v, want 30m", w2.interval)
	}
}

func TestSelfImproveOutcomeWorker_StartNilUsecaseNoop(t *testing.T) {
	w := NewSelfImproveOutcomeWorker(time.Minute, nil, loggateway.NewNoop())
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

// Start 首 tick 即执行一次归因扫描（与 SelfImproveObserveWorker 一致）。
func TestSelfImproveOutcomeWorker_RunsOnceImmediately(t *testing.T) {
	store := &siDriveRunStoreStub{}
	uc, err := biz.NewSelfImprovementOutcomeUsecase(biz.SelfImprovementOutcomeDeps{
		RunReader: store, Outcomes: &siOutcomeWriterStub{},
		Lg: loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewSelfImprovementOutcomeUsecase: %v", err)
	}
	w := NewSelfImproveOutcomeWorker(10*time.Millisecond, uc, loggateway.NewNoop())
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Start(ctx) // 阻塞至 ctx 超时；不 panic 即通过
	if store.getListCalls() != 0 {
		t.Fatal("空待归因集不应触发 List（RunFilter 路径）")
	}
}

type siOutcomeWriterStub struct{}

func (siOutcomeWriterStub) CreateOutcome(context.Context, *biz.PatchOutcome) error { return nil }
func (siOutcomeWriterStub) ListOutcomesByRun(context.Context, string) ([]biz.PatchOutcome, error) {
	return nil, nil
}
func (siOutcomeWriterStub) ListRecentOutcomesByTrigger(context.Context, string, int) ([]biz.PatchOutcome, error) {
	return nil, nil
}
