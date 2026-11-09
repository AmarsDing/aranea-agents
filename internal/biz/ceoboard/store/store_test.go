// store 核心语义测试(自 ceo-dashboard/internal/store/store_test.go 移植,
// SQLite→Ent/SQLite(modernc),API 加 ctx;并发用例改走 Ent 连接池)。
package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// 事件幂等:同 event_uid 重复插入只算一次(重启后仍成立——库里已有记录)。
func TestInsertEvent_Idempotent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := &Event{UID: "hcf:evt_1", Platform: "hcf", Subject: "mm.intel.raw", SubjectKey: "mm.intel.raw",
		EventID: "evt_1", ReceivedAt: time.Now()}
	ok, err := s.InsertEvent(ctx, ev)
	if err != nil || !ok {
		t.Fatalf("首次插入应成功: ok=%v err=%v", ok, err)
	}
	ok, err = s.InsertEvent(ctx, ev)
	if err != nil || ok {
		t.Fatalf("重复插入应被去重: ok=%v err=%v", ok, err)
	}
	n, _ := s.TotalEvents(ctx)
	if n != 1 {
		t.Fatalf("总数应为 1,实际 %d", n)
	}
}

// 窗口计数按 received_at;subject_key 过滤。
func TestCountEvents_Window(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	now := time.Now()
	old := &Event{UID: "hcf:old", Platform: "hcf", Subject: "a.v1", SubjectKey: "a", ReceivedAt: now.Add(-2 * time.Hour)}
	fresh := &Event{UID: "hcf:new", Platform: "hcf", Subject: "a.v2", SubjectKey: "a", ReceivedAt: now}
	other := &Event{UID: "nt:new", Platform: "novatrader", Subject: "a", SubjectKey: "a", ReceivedAt: now}
	for _, ev := range []*Event{old, fresh, other} {
		if _, err := s.InsertEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.CountEvents(ctx, "hcf", "a", now.Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("窗口计数应为 1(仅 fresh),实际 %d err=%v", n, err)
	}
	// 版本后缀不同但 subject_key 相同 → 版本演进不丢事件(都计入)
	n, _ = s.CountEvents(ctx, "hcf", "a", now.Add(-3*time.Hour))
	if n != 2 {
		t.Fatalf("跨版本同 key 应计 2,实际 %d", n)
	}
}

// webhook 投递幂等 + 死信重放闭环。
func TestDeliveryAndDeadLetter(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	d := &Delivery{Key: "k1", Source: "aranea", Event: "opportunity.created", At: time.Now(), Status: "ok"}
	ok, _ := s.InsertDelivery(ctx, d)
	if !ok {
		t.Fatal("首次投递应插入")
	}
	ok, _ = s.InsertDelivery(ctx, d)
	if ok {
		t.Fatal("重复投递应去重")
	}
	if err := s.MarkDelivery(ctx, "aranea", "k1", "failed", "处理失败"); err != nil {
		t.Fatal(err)
	}
	n, _ := s.CountDeliveries(ctx, "aranea", "", time.Now().Add(-time.Hour))
	if n != 0 {
		t.Fatalf("failed 投递不计入成功数,实际 %d", n)
	}

	id, err := s.AddDeadLetter(ctx, "webhook-process", "aranea", "k1", "{}", "处理失败")
	if err != nil {
		t.Fatal(err)
	}
	pending, _ := s.PendingDeadLetters(ctx)
	if pending != 1 {
		t.Fatalf("待重放死信应为 1,实际 %d", pending)
	}
	dl, err := s.GetDeadLetter(ctx, id)
	if err != nil || dl.RefKey != "k1" {
		t.Fatalf("取死信失败: %+v err=%v", dl, err)
	}
	if err := s.MarkReplayed(ctx, id); err != nil {
		t.Fatal(err)
	}
	pending, _ = s.PendingDeadLetters(ctx)
	if pending != 0 {
		t.Fatalf("重放后待重放应为 0,实际 %d", pending)
	}
}

// 死信幂等:同 (kind,source,ref_key) 未重放时重复写入返回已有 id。
func TestDeadLetter_IdempotentRefKey(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	id1, err := s.AddDeadLetter(ctx, "webhook-receive", "aranea", "key-1", "{}", "bad json")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.AddDeadLetter(ctx, "webhook-receive", "aranea", "key-1", "{}", "bad json")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("未重放同键死信应幂等返回 %d,实际 %d", id1, id2)
	}
	pending, _ := s.PendingDeadLetters(ctx)
	if pending != 1 {
		t.Fatalf("幂等写应只有 1 条死信,实际 %d", pending)
	}
	// 重放后再写是新死信
	if err := s.MarkReplayed(ctx, id1); err != nil {
		t.Fatal(err)
	}
	id3, err := s.AddDeadLetter(ctx, "webhook-receive", "aranea", "key-1", "{}", "bad json again")
	if err != nil {
		t.Fatal(err)
	}
	if id3 == id1 {
		t.Fatal("已重放后同键应生成新死信")
	}
}

