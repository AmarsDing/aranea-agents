// 幂等键长度预算钳制测试(评审 2026-11-09 R1):
// 复合存储主键 = source + ":" + 幂等键,上限 512(source ≤64)→ 447;adapter 落
// 事件时 event_id 列直接取键(MaxLen 256)——故预算取下限 256。超预算键在
// adapter 入口即 4xx 拒收,且不留任何库足迹(投递/事件/死信都不写——写了也只
// 在 ent 校验处失败,源把 503 当瞬态故障无限重试 = 毒消息);预算内键(含 256
// 边界)行为不变。store 层另留复合键 >512 的防御兜底。
package adapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/ceoboard/store"
)

// postWebhookHeader 在 postWebhook 基础上携带请求头(幂等键头来源验证用)。
func postWebhookHeader(r *WebhookReceiver, source string, header http.Header, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/ceoboard/webhooks/"+source, strings.NewReader(body))
	for k, vals := range header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	r.Handler(source).ServeHTTP(rec, req)
	return rec
}

// 448 字符 Idempotency-Key:HTTP 400,且投递/事件/死信三表零足迹。
func TestWebhookIdempotencyKeyOverBudgetRejected(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	r := NewWebhookReceiver(st, "")
	longKey := strings.Repeat("k", store.MaxIdempotencyKeyLen+1)
	header := http.Header{"Idempotency-Key": []string{longKey}}
	body := `{"event_id":"evt-r1-over","event":"x.y"}`

	ev := r.Ingest(ctx, "hcf", header, []byte(body))
	if !errors.Is(ev.PersistErr, store.ErrIdempotencyKeyTooLong) {
		t.Fatalf("Ingest 须报 ErrIdempotencyKeyTooLong: %v", ev.PersistErr)
	}
	resp := postWebhookHeader(r, "hcf", header, body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("超预算键应 400,实际 %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"accepted":false`) {
		t.Fatalf("400 响应须带 accepted=false: %s", resp.Body.String())
	}
	// 零足迹:超预算键不落任何表。
	if n := r.CountSince(ctx, "hcf", "", time.Now().Add(-time.Hour)); n != 0 {
		t.Fatalf("超预算键不得留投递足迹: %d", n)
	}
	if n, err := st.TotalEvents(ctx); err != nil || n != 0 {
		t.Fatalf("超预算键不得留事件足迹: n=%d err=%v", n, err)
	}
	if n, err := st.PendingDeadLetters(ctx); err != nil || n != 0 {
		t.Fatalf("超预算键不得留死信足迹: n=%d err=%v", n, err)
	}
}

// 447 字符边界键:全额预算必须受理(202),且以原键落库(dedup 身份不被改写)。
func TestWebhookIdempotencyKeyAtBudgetAccepted(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	r := NewWebhookReceiver(st, "")
	key := strings.Repeat("k", store.MaxIdempotencyKeyLen)
	header := http.Header{"Idempotency-Key": []string{key}}
	body := `{"event_id":"evt-r1-at","event":"x.y"}`

	resp := postWebhookHeader(r, "hcf", header, body)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("预算边界键应 202,实际 %d body=%s", resp.Code, resp.Body.String())
	}
	evs := r.Events(ctx, "hcf", 10)
	if len(evs) != 1 || evs[0].Key != key {
		t.Fatalf("预算边界键须原样落库(不截断不替换): %+v", evs)
	}
	// 同源同键重投仍去重(幂等语义不破)。
	ev := r.Ingest(ctx, "hcf", header, []byte(body))
	if !ev.Deduped || ev.PersistErr != nil {
		t.Fatalf("边界键重复投递应去重: deduped=%v err=%v", ev.Deduped, ev.PersistErr)
	}
}

// store 层防御:非 adapter 调用方直插复合键 >512 的投递,必须吃显式哨兵错误
// (而非 ent 校验裸错);预算内复合键行为不变(adapter 预算 256 远松于该兜底)。
func TestStoreDeliveryCompositeKeyOver512Guard(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	// 复合键 = 64(source 上限) + ":" + 449 = 514 > 512 → 触发兜底。
	overKey := strings.Repeat("k", 512-64) // 449
	_, err := st.InsertDelivery(ctx, &store.Delivery{
		Source: strings.Repeat("s", 64), // 上限 64 的 source
		Key:    overKey,
		Event:  "x.y", At: time.Now(),
	})
	if !errors.Is(err, store.ErrIdempotencyKeyTooLong) {
		t.Fatalf("复合键超 512 必须报哨兵错误: %v", err)
	}
	if err := st.MarkDelivery(ctx, strings.Repeat("s", 64), overKey, "ok", ""); !errors.Is(err, store.ErrIdempotencyKeyTooLong) {
		t.Fatalf("MarkDelivery 复合键超 512 必须报哨兵错误: %v", err)
	}
	// 预算内的合法复合键:正常受理。
	ok, err := st.InsertDelivery(ctx, &store.Delivery{
		Source: strings.Repeat("s", 64),
		Key:    strings.Repeat("k", store.MaxIdempotencyKeyLen),
		Event:  "x.y", At: time.Now(),
	})
	if err != nil || !ok {
		t.Fatalf("预算内复合键应正常受理: ok=%v err=%v", ok, err)
	}
}
