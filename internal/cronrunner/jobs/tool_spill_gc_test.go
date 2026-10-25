package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/toolspill"
)

// fakeGCStore 是 toolspill.Store 的最小测试实现：只有 GC 行为可控，其余方法
// 不可达（本 worker 只消费 GC）。
type fakeGCStore struct {
	gc func(ctx context.Context, now time.Time) (toolspill.GCStats, error)
}

func (f *fakeGCStore) Put(context.Context, toolspill.Record) (toolspill.Locator, error) {
	return toolspill.Locator{}, errors.New("not implemented")
}
func (f *fakeGCStore) Get(context.Context, string, string) (toolspill.Record, error) {
	return toolspill.Record{}, errors.New("not implemented")
}
func (f *fakeGCStore) Stat(context.Context, string, string) (toolspill.FileInfo, error) {
	return toolspill.FileInfo{}, errors.New("not implemented")
}
func (f *fakeGCStore) GC(ctx context.Context, now time.Time) (toolspill.GCStats, error) {
	return f.gc(ctx, now)
}

// recCollector 收集 EmitGate 落下的决策记录（decision.Collector 单方法接口）。
type recCollector struct {
	mu   sync.Mutex
	recs []decision.Record
}

func (c *recCollector) Emit(_ context.Context, rec decision.Record) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recs = append(c.recs, rec)
}

func (c *recCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.recs)
}

func (c *recCollector) first() decision.Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.recs) == 0 {
		return decision.Record{}
	}
	return c.recs[0]
}

func TestToolSpillGCDisabled(t *testing.T) {
	t.Setenv("TOOLSPILL_GC_DISABLED", "")
	if ToolSpillGCDisabled() {
		t.Fatal("empty env should be enabled")
	}
	t.Setenv("TOOLSPILL_GC_DISABLED", "1")
	if !ToolSpillGCDisabled() {
		t.Fatal("env=1 should disable")
	}
	t.Setenv("TOOLSPILL_GC_DISABLED", "TRUE")
	if !ToolSpillGCDisabled() {
		t.Fatal("env=TRUE should disable")
	}
}

func TestToolSpillGCIntervalResolution(t *testing.T) {
	t.Setenv("TOOLSPILL_GC_INTERVAL", "")
	if got := NewToolSpillGC(nil, "fs", 0, nil, nil); got.interval != time.Hour {
		t.Fatalf("default interval = %v, want 1h", got.interval)
	}
	t.Setenv("TOOLSPILL_GC_INTERVAL", "30m")
	if got := NewToolSpillGC(nil, "fs", 0, nil, nil); got.interval != 30*time.Minute {
		t.Fatalf("env interval = %v, want 30m", got.interval)
	}
	t.Setenv("TOOLSPILL_GC_INTERVAL", "1m")
	if got := NewToolSpillGC(nil, "fs", 0, nil, nil); got.interval != toolSpillGCMinInterval {
		t.Fatalf("env below floor = %v, want clamp to %v", got.interval, toolSpillGCMinInterval)
	}
	if got := NewToolSpillGC(nil, "fs", 42*time.Second, nil, nil); got.interval != 42*time.Second {
		t.Fatalf("explicit interval = %v, want 42s (constructor choice honored)", got.interval)
	}
}

// TestToolSpillGCStartNilSafe 钉 typed-nil 兜底：provider 在 spill OFF/构造失败
// 时返 nil 指针，workers.go != nil 判定失效，Start 必须直接返回零 panic。
func TestToolSpillGCStartNilSafe(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var w *ToolSpillGC
		w.Start(context.Background())
		(&ToolSpillGC{}).Start(context.Background()) // store=nil
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start on nil/storeless worker should return immediately")
	}
}