// 并发写争用:文件库 WAL + busy_timeout 下,N 协程并发写不出 BUSY;
// 其中跨协程重复 uid,验证并发下幂等仍成立。
func TestConcurrentWrites_NoBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")
	s, db := openFileStore(t, path)
	t.Cleanup(func() { s.client.Close(); db.Close() })
	ctx := context.Background()

	const workers = 8
	const perWorker = 50 // 每协程 25 新 + 25 重复
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				uid := fmt.Sprintf("u%d", i%25) // 跨协程重复的 uid
				_, err := s.InsertEvent(ctx, &Event{UID: fmt.Sprintf("w%d:%s", w, uid), Platform: "hcf",
					Subject: "mm.intel.raw", SubjectKey: "mm.intel.raw", EventID: uid, ReceivedAt: time.Now()})
				if err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发写出错: %v", err)
	}
	n, _ := s.TotalEvents(ctx)
	if n != workers*25 {
		t.Fatalf("并发写入总数应为 %d,实际 %d", workers*25, n)
	}
}

// latency 样本:B 侧幂等(同 metric+b_uid 重复忽略)。
func TestLatencySample_BSideIdempotent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	sm := &LatencySample{MetricID: "m.x", BUID: "b1", AUID: "a1", EntityID: "e1", LatencyMs: 120, At: time.Now()}
	ok, err := s.UpsertLatencySample(ctx, sm)
	if err != nil || !ok {
		t.Fatalf("首写应插入: ok=%v err=%v", ok, err)
	}
	ok, err = s.UpsertLatencySample(ctx, sm)
	if err != nil || ok {
		t.Fatalf("重复样本应忽略: ok=%v err=%v", ok, err)
	}
	vals, err := s.LatencyWindow(ctx, "m.x", time.Now().Add(-time.Hour))
	if err != nil || len(vals) != 1 || vals[0] != 120 {
		t.Fatalf("窗口应只有 1 条样本 120: %v err=%v", vals, err)
	}
}

// ListPairEvents:entity_id 非空 + received_at 窗口;occurred_at 缺失退 received_at。
func TestListPairEvents_OccurredFallback(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	now := time.Now()
	occ := now.Add(-2 * time.Second)
	withOcc := &Event{UID: "hcf:p1", Platform: "hcf", Subject: "s.v1", SubjectKey: "s",
		EntityID: "e1", OccurredAt: occ, ReceivedAt: now}
	noOcc := &Event{UID: "hcf:p2", Platform: "hcf", Subject: "s.v1", SubjectKey: "s",
		EntityID: "e2", ReceivedAt: now}
	noEntity := &Event{UID: "hcf:p3", Platform: "hcf", Subject: "s.v1", SubjectKey: "s",
		ReceivedAt: now}
	for _, ev := range []*Event{withOcc, noOcc, noEntity} {
		if _, err := s.InsertEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	pairs, err := s.ListPairEvents(ctx, "hcf", "s", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 {
		t.Fatalf("无 entity 事件不得进入配对: %+v", pairs)
	}
	got := map[string]time.Time{}
	for _, p := range pairs {
		got[p.UID] = p.At
	}
	if !got["hcf:p1"].Equal(occ.UTC()) {
		t.Fatalf("有 occurred_at 应用信封时间: %v ≠ %v", got["hcf:p1"], occ)
	}
	if !got["hcf:p2"].Equal(noOcc.ReceivedAt.UTC()) {
		t.Fatalf("无 occurred_at 应退 received_at: %v", got["hcf:p2"])
	}
}
