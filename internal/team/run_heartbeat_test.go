// run_heartbeat_test.go — P2-1 持久化心跳节流写入器测试。
package team

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// fakeHeartbeatWriter 记录 TouchTeamRunHeartbeat 调用，可注入错误。
type fakeHeartbeatWriter struct {
	mu    sync.Mutex
	calls []time.Time
	err   error
}

func (f *fakeHeartbeatWriter) TouchTeamRunHeartbeat(_ context.Context, _ string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err == nil {
		f.calls = append(f.calls, at)
	}
	return f.err
}

func (f *fakeHeartbeatWriter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestTeamRunHeartbeatPinger_Throttle 首 ping 立即写，间隔内节流，超间隔再写。
func TestTeamRunHeartbeatPinger_Throttle(t *testing.T) {
	t.Parallel()
	w := &fakeHeartbeatWriter{}
	p := newTeamRunHeartbeatPinger(w, "run-1", loggateway.NewNoop())
	ctx := context.Background()

	p.Ping(ctx)
	if got := w.count(); got != 1 {
		t.Fatalf("首次 Ping 后写入次数 = %d, want 1", got)
	}

	// 间隔内连续 Ping 全部节流。
	for i := 0; i < 5; i++ {
		p.Ping(ctx)
	}
	if got := w.count(); got != 1 {
		t.Fatalf("间隔内 Ping 后写入次数 = %d, want 1（应被节流）", got)
	}

	// 白盒拨快 last，模拟超过节流间隔。
	p.mu.Lock()
	p.last = time.Now().Add(-teamRunHeartbeatInterval - time.Second)
	p.mu.Unlock()
	p.Ping(ctx)
	if got := w.count(); got != 2 {
		t.Fatalf("超间隔 Ping 后写入次数 = %d, want 2", got)
	}
}

// TestTeamRunHeartbeatPinger_CanceledCtx ctx 取消后静默跳过（run 收尾期不
// 再写库）。
func TestTeamRunHeartbeatPinger_CanceledCtx(t *testing.T) {
	t.Parallel()
	w := &fakeHeartbeatWriter{}
	p := newTeamRunHeartbeatPinger(w, "run-1", loggateway.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Ping(ctx)
	if got := w.count(); got != 0 {
		t.Fatalf("ctx 取消后写入次数 = %d, want 0", got)
	}
}

// TestTeamRunHeartbeatPinger_NilSafe nil pinger / nil writer / 空 runID
// 全部安全无操作。
func TestTeamRunHeartbeatPinger_NilSafe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var nilP *teamRunHeartbeatPinger
	nilP.Ping(ctx) // 不 panic

	w := &fakeHeartbeatWriter{}
	newTeamRunHeartbeatPinger(nil, "run-1", loggateway.NewNoop()).Ping(ctx)
	newTeamRunHeartbeatPinger(w, "", loggateway.NewNoop()).Ping(ctx)
	if got := w.count(); got != 0 {
		t.Fatalf("空 runID 写入次数 = %d, want 0", got)
	}
}

// TestTeamRunHeartbeatPinger_WriteErrorSwallowed 写失败仅记日志，不 panic、
// 不影响流式消费；且失败也刷新节流窗口（避免失败风暴刷日志）。
func TestTeamRunHeartbeatPinger_WriteErrorSwallowed(t *testing.T) {
	t.Parallel()
	w := &fakeHeartbeatWriter{err: errors.New("db down")}
	p := newTeamRunHeartbeatPinger(w, "run-1", loggateway.NewNoop())
	p.Ping(context.Background()) // 不 panic
	p.Ping(context.Background()) // 间隔内：第二次不再调写库
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.calls) != 0 {
		t.Fatalf("写失败时不应记录成功调用, got %d", len(w.calls))
	}
}

// TestTeamRunHeartbeatPinger_Concurrent 多成员流并发 Ping：线程安全且全局
// 节流为 1 次写入。
func TestTeamRunHeartbeatPinger_Concurrent(t *testing.T) {
	t.Parallel()
	w := &fakeHeartbeatWriter{}
	p := newTeamRunHeartbeatPinger(w, "run-1", loggateway.NewNoop())
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Ping(ctx)
		}()
	}
	wg.Wait()
	if got := w.count(); got != 1 {
		t.Fatalf("并发 Ping 写入次数 = %d, want 1（全局节流）", got)
	}
}
