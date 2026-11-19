// rest-poll / webhook-recv 单测(移植自 ceo-dashboard/internal/adapter/adapter_test.go)。
// 移植差异:SQLite 直开 → storetest 基建(内存唯一命名 / 文件 WAL);存储调用穿 ctx。
package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// rest-poll:成功解析 items;连续失败达到阈值后断路,断路期间不再请求源。
func TestPoller_BreakerOpens(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewPoller(PollerConfig{QPSPerHost: 1000, Timeout: time.Second, FailThreshold: 3, OpenDuration: time.Hour})
	for i := 0; i < 5; i++ {
		p.PollOnce(context.Background(), "k", "GET", srv.URL)
	}
	st := p.Lookup("k")
	if st.FailCount < 3 {
		t.Fatalf("FailCount=%d", st.FailCount)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("断路后仍在请求源: calls=%d, 期望 3", got)
	}
	if !st.CircuitOpen(time.Now()) {
		t.Fatal("断路器应断开")
	}
}

// rest-poll:成功时归一 items 并记录 last_ok_at。
func TestPoller_SuccessExtractsItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"items":[{"state":"queued"},{"state":"done"}],"total":2}`)
	}))
	defer srv.Close()
	p := NewPoller(PollerConfig{QPSPerHost: 1000, Timeout: time.Second})
	p.PollOnce(context.Background(), "k", "GET", srv.URL)
	st := p.Lookup("k")
	if st == nil || st.LastOKAt.IsZero() {
		t.Fatal("未记录成功")
	}
	if len(st.Items) != 2 {
		t.Fatalf("items=%v", st.Items)
	}
}

// rest-poll:items 归一的三种形状(顶层数组 / 单对象包一层 / 非法 JSON)。
func TestPoller_ExtractItemsShapes(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{`[{"a":1},{"a":2},{"a":3}]`, 3}, // 顶层数组
		{`{"state":"queued"}`, 1},        // 单对象包一层
		{`{"items":[{"a":1}]}`, 1},       // items 包装
		{`{not json`, 0},                 // 非法 JSON → nil
	}
	for i, c := range cases {
		if got := len(extractItems([]byte(c.body))); got != c.want {
			t.Fatalf("case %d: len=%d 期望 %d", i, got, c.want)
		}
	}
}

// webhook-recv:幂等键去重;重复投递不计数。
func TestWebhookReceiver_Dedupe(t *testing.T) {
	ctx := context.Background()
	r := NewWebhookReceiver(openTestStore(t), "")
	body := []byte(`{"schema_version":"1","event":"opportunity.created","opportunity":{"id":"o-1","title":"t","status":"open"}}`)
	h := make(http.Header)
	h.Set("Idempotency-Key", "key-1")

	ev1 := r.Ingest(ctx, "aranea", h, body)
	if ev1.Deduped {
		t.Fatal("首次不应判重")
	}
	ev2 := r.Ingest(ctx, "aranea", h, body)
	if !ev2.Deduped {
		t.Fatal("重复投递应判重")
	}
	if n := r.CountSince(ctx, "aranea", "opportunity.", time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("CountSince=%d, 期望 1", n)
	}
	if n := r.CountSince(ctx, "aranea", "", time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("CountSince(all)=%d, 期望 1", n)
	}
}

// webhook-recv:共享密钥校验。
func TestWebhookReceiver_Auth(t *testing.T) {
	r := NewWebhookReceiver(openTestStore(t), "secret")
	h := r.Handler("aranea")
	req := httptest.NewRequest(http.MethodPost, "/v1/ceoboard/webhooks/aranea", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无密钥应 401,实际 %d", rec.Code)
	}
	req2 := httptest.NewRequest(http.MethodPost, "/v1/ceoboard/webhooks/aranea", nil)
	req2.Header.Set("X-CEO-Key", "secret")
	rec2 := httptest.NewRecorder()
	h(rec2, req2)
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("带密钥应 202,实际 %d", rec2.Code)
	}
}

// 核心证据:重启(换库连接)后幂等去重仍成立,数据不丢。
func TestWebhookReceiver_PersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wh.db")
	st1, db1 := openFileStore(t, path)
	r1 := NewWebhookReceiver(st1, "")
	body := []byte(`{"event":"opportunity.created","opportunity":{"id":"o-9","status":"open"}}`)
	h := make(http.Header)
	h.Set("Idempotency-Key", "restart-key")
	r1.Ingest(ctx, "aranea", h, body)
	if err := db1.Close(); err != nil { // "重启":关闭旧连接
		t.Fatal(err)
	}

	st2, _ := openFileStore(t, path)
	r2 := NewWebhookReceiver(st2, "")
	ev := r2.Ingest(ctx, "aranea", h, body)
	if !ev.Deduped {
		t.Fatal("重启后重复投递仍应判重")
	}
	if n := r2.CountSince(ctx, "aranea", "", time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("重启后投递计数应为 1,实际 %d", n)
	}
}

// 处理失败(非法 JSON)进死信;重放后幂等命中不重复计数,死信标记已重放。
func TestWebhookReceiver_DeadLetterReplay(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	r := NewWebhookReceiver(st, "")
	bad := []byte(`{not json`)
	ev := r.Ingest(ctx, "aranea", make(http.Header), bad)
	if ev.Status != "failed" {
		t.Fatalf("非法 JSON 应 failed,实际 %s", ev.Status)
	}
	dls, err := st.ListDeadLetters(ctx, 10)
	if err != nil || len(dls) != 1 {
		t.Fatalf("应产生 1 条死信,实际 %d err=%v", len(dls), err)
	}
	if dls[0].Kind != "webhook-process" || dls[0].Source != "aranea" {
		t.Fatalf("死信字段错误: %+v", dls[0])
	}
	// 重放:原文仍非法 → 再次 failed,但幂等命中(不重复计数),死信标记已重放
	replayed, err := r.ReplayDeadLetter(ctx, dls[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Deduped {
		t.Fatal("重放应幂等命中(原投递已落库)")
	}
	if n := r.CountSince(ctx, "aranea", "", time.Now().Add(-time.Hour)); n != 0 {
		t.Fatalf("failed 投递不计入成功数,实际 %d", n)
	}
	dl, _ := st.GetDeadLetter(ctx, dls[0].ID)
	if dl.ReplayedAt == nil {
		t.Fatal("重放后应标记 replayed_at")
	}
	if _, err := r.ReplayDeadLetter(ctx, dls[0].ID); err == nil {
		t.Fatal("重复重放应被拒绝")
	}
}
