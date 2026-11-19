// 评审 2026-11-09 L6:归档后重投查重(归档内查重方案)演练。
// 核心契约:归档硬删除关闭 event_uid 主键去重窗口后,DeliverAll 重放的老
// 信封事件不得再被当新事件计数;同时不得误杀全新事件、不得阻断归档恢复。
package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 核心场景:归档 → 重投同 UID(received_at 被归一化为 now,occurred_at 仍是
// 原始信封时间)→ 判重,不计入新事件;热库无行、窗口计数为零,且幂等。
func TestArchivedRedeliveryDoesNotRecount(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	old := time.Now().In(archiveZone).AddDate(0, 0, -3)
	ev := testEvent("hcf:arch-redeliv")
	ev.ReceivedAt = old
	ev.OccurredAt = old
	inserted, err := s.InsertEvent(ctx, ev)
	if err != nil || !inserted {
		t.Fatalf("首次摄入应成功: inserted=%v err=%v", inserted, err)
	}
	if _, err := s.ArchiveClosedDays(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if eventExists(t, s, ev.UID) {
		t.Fatal("归档后热数据应已删除")
	}

	// 重投:同一 UID,信封时间不变,received_at 刷新为 now(归一化行为)。
	redelivered := testEvent(ev.UID)
	redelivered.ReceivedAt = time.Now()
	redelivered.OccurredAt = old
	for i := 0; i < 2; i++ { // 重复重投同样判重(幂等)
		inserted, err := s.InsertEvent(ctx, redelivered)
		if err != nil {
			t.Fatalf("第 %d 次重投: %v", i+1, err)
		}
		if inserted {
			t.Fatalf("第 %d 次重投被当新事件计数(L6)", i+1)
		}
	}
	if eventExists(t, s, ev.UID) {
		t.Fatal("重投不得在热库重建行")
	}
	if n, _ := s.TotalEvents(ctx); n != 0 {
		t.Fatalf("重投后热库总行数应为 0,实际 %d", n)
	}
	if n, err := s.CountEvents(ctx, "hcf", "", time.Now().AddDate(0, 0, -30)); err != nil || n != 0 {
		t.Fatalf("宽窗口计数不得包含重投: n=%d err=%v", n, err)
	}
}

// 午夜边界:occurred_at 落在归档日的前一天(跨午夜 + 时钟偏移),须由
// occurred 当日 ±1 天候选命中实际归档日。
func TestArchiveDedupMidnightBoundary(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	occ := time.Date(2026, 11, 3, 23, 30, 0, 0, archiveZone)
	ev := testEvent("hcf:arch-midnight")
	ev.ReceivedAt = time.Date(2026, 11, 4, 0, 30, 0, 0, archiveZone) // 归档在 11-04
	ev.OccurredAt = occ
	if _, err := s.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ArchiveClosedDays(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "events-2026-11-04.jsonl.gz")); err != nil {
		t.Fatalf("归档日应为 received 所在日 11-04: %v", err)
	}

	redelivered := testEvent(ev.UID)
	redelivered.ReceivedAt = time.Now()
	redelivered.OccurredAt = occ
	inserted, err := s.InsertEvent(ctx, redelivered)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("跨午夜重投未被归档日(+1 候选)判重")
	}
}

// 反误杀:老信封的全新事件(迟到新事件,UID 从未出现)必须正常摄入。
func TestArchiveDedupDoesNotBlockFreshOldEnvelopeEvent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.ConfigureArchive(filepath.Join(t.TempDir(), "archive"), 1); err != nil {
		t.Fatal(err)
	}
	fresh := testEvent("hcf:late-but-new")
	fresh.ReceivedAt = time.Now()
	fresh.OccurredAt = time.Now().In(archiveZone).AddDate(0, 0, -5)
	inserted, err := s.InsertEvent(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted || !eventExists(t, s, fresh.UID) {
		t.Fatal("老信封全新事件不得被归档查重误杀")
	}
}

// 零成本路径:新信封事件(occurred_at 未老于截止线)不触发任何归档探测,
// 直接落库。
func TestArchiveDedupSkipsRecentEnvelope(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.ConfigureArchive(filepath.Join(t.TempDir(), "archive"), 1); err != nil {
		t.Fatal(err)
	}
	ev := testEvent("hcf:recent")
	ev.ReceivedAt = time.Now()
	ev.OccurredAt = time.Now().Add(-time.Hour)
	inserted, err := s.InsertEvent(ctx, ev)
	if err != nil || !inserted {
		t.Fatalf("新信封事件应直接落库: inserted=%v err=%v", inserted, err)
	}
	if len(s.archDedup.order) != 0 {
		t.Fatalf("新信封摄入不得触发归档日加载: %v", s.archDedup.order)
	}
}

// 退化语义:归档清单损坏时查重"只容漏、不容错"——放行进库(由热库主键继续
// 兜底),绝不因查重误杀事件;也不缓存损坏结果(修复后下轮重投应能判重)。
func TestArchiveDedupDegradedOnCorruptArchive(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	old := time.Now().In(archiveZone).AddDate(0, 0, -3)
	ev := testEvent("hcf:arch-corrupt")
	ev.ReceivedAt = old
	ev.OccurredAt = old
	if _, err := s.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ArchiveClosedDays(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "events-"+old.Format("2006-01-02")+".jsonl.gz.sha256.json")
	if err := os.WriteFile(manifest, []byte(`{"sha256":"corrupt"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	redelivered := testEvent(ev.UID)
	redelivered.ReceivedAt = time.Now()
	redelivered.OccurredAt = old
	inserted, err := s.InsertEvent(ctx, redelivered)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("清单损坏时不得把无从证明当命中(误杀事件)")
	}

	// 修复清单后重投恢复判重(损坏结果未被缓存)。
	if err := repairArchiveManifest(filepath.Join(dir, "events-"+old.Format("2006-01-02")+".jsonl.gz")); err != nil {
		t.Fatal(err)
	}
	inserted, err = s.InsertEvent(ctx, redelivered)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("清单修复后重投应恢复判重")
	}
}

// 恢复旁路:RestoreArchive 直调 insertEventWithFlags,带老信封时间的归档事件
// 必须能恢复(不被查重拒收);二次恢复由热库主键幂等。
func TestRestoreArchiveBypassesDedupForOldEnvelopes(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	old := time.Now().In(archiveZone).AddDate(0, 0, -3)
	ev := testEvent("hcf:arch-restore")
	ev.ReceivedAt = old
	ev.OccurredAt = old
	if _, err := s.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ArchiveClosedDays(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events-"+old.Format("2006-01-02")+".jsonl.gz")
	imported, err := s.RestoreArchive(ctx, path)
	if err != nil || imported != 1 {
		t.Fatalf("老信封归档事件必须可恢复(旁路查重): imported=%d err=%v", imported, err)
	}
	imported, err = s.RestoreArchive(ctx, path)
	if err != nil || imported != 0 {
		t.Fatalf("二次恢复应幂等: imported=%d err=%v", imported, err)
	}
}
