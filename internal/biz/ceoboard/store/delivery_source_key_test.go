// 投递幂等维度化 (source,key) 测试(评审 2026-11-09 MEDIUM-7):
//   - 同一 (source,key) 重复投递幂等;
//   - 跨源同 key 互不冲突(核心回归:原全局键模型下后到的源被静默去重);
//   - 存储主键确为 source:幂等键 复合串,读侧剥前缀还原裸键;
//   - 未经迁移 20261311 回填的历史裸键行读取恒等兼容。
package store

import (
	"context"
	"testing"
	"time"
)

// 同一源同键去重 + 跨源同键不冲突 + 读路径按源隔离且返回裸键。
func TestDelivery_SourceScopedIdempotency(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	at := time.Now()
	dAranea := &Delivery{Key: "evt-9", Source: "aranea", Event: "opportunity.created", EntityID: "opp-1", At: at, Status: "ok"}
	dHcf := &Delivery{Key: "evt-9", Source: "hcf", Event: "opportunity.created", EntityID: "opp-1", At: at, Status: "ok"}

	if ok, err := s.InsertDelivery(ctx, dAranea); err != nil || !ok {
		t.Fatalf("aranea 首次投递应插入: ok=%v err=%v", ok, err)
	}
	// MEDIUM-7 核心:跨源同 key 必须各自成单(旧全局键模型此处被静默去重丢事件)。
	if ok, err := s.InsertDelivery(ctx, dHcf); err != nil || !ok {
		t.Fatalf("hcf 同键投递不得被 aranea 去重: ok=%v err=%v", ok, err)
	}
	// 同源同键重复投递仍去重。
	if ok, err := s.InsertDelivery(ctx, dAranea); err != nil || ok {
		t.Fatalf("aranea 重复投递应去重: ok=%v err=%v", ok, err)
	}

	// 存储主键为复合串(包内直查)。
	if _, err := s.client.CeoboardWebhookDelivery.Get(ctx, "aranea:evt-9"); err != nil {
		t.Fatalf("存储主键应为 source:幂等键 复合串 aranea:evt-9: %v", err)
	}
	if _, err := s.client.CeoboardWebhookDelivery.Get(ctx, "hcf:evt-9"); err != nil {
		t.Fatalf("存储主键应为 hcf:evt-9: %v", err)
	}

	// 读路径按源隔离,且对外暴露裸幂等键(剥前缀还原)。
	for _, tc := range []struct{ source string }{{"aranea"}, {"hcf"}} {
		ds, err := s.RecentDeliveries(ctx, tc.source, 10)
		if err != nil || len(ds) != 1 {
			t.Fatalf("%s 最近投递应为 1,实际 %d err=%v", tc.source, len(ds), err)
		}
		if ds[0].Key != "evt-9" {
			t.Fatalf("%s 对外键应为裸幂等键 evt-9,实际 %q", tc.source, ds[0].Key)
		}
	}
	// 计数口径按源隔离,互不影响。
	if n, _ := s.CountDeliveries(ctx, "aranea", "", at.Add(-time.Hour)); n != 1 {
		t.Fatalf("aranea 成功投递应为 1,实际 %d", n)
	}
	if n, _ := s.CountDeliveries(ctx, "hcf", "", at.Add(-time.Hour)); n != 1 {
		t.Fatalf("hcf 成功投递应为 1,实际 %d", n)
	}
}

// MarkDelivery 按同一复合主键定位:只改目标源的行,不串源。
func TestMarkDelivery_SourceScoped(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	at := time.Now()
	for _, src := range []string{"aranea", "hcf"} {
		if _, err := s.InsertDelivery(ctx, &Delivery{Key: "evt-1", Source: src, Event: "opportunity.created", At: at, Status: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkDelivery(ctx, "hcf", "evt-1", "failed", "处理失败"); err != nil {
		t.Fatal(err)
	}
	hcf, _ := s.RecentDeliveries(ctx, "hcf", 10)
	if hcf[0].Status != "failed" || hcf[0].Error != "处理失败" {
		t.Fatalf("hcf 投递应标 failed,实际 %+v", hcf[0])
	}
	aranea, _ := s.RecentDeliveries(ctx, "aranea", 10)
	if aranea[0].Status != "ok" {
		t.Fatalf("aranea 投递不应受影响,实际 %+v", aranea[0])
	}
	// failed 不计成功数;hcf 归零,aranea 仍 1。
	if n, _ := s.CountDeliveries(ctx, "hcf", "", at.Add(-time.Hour)); n != 0 {
		t.Fatalf("hcf failed 投递不计成功,实际 %d", n)
	}
	if n, _ := s.CountDeliveries(ctx, "aranea", "", at.Add(-time.Hour)); n != 1 {
		t.Fatalf("aranea 成功投递应仍为 1,实际 %d", n)
	}
}

// 含 ':' 的键(如摘要回退键 "sha256:…")复合/还原不受冒号歧义影响。
func TestDelivery_KeyWithColonRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	at := time.Now()
	d := &Delivery{Key: "sha256:1a2b3c4d5e6f7080", Source: "hcf", At: at, Status: "ok"}
	if ok, err := s.InsertDelivery(ctx, d); err != nil || !ok {
		t.Fatalf("插入失败: ok=%v err=%v", ok, err)
	}
	if ok, err := s.InsertDelivery(ctx, d); err != nil || ok {
		t.Fatalf("同键重复应去重: ok=%v err=%v", ok, err)
	}
	ds, _ := s.RecentDeliveries(ctx, "hcf", 10)
	if len(ds) != 1 || ds[0].Key != d.Key {
		t.Fatalf("带冒号键应原样还原 %q,实际 %+v", d.Key, ds)
	}
}

// 历史裸键行(未经迁移 20261311 回填,或超长键被回填 guard 跳过)读取恒等兼容。
func TestDelivery_LegacyBareKeyReadable(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	err := s.client.CeoboardWebhookDelivery.Create().
		SetID("legacy-bare-key").
		SetSource("aranea").
		SetEvent("opportunity.created").
		SetAt(time.Now().UTC()).
		Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := s.RecentDeliveries(ctx, "aranea", 10)
	if err != nil || len(ds) != 1 {
		t.Fatalf("裸键行应可读: len=%d err=%v", len(ds), err)
	}
	if ds[0].Key != "legacy-bare-key" {
		t.Fatalf("无前缀行应恒等返回,实际 %q", ds[0].Key)
	}
}
