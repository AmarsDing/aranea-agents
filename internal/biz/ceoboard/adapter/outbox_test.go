// aranea 进程内 outbox 直投单测(P2-6 新增):与 HTTP 投递同语义——
// 幂等键去重、实体 id 进实体列、统一事件流落库、失败可重试。
package adapter

import (
	"context"
	"testing"
	"time"
)

// 直投事件归一:幂等键与实体 id 落位;重复直投去重不计数。
func TestOutbox_EmitDedupe(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	r := NewWebhookReceiver(st, "")
	ob := r.Outbox("aranea")

	ev, err := ob.Emit(ctx, "opportunity.created", "evt-1", "opp-1", map[string]any{"score": 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Deduped || ev.Key != "evt-1" || ev.EntityID != "opp-1" || ev.Event != "opportunity.created" {
		t.Fatalf("直投归一错误: %+v", ev)
	}
	// 重复直投:去重命中
	ev2, err := ob.Emit(ctx, "opportunity.created", "evt-1", "opp-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ev2.Deduped {
		t.Fatal("重复直投应判重")
	}
	if n := r.CountSince(ctx, "aranea", "", time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("去重后成功投递应为 1,实际 %d", n)
	}
	// 统一事件流:实体 id 进实体列(供 latency 配对/下钻)
	got, err := st.GetEvent(ctx, "aranea:evt-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.EntityID != "opp-1" || got.Platform != "aranea" || got.SubjectKey != "opportunity.created" {
		t.Fatalf("统一事件流落库错误: %+v", got)
	}
}

// 保留键防误填:payload 里的 event/idempotency_key/entity_id 由参数强制覆盖。
func TestOutbox_ReservedKeysOverridePayload(t *testing.T) {
	ctx := context.Background()
	r := NewWebhookReceiver(openTestStore(t), "")
	ob := r.Outbox("aranea")
	ev, err := ob.Emit(ctx, "real.event", "real-key", "real-entity",
		map[string]any{"event": "fake", "idempotency_key": "fake", "entity_id": "fake", "x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Event != "real.event" || ev.Key != "real-key" || ev.EntityID != "real-entity" {
		t.Fatalf("保留键必须由参数覆盖: %+v", ev)
	}
}

// 幂等键为空:退实体 id → 内容摘要(与 HTTP 链路同一优先级链)。
func TestOutbox_KeyFallbackChain(t *testing.T) {
	ctx := context.Background()
	r := NewWebhookReceiver(openTestStore(t), "")
	ob := r.Outbox("aranea")
	ev, err := ob.Emit(ctx, "metric.tick", "", "entity-7", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Key != "entity-7" {
		t.Fatalf("空幂等键应退实体 id: %q", ev.Key)
	}
	ev2, err := ob.Emit(ctx, "metric.tick", "", "", map[string]any{"v": 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(ev2.Key) < 7 || ev2.Key[:7] != "sha256:" {
		t.Fatalf("皆空应退内容摘要: %q", ev2.Key)
	}
}
