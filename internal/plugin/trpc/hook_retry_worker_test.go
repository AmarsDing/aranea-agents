package plugintrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"
)

// blockingStaleRepo 让 retryStale 卡在 ListStalePending 内，模拟 N-B3 的
// 「Stop 时 worker 正在长查询」窗口。
type blockingStaleRepo struct {
	enteredOnce sync.Once
	entered     chan struct{}
	release     chan struct{}
}

func newBlockingStaleRepo() *blockingStaleRepo {
	return &blockingStaleRepo{entered: make(chan struct{}), release: make(chan struct{})}
}

func (r *blockingStaleRepo) Insert(context.Context, biz.HookDelivery) error { return nil }
func (r *blockingStaleRepo) UpdateResult(context.Context, string, biz.HookDeliveryStatus, int, string) error {
	return nil
}
func (r *blockingStaleRepo) List(context.Context, biz.HookDeliveryQuery) (biz.HookDeliveryListResult, error) {
	return biz.HookDeliveryListResult{}, nil
}
func (r *blockingStaleRepo) ListStalePending(ctx context.Context, _ time.Time, _ int) ([]biz.HookDelivery, error) {
	r.enteredOnce.Do(func() { close(r.entered) })
	select {
	case <-r.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (r *blockingStaleRepo) TryClaimForRetry(context.Context, string, int) (bool, error) {
	return false, nil
}

func fastHookRuntimeConf() *conf.Runtime {
	return &conf.Runtime{Hook: &conf.Runtime_Hook{
		RetryPollIntervalMs: 10,
		RetryQueryTimeoutMs: 5000,
		RetryStaleAfterMs:   0,
		RetryBatchSize:      5,
	}}
}

// N-B3 回归：Stop 在 retryStale 进行中被调用时信号不得丢失，loop 必须退出。
// 旧实现（无缓冲 channel + 非阻塞发送）在此场景必丢信号，本测试会超时失败。
func TestHookDeliveryRetryWorker_StopDuringRetryExits(t *testing.T) {
	repo := newBlockingStaleRepo()
	w := NewHookDeliveryRetryWorker(fastHookRuntimeConf(), repo, nil, loggateway.NewNoop())
	w.Start()
	select {
	case <-repo.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not enter retryStale in time")
	}
	// retryStale 正卡在 ListStalePending —— 旧实现这里 Stop 信号必丢。
	w.Stop()
	close(repo.release)
	if !w.Wait(2 * time.Second) {
		t.Fatal("worker loop did not exit after Stop during retryStale (signal lost)")
	}
	// 幂等：重复 Stop / Stop(nil) 不得 panic 或阻塞。
	w.Stop()
	var nilWorker *HookDeliveryRetryWorker
	nilWorker.Stop()
	if !nilWorker.Wait(time.Second) {
		t.Fatal("nil worker Wait should return true")
	}
}

// Wait 对从未 Start 的 worker 必须立即返回（Runtime.Close 路径）。
func TestHookDeliveryRetryWorker_WaitNeverStarted(t *testing.T) {
	w := NewHookDeliveryRetryWorker(fastHookRuntimeConf(), newBlockingStaleRepo(), nil, loggateway.NewNoop())
	if !w.Wait(50 * time.Millisecond) {
		t.Fatal("Wait on never-started worker must return immediately")
	}
}
