package jobs

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestNewSelfImproveObserveWorker_Defaults(t *testing.T) {
	w := NewSelfImproveObserveWorker(0, nil, loggateway.NewNoop())
	if w.interval != 15*time.Minute {
		t.Errorf("interval = %v, want 15m（design §5）", w.interval)
	}
	w2 := NewSelfImproveObserveWorker(5*time.Minute, nil, loggateway.NewNoop())
	if w2.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m", w2.interval)
	}
}

func TestSelfImproveObserveWorker_StartNilUsecaseNoop(t *testing.T) {
	w := NewSelfImproveObserveWorker(time.Minute, nil, loggateway.NewNoop())
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

// Start 首 tick 即执行一次扫描（与 EvolutionOrchestratorWorker 一致）。
func TestSelfImproveObserveWorker_RunsOnceImmediately(t *testing.T) {
	// nil-dep usecase: ScanOnce 返回 (0,nil)，不 panic。
	uc := biz.NewSelfImprovementObserveUsecase(nil, nil, nil, nil, loggateway.NewNoop())
	w := NewSelfImproveObserveWorker(10*time.Millisecond, uc, loggateway.NewNoop())
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Start(ctx) // 阻塞至 ctx 超时；不 panic 即通过
}
