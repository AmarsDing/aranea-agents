// UTC+8 闭日归档测试(自 ceo-dashboard/internal/store/archive_test.go 移植,
// API 加 ctx;行存废断言改走 eventExists/GetEvent,不再裸 SQL)。
package store

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchiveClosedDayVerifiesBeforeDelete(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	old := testEvent("hcf:closed-day")
	old.ReceivedAt = time.Date(2026, 11, 3, 7, 0, 0, 0, time.FixedZone("CST", 8*3600))
	current := testEvent("hcf:current-day")
	current.ReceivedAt = time.Date(2026, 11, 4, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))
	for _, ev := range []*Event{old, current} {
		if _, err := s.InsertEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	archived, err := s.ArchiveClosedDays(ctx, now)
	if err != nil || archived != 1 {
		t.Fatalf("应归档 1 个闭日: archived=%d err=%v", archived, err)
	}
	if eventExists(t, s, old.UID) || !eventExists(t, s, current.UID) {
		t.Fatal("只应删除已校验闭日数据")
	}
	path := filepath.Join(dir, "events-2026-11-03.jsonl.gz")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var rec Event
	if err := json.NewDecoder(zr).Decode(&rec); err != nil {
		t.Fatal(err)
	}
	if rec.UID != old.UID || rec.Payload != old.Payload {
		t.Fatalf("归档原文不完整: %+v", rec)
	}
	if _, err := os.Stat(path + ".sha256.json"); err != nil {
		t.Fatalf("归档必须有 checksum 清单: %v", err)
	}
}

