// 跨源投递幂等维度化的接收端集成测试(评审 2026-11-09 MEDIUM-7):
// 不同 webhook 源携带相同幂等键(同 event_id)投递时,必须各自受理并计数——
// 原全局幂等键模型下后到的源被静默去重(deduped=true,不计数,事件丢失)。
package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// postWebhook 走 HTTP 层投递(与本包 persistence_test 同形态)。
func postWebhook(r *WebhookReceiver, source, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/ceoboard/webhooks/"+source, strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.Handler(source).ServeHTTP(rec, req)
	return rec
}

// 两个源以相同 event_id 投递:双方均应受理为非去重,计数与最近投递各自独立。
func TestWebhookCrossSourceSameKeyNoConflict(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	r := NewWebhookReceiver(st, "")
	body := `{"event_id":"evt-shared-1","event":"opportunity.created","opportunity":{"id":"opp-1"}}`

	ev1 := r.Ingest(ctx, "aranea", make(http.Header), []byte(body))
	if ev1.PersistErr != nil || ev1.Deduped {
		t.Fatalf("aranea 首次投递应受理非去重: deduped=%v err=%v", ev1.Deduped, ev1.PersistErr)
	}
	ev2 := r.Ingest(ctx, "hcf", make(http.Header), []byte(body))
	if ev2.PersistErr != nil {
		t.Fatalf("hcf 同键投递不应持久化失败: %v", ev2.PersistErr)
	}
	if ev2.Deduped {
		t.Fatal("hcf 同键投递不得被 aranea 去重(MEDIUM-7 回归)")
	}

	// 计数按源隔离。
	if n := r.CountSince(ctx, "aranea", "", time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("aranea 成功投递应为 1,实际 %d", n)
	}
	if n := r.CountSince(ctx, "hcf", "", time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("hcf 成功投递应为 1,实际 %d", n)
	}
	// 同源重复投递仍去重(幂等不丢)。
	ev3 := r.Ingest(ctx, "aranea", make(http.Header), []byte(body))
	if !ev3.Deduped || ev3.PersistErr != nil {
		t.Fatalf("aranea 重复投递应去重: deduped=%v err=%v", ev3.Deduped, ev3.PersistErr)
	}
	// 最近投递对外暴露裸幂等键。
	for _, src := range []string{"aranea", "hcf"} {
		evs := r.Events(ctx, src, 10)
		if len(evs) != 1 || evs[0].Key != "evt-shared-1" {
			t.Fatalf("%s 最近投递应为 1 条裸键 evt-shared-1,实际 %+v", src, evs)
		}
	}
}

// 显式 Idempotency-Key 头跨源相同:同样不跨源去重(键构造与头来源无关)。
func TestWebhookCrossSourceSameIdempotencyHeader(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	r := NewWebhookReceiver(st, "")
	header := http.Header{"Idempotency-Key": []string{"idem-xyz"}}
	body := `{"event":"a.b"}`
	if ev := r.Ingest(ctx, "novatrader", header, []byte(body)); ev.PersistErr != nil || ev.Deduped {
		t.Fatalf("novatrader 首次应受理: deduped=%v err=%v", ev.Deduped, ev.PersistErr)
	}
	if ev := r.Ingest(ctx, "workbench", header, []byte(body)); ev.PersistErr != nil || ev.Deduped {
		t.Fatalf("workbench 同头键不得被跨源去重: deduped=%v err=%v", ev.Deduped, ev.PersistErr)
	}
}

// 鉴权路径顺带验证:HTTP 层跨源投递均 202(防 5xx 误判回补)。仅核对 store 层结果。
func TestWebhookCrossSourceHandlerAccepted(t *testing.T) {
	st := openTestStore(t)
	r := NewWebhookReceiver(st, "")
	rec1 := postWebhook(r, "aranea", `{"event_id":"hs-1","event":"x.y"}`)
	rec2 := postWebhook(r, "hcf", `{"event_id":"hs-1","event":"x.y"}`)
	if rec1.Code != http.StatusAccepted || rec2.Code != http.StatusAccepted {
		t.Fatalf("跨源同键均应 202 受理,实际 %d / %d", rec1.Code, rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), `"accepted":true`) || strings.Contains(rec2.Body.String(), `"deduped":true`) {
		t.Fatalf("hcf 响应应为非去重受理,实际 %s", rec2.Body.String())
	}
}