func TestToolSpillGCRunOnceFailureEmitsOneRecord(t *testing.T) {
	store := &fakeGCStore{gc: func(context.Context, time.Time) (toolspill.GCStats, error) {
		return toolspill.GCStats{Scanned: 3, Removed: 1}, errors.New("disk full")
	}}
	coll := &recCollector{}
	w := NewToolSpillGC(store, "fs", time.Hour, coll, nil)
	w.runOnce(context.Background())

	if coll.count() != 1 {
		t.Fatalf("failure cycle must emit exactly one record, got %d", coll.count())
	}
	rec := coll.first()
	if got := rec.Metadata["trigger_rule"]; got != decision.TriggerSpillGCFailed {
		t.Fatalf("trigger_rule = %v, want %s", got, decision.TriggerSpillGCFailed)
	}
	if rec.Outcome != "tripped" {
		t.Fatalf("outcome = %q, want tripped", rec.Outcome)
	}
	if got := rec.Metadata["store_kind"]; got != "fs" {
		t.Fatalf("store_kind = %v, want fs", got)
	}
}

func TestToolSpillGCRunOnceSuccessZeroEvents(t *testing.T) {
	for _, removed := range []int{0, 2} {
		store := &fakeGCStore{gc: func(context.Context, time.Time) (toolspill.GCStats, error) {
			return toolspill.GCStats{Scanned: 10, Removed: removed, ReclaimedBytes: 64}, nil
		}}
		coll := &recCollector{}
		w := NewToolSpillGC(store, "fs", time.Hour, coll, nil)
		w.runOnce(context.Background())
		if coll.count() != 0 {
			t.Fatalf("removed=%d: success must emit no record, got %d", removed, coll.count())
		}
	}
}

// TestToolSpillGCRunOnceCtxCancelledIsNeutral 钉关停期取消不产生审计事件：
// store 返回 ctx.Err() 时按 cancelled 计，不出现 spill_gc_failed 记录。
func TestToolSpillGCRunOnceCtxCancelledIsNeutral(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &fakeGCStore{gc: func(gctx context.Context, _ time.Time) (toolspill.GCStats, error) {
		return toolspill.GCStats{}, gctx.Err()
	}}
	coll := &recCollector{}
	w := NewToolSpillGC(store, "pg", time.Hour, coll, nil)
	w.runOnce(ctx)
	if coll.count() != 0 {
		t.Fatalf("cancelled cycle must be neutral (no record), got %d", coll.count())
	}
}

func TestToolSpillGCRunOncePanicRecovered(t *testing.T) {
	store := &fakeGCStore{gc: func(context.Context, time.Time) (toolspill.GCStats, error) {
		panic("boom")
	}}
	coll := &recCollector{}
	w := NewToolSpillGC(store, "fs", time.Hour, coll, nil)
	// D2-f-1：panic 周期必须落 runs_total{panic} 平面（recover 吞 metrics 三平面丢信缺陷钉）
	before := testutil.ToFloat64(metrics.ToolSpillGCRunsTotal.WithLabelValues("fs", "panic"))
	w.runOnce(context.Background()) // must not propagate
	after := testutil.ToFloat64(metrics.ToolSpillGCRunsTotal.WithLabelValues("fs", "panic"))
	if after != before+1 {
		t.Fatalf("runs_total{status=panic} delta = %v, want 1", after-before)
	}
}

func TestToolSpillGCStartStopsOnCtxCancel(t *testing.T) {
	// D2-f-2：一次性信号必须幂等——cancel 观察窗口内 ticker 可多发一拍，
	// WaitGroup 重复 Done 会打出 negative counter panic（且被 runOnce recover
	// 静默吞走）。buffered chan 非阻塞发送天然幂等。
	firstCycle := make(chan struct{}, 1)
	store := &fakeGCStore{gc: func(context.Context, time.Time) (toolspill.GCStats, error) {
		select {
		case firstCycle <- struct{}{}:
		default:
		}
		return toolspill.GCStats{}, nil
	}}
	w := NewToolSpillGC(store, "fs", 10*time.Millisecond, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Start(ctx)
	}()
	<-firstCycle
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}
