// 评审 2026-11-09 L3:PollOnce 返回快照(不共享内部状态指针)与并发隔离演练。
package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// PollOnce 返回值必须是锁内快照:改写返回值不得影响内部状态;不同 key 互不串话。
func TestPollOnce_ReturnsSnapshotNotSharedPointer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"items":[{"state":"queued"}]}`)
	}))
	defer srv.Close()
	p := NewPoller(PollerConfig{QPSPerHost: 10000, Timeout: time.Second})
	ctx := context.Background()

	got := p.PollOnce(ctx, "node/wf/a", "GET", srv.URL)
	if got == nil || got.LastOKAt.IsZero() {
		t.Fatal("首次轮询应成功")
	}
	// 改写返回值(模拟调用方持有并篡改)——内部状态不得随之变化。
	got.FailCount = -999
	got.Items = nil
	got.LastErr = "tampered"
	st := p.Lookup("node/wf/a")
	if st.FailCount != 0 || st.LastErr != "" || len(st.Items) != 1 {
		t.Fatalf("返回值与内部状态共享指针(串话): %+v", st)
	}
	// 不同 key 的状态互不共享:另一 key 失败不得影响本 key。
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	p.PollOnce(ctx, "node/wf/b", "GET", bad.URL)
	stA, stB := p.Lookup("node/wf/a"), p.Lookup("node/wf/b")
	if stA.FailCount != 0 || stB.FailCount != 1 {
		t.Fatalf("跨 key 状态串话: a=%+v b=%+v", stA, stB)
	}
}

// 断路中的返回值同样是快照:篡改返回的 OpenUntil 不得改内部断路状态。
func TestPollOnce_CircuitOpenReturnsSnapshot(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	p := NewPoller(PollerConfig{QPSPerHost: 10000, Timeout: time.Second, FailThreshold: 3, OpenDuration: time.Hour})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		p.PollOnce(ctx, "k", "GET", bad.URL)
	}
	got := p.PollOnce(ctx, "k", "GET", bad.URL) // 断路中:直接返回
	if !got.CircuitOpen(time.Now()) {
		t.Fatal("断路中返回的状态应报告 CircuitOpen")
	}
	got.OpenUntil = time.Time{} // 篡改快照:试图"合上"断路器
	st := p.Lookup("k")
	if !st.CircuitOpen(time.Now()) {
		t.Fatal("篡改返回值不得改变内部断路状态")
	}
}

// 并发轮询:同 key 多协程 + 跨 key 并行,状态各自收敛、互不串话。
func TestPollOnce_ConcurrentIsolation(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"items":[{"state":"queued"}]}`)
	}))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()
	p := NewPoller(PollerConfig{QPSPerHost: 10000, Timeout: time.Second})
	ctx := context.Background()

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(2)
		// 同 key 多协程(重复接线/手工触发的放大面)。
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				p.PollOnce(ctx, "node/wf/a", "GET", okSrv.URL)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				p.PollOnce(ctx, "node/wf/b", "GET", badSrv.URL)
			}
		}()
	}
	wg.Wait()

	stA, stB := p.Lookup("node/wf/a"), p.Lookup("node/wf/b")
	if stA == nil || stA.LastOKAt.IsZero() || stA.FailCount != 0 || len(stA.Items) != 1 {
		t.Fatalf("成功 key 状态被串话: %+v", stA)
	}
	if stB == nil || stB.FailCount == 0 || stB.LastErr == "" || !stB.LastOKAt.IsZero() {
		t.Fatalf("失败 key 状态被串话: %+v", stB)
	}
	if stB.Items != nil {
		t.Fatalf("失败 key 不得持有成功 key 的 items: %+v", stB.Items)
	}
}