func TestArchiveRestoreIsIdempotentAndKeepsFile(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	ev := testEvent("hcf:restore")
	ev.ReceivedAt = time.Date(2026, 11, 3, 8, 0, 0, 0, time.FixedZone("CST", 8*3600))
	_, _ = s.InsertEvent(ctx, ev)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if _, err := s.ArchiveClosedDays(ctx, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events-2026-11-03.jsonl.gz")
	imported, err := s.RestoreArchive(ctx, path)
	if err != nil || imported != 1 {
		t.Fatalf("首次恢复应导入 1 条: imported=%d err=%v", imported, err)
	}
	imported, err = s.RestoreArchive(ctx, path)
	if err != nil || imported != 0 {
		t.Fatalf("重复恢复应幂等: imported=%d err=%v", imported, err)
	}
	if n, _ := s.TotalEvents(ctx); n != 1 {
		t.Fatalf("恢复后应只有 1 行,实际 %d", n)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("恢复不得删除归档: %v", err)
	}
}

func TestArchiveCorruptManifestDoesNotDeleteEvents(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	ev := testEvent("hcf:manifest")
	ev.ReceivedAt = time.Date(2026, 11, 3, 8, 0, 0, 0, time.FixedZone("CST", 8*3600))
	_, _ = s.InsertEvent(ctx, ev)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if _, err := s.ArchiveClosedDays(ctx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "events-2026-11-03.jsonl.gz.sha256.json")
	if err := os.WriteFile(manifest, []byte(`{"sha256":"corrupt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ArchiveClosedDays(ctx, now); err == nil {
		t.Fatal("清单损坏必须停止归档删除")
	}
	if !eventExists(t, s, ev.UID) {
		t.Fatal("checksum 校验失败时热数据被删除")
	}
}

// 崩溃续作:发布归档后、删库前崩溃 → 下次确认完整性后续删。
func TestArchiveExistingCompleteArchiveFinishesDelete(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	ev := testEvent("hcf:crash-after-publish")
	ev.ReceivedAt = time.Date(2026, 11, 3, 8, 0, 0, 0, time.FixedZone("CST", 8*3600))
	_, _ = s.InsertEvent(ctx, ev)
	ev.PayloadComplete = true
	path := filepath.Join(dir, "events-2026-11-03.jsonl.gz")
	if err := writeArchive(path, []Event{*ev}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	archived, err := s.ArchiveClosedDays(ctx, now)
	if err != nil || archived != 1 {
		t.Fatalf("归档已发布但删库前崩溃后应可续作: archived=%d err=%v", archived, err)
	}
	if n, _ := s.TotalEvents(ctx); n != 0 {
		t.Fatalf("已校验归档对应的热数据应删除,实际 %d", n)
	}
}

// 归档发布后被补齐的热行不得按归档的截断副本续删。
func TestArchiveContinuationKeepsPayloadRepairedAfterPublish(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	legacy := testEvent("hcf:repaired-after-publish")
	legacy.ReceivedAt = time.Date(2026, 11, 3, 8, 0, 0, 0, archiveZone)
	legacy.Payload = "truncated"
	if _, err := s.insertEventWithFlags(ctx, legacy, false, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events-2026-11-03.jsonl.gz")
	if err := writeArchive(path, []Event{*legacy}); err != nil {
		t.Fatal(err)
	}
	full := testEvent(legacy.UID)
	full.ReceivedAt = legacy.ReceivedAt
	full.Payload = `{"data":"complete"}`
	if _, err := s.InsertEvent(ctx, full); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 4, 12, 0, 0, 0, archiveZone)
	archived, err := s.ArchiveClosedDays(ctx, now)
	if err == nil || archived != 0 {
		t.Fatalf("归档仍是截断副本时不得续删: archived=%d err=%v", archived, err)
	}
	got, err := s.GetEvent(ctx, legacy.UID)
	if err != nil {
		t.Fatalf("已补齐热行必须保留: %v", err)
	}
	if got.Payload != full.Payload || !got.PayloadComplete {
		t.Fatalf("归档续作覆盖了补齐原文: payload=%q complete=%v", got.Payload, got.PayloadComplete)
	}
}

// 删除守卫:热行在快照后补齐(0→1),按旧快照删除必须整体回滚。
func TestArchiveDeleteRejectsStaleIncompleteSnapshot(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	legacy := testEvent("hcf:stale-archive-snapshot")
	legacy.Payload = "truncated"
	if _, err := s.insertEventWithFlags(ctx, legacy, false, false); err != nil {
		t.Fatal(err)
	}
	snapshot := []Event{*legacy}
	full := testEvent(legacy.UID)
	full.Payload = `{"data":"complete"}`
	if _, err := s.InsertEvent(ctx, full); err != nil {
		t.Fatal(err)
	}

	if err := s.deleteArchivedEvents(ctx, snapshot); err == nil {
		t.Fatal("热行已补齐后不得按旧的截断快照删除")
	}
	got, err := s.GetEvent(ctx, legacy.UID)
	if err != nil {
		t.Fatalf("旧快照删除必须回滚并保留补齐热行: %v", err)
	}
	if got.Payload != full.Payload || !got.PayloadComplete {
		t.Fatalf("旧快照删除损坏了补齐原文: payload=%q complete=%v", got.Payload, got.PayloadComplete)
	}
}

// 数据文件已发布但清单丢失(两次改名之间崩溃)→ 重建清单后续作。
func TestArchiveDataWithoutManifestIsRepaired(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	ev := testEvent("hcf:orphan-data")
	ev.ReceivedAt = time.Date(2026, 11, 3, 8, 0, 0, 0, archiveZone)
	_, _ = s.InsertEvent(ctx, ev)
	ev.PayloadComplete = true
	path := filepath.Join(dir, "events-2026-11-03.jsonl.gz")
	if err := writeArchive(path, []Event{*ev}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path + ".sha256.json"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, archiveZone)
	archived, err := s.ArchiveClosedDays(ctx, now)
	if err != nil || archived != 1 {
		t.Fatalf("两次改名之间崩溃留下数据文件后应可续作: archived=%d err=%v", archived, err)
	}
	if err := verifyArchive(path); err != nil {
		t.Fatalf("续作后归档与清单必须一致: %v", err)
	}
	if n, _ := s.TotalEvents(ctx); n != 0 {
		t.Fatalf("完整归档发布后热数据应删除,实际 %d", n)
	}
}

// 迟到事件只保留当日热数据,不阻断后续闭日归档。
func TestArchiveLateEventDoesNotBlockLaterDays(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	first := testEvent("hcf:first")
	first.ReceivedAt = time.Date(2026, 11, 2, 8, 0, 0, 0, archiveZone)
	late := testEvent("hcf:late")
	late.ReceivedAt = first.ReceivedAt.Add(time.Hour)
	second := testEvent("hcf:second-day")
	second.ReceivedAt = time.Date(2026, 11, 3, 8, 0, 0, 0, archiveZone)
	for _, ev := range []*Event{first, late, second} {
		_, _ = s.InsertEvent(ctx, ev)
	}
	first.PayloadComplete = true
	firstPath := filepath.Join(dir, "events-2026-11-02.jsonl.gz")
	if err := writeArchive(firstPath, []Event{*first}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, archiveZone)
	archived, err := s.ArchiveClosedDays(ctx, now)
	if err == nil || archived != 1 {
		t.Fatalf("迟到日应报错保留,后续闭日仍应归档: archived=%d err=%v", archived, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "events-2026-11-03.jsonl.gz")); err != nil {
		t.Fatalf("后续闭日归档被阻断: %v", err)
	}
	if !eventExists(t, s, late.UID) || eventExists(t, s, second.UID) {
		t.Fatal("迟到事件必须保留且后续已归档事件删除")
	}
}

// 含非法 UTF-8 原文的事件:写入即消毒(H3,PG TEXT 拒收原文),归档往返保真的
// 是「库内存量形态」(消毒形态)——归档/恢复不二次变形。
func TestArchivePreservesSanitizedPayload(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "archive")
	if err := s.ConfigureArchive(dir, 1); err != nil {
		t.Fatal(err)
	}
	ev := testEvent("hcf:archive-invalid-utf8")
	ev.Payload = string([]byte{0xff, 0xfe, 0x00, 'x'})
	ev.ReceivedAt = time.Date(2026, 11, 3, 8, 0, 0, 0, time.FixedZone("CST", 8*3600))
	_, _ = s.InsertEvent(ctx, ev)
	stored, err := s.GetEvent(ctx, ev.UID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Payload != sanitizeText(ev.Payload) {
		t.Fatalf("入库应为消毒形态: got=%v want=%v", []byte(stored.Payload), []byte(sanitizeText(ev.Payload)))
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if _, err := s.ArchiveClosedDays(ctx, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events-2026-11-03.jsonl.gz")
	if _, err := s.RestoreArchive(ctx, path); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEvent(ctx, ev.UID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Payload != stored.Payload {
		t.Fatalf("归档恢复后存量形态未保真: got=%v want=%v", []byte(got.Payload), []byte(stored.Payload))
	}
}

// 存储指标输出(指标名与原系统一致,供双跑对比)。
func TestMetricsExposeStorageAndSpoolGauges(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, _ = s.InsertEvent(ctx, testEvent("hcf:metrics"))
	attachTestSpool(t, s, filepath.Join(t.TempDir(), "spool"), 4096, 4096)
	if err := s.spool.append(testEvent("hcf:pending"), nil); err != nil {
		t.Fatal(err)
	}
	s.spoolWriteFailed.Add(2)
	s.auditWriteFailed.Add(3)
	if err := s.ConfigureArchive(filepath.Join(t.TempDir(), "archive"), 90); err != nil {
		t.Fatal(err)
	}
	text, err := s.MetricsText(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"ceoboard_spool_write_failed_total 2",
		"ceoboard_spool_pending_bytes",
		"ceoboard_db_size_bytes",
		"ceoboard_wal_size_bytes",
		"ceoboard_events_rows 1",
		"ceoboard_archive_lag_days",
		"ceoboard_db_capacity_used_ratio",
		"ceoboard_audit_write_failed_total 3",
	} {
		if !strings.Contains(text, name) {
			t.Errorf("指标缺少 %q:\n%s", name, text)
		}
	}
}
