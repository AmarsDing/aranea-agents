// spool 崩溃恢复与双失败语义测试(自 ceo-dashboard/internal/store/spool_test.go
// 移植,API 加 ctx;"DB 宕机"通过关闭底层 *sql.DB 模拟,断言改走 GetEvent)。
package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpoolAppendSyncRotateAndRecover(t *testing.T) {
	s := openTest(t)
	dir := filepath.Join(t.TempDir(), "spool")
	attachTestSpool(t, s, dir, 600, 16*1024)

	for _, uid := range []string{"hcf:1", "hcf:2", "hcf:3"} {
		if err := s.spool.append(testEvent(uid), errors.New("db down")); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(spoolFiles(t, dir)); got < 2 {
		t.Fatalf("达到单段上限后应轮转,实际 %d 个段", got)
	}

	imported, err := s.ImportSpool(context.Background())
	if err != nil || imported != 3 {
		t.Fatalf("恢复应导入 3 条: imported=%d err=%v", imported, err)
	}
	if got := len(spoolFiles(t, dir)); got != 0 {
		t.Fatalf("完整导入后应删除 pending 段,剩余 %d", got)
	}
}

func TestSpoolRejectsOversizedRecord(t *testing.T) {
	s := openTest(t)
	dir := filepath.Join(t.TempDir(), "spool")
	attachTestSpool(t, s, dir, 1, 4096)
	if err := s.spool.append(testEvent("hcf:oversized"), nil); err == nil {
		t.Fatal("单条记录超过段上限必须返回错误")
	}
	if got := len(spoolFiles(t, dir)); got != 0 {
		t.Fatalf("超限记录不得形成超写段,实际 %d", got)
	}
}

func TestSpoolTotalLimitNeverOverwritesPending(t *testing.T) {
	s := openTest(t)
	dir := filepath.Join(t.TempDir(), "spool")
	attachTestSpool(t, s, dir, 4096, 600)
	if err := s.spool.append(testEvent("hcf:oldest"), errors.New("db down")); err != nil {
		t.Fatal(err)
	}
	files := spoolFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("期望 1 个 pending 段,实际 %d", len(files))
	}
	before, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	sumBefore := sha256.Sum256(before)

	err = s.spool.append(testEvent("hcf:new"), errors.New("db still down"))
	if !errors.Is(err, ErrSpoolFull) {
		t.Fatalf("总容量不足应返回 ErrSpoolFull,实际 %v", err)
	}
	after, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	sumAfter := sha256.Sum256(after)
	if sumBefore != sumAfter {
		t.Fatal("spool 满时最老 pending 段被修改")
	}
}

func TestSpoolTotalLimitIncludesQuarantine(t *testing.T) {
	s := openTest(t)
	dir := filepath.Join(t.TempDir(), "spool")
	attachTestSpool(t, s, dir, 4096, 600)
	if err := os.WriteFile(filepath.Join(dir, "events-old.jsonl.quarantine"), bytes.Repeat([]byte("x"), 200), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.spool.append(testEvent("hcf:capacity"), nil); !errors.Is(err, ErrSpoolFull) {
		t.Fatalf("quarantine 必须计入目录硬上限,实际 %v", err)
	}
	if got := len(spoolFiles(t, dir)); got != 0 {
		t.Fatalf("容量不足不得新增 pending 段,实际 %d", got)
	}
}

func TestSpoolRejectsChecksumDamageAndQuarantinesTornTail(t *testing.T) {
	t.Run("checksum damage", func(t *testing.T) {
		s := openTest(t)
		dir := filepath.Join(t.TempDir(), "spool")
		attachTestSpool(t, s, dir, 4096, 4096)
		if err := s.spool.append(testEvent("hcf:bad"), errors.New("db down")); err != nil {
			t.Fatal(err)
		}
		file := spoolFiles(t, dir)[0]
		raw, _ := os.ReadFile(file)
		damaged := strings.Replace(string(raw), "db down", "tampered", 1)
		if err := os.WriteFile(file, []byte(damaged), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ImportSpool(context.Background()); err == nil {
			t.Fatal("checksum 损坏必须拒绝导入")
		}
		if n, _ := s.TotalEvents(context.Background()); n != 0 {
			t.Fatalf("损坏记录不得入库,实际 %d", n)
		}
		quarantined, _ := filepath.Glob(filepath.Join(dir, "*.quarantine"))
		if len(quarantined) != 1 {
			t.Fatalf("损坏段应隔离,实际 %v", quarantined)
		}
	})

	t.Run("torn tail", func(t *testing.T) {
		s := openTest(t)
		dir := filepath.Join(t.TempDir(), "spool")
		attachTestSpool(t, s, dir, 4096, 4096)
		if err := s.spool.append(testEvent("hcf:good"), errors.New("db down")); err != nil {
			t.Fatal(err)
		}
		file := spoolFiles(t, dir)[0]
		f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString(`{"event":`)
		_ = f.Close()

		imported, err := s.ImportSpool(context.Background())
		if err != nil || imported != 1 {
			t.Fatalf("完整前缀应导入并隔离撕裂尾: imported=%d err=%v", imported, err)
		}
		quarantined, _ := filepath.Glob(filepath.Join(dir, "*.quarantine"))
		if len(quarantined) != 1 {
			t.Fatalf("撕裂尾应保留为 quarantine,实际 %v", quarantined)
		}
	})
}

func TestSpoolAppendAfterTornTailRecoversValidRecords(t *testing.T) {
	s := openTest(t)
	dir := filepath.Join(t.TempDir(), "spool")
	attachTestSpool(t, s, dir, 4096, 16*1024)
	if err := s.spool.append(testEvent("hcf:before-tear"), nil); err != nil {
		t.Fatal(err)
	}
	file := spoolFiles(t, dir)[0]
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"event":`)
	_ = f.Close()
	if err := s.spool.append(testEvent("hcf:after-tear"), nil); err != nil {
		t.Fatal(err)
	}

	imported, err := s.ImportSpool(context.Background())
	if err != nil || imported != 2 {
		t.Fatalf("撕裂尾不得污染后续记录: imported=%d err=%v", imported, err)
	}
	quarantined, _ := filepath.Glob(filepath.Join(dir, "*.quarantine"))
	if len(quarantined) != 1 {
		t.Fatalf("撕裂尾应单独隔离,实际 %v", quarantined)
	}
}

// 完整重投单调补齐历史截断行(payload_complete 0→1,直连与 spool 两条路径)。
func TestFullPayloadRepairsLegacyEvent(t *testing.T) {
	const uid = "hcf:legacy-truncated"
	fullPayload := `{"data":"` + strings.Repeat("x", 16*1024) + `"}`
	ctx := context.Background()

	t.Run("direct retry", func(t *testing.T) {
		s := openTest(t)
		legacy := testEvent(uid)
		legacy.Payload = "truncated"
		if _, err := s.insertEventWithFlags(ctx, legacy, false, false); err != nil {
			t.Fatal(err)
		}
		full := testEvent(uid)
		full.Payload = fullPayload
		result, err := s.IngestEvent(ctx, full, nil)
		if err != nil || !result.Duplicate || result.Inserted {
			t.Fatalf("完整重投仍是同 UID 去重: %+v err=%v", result, err)
		}
		got, err := s.GetEvent(ctx, uid)
		if err != nil {
			t.Fatal(err)
		}
		if got.Payload != fullPayload || !got.PayloadComplete || got.ImportedFromSpool {
			t.Fatalf("直连重投应补齐历史原文: len=%d complete=%v spool=%v", len(got.Payload), got.PayloadComplete, got.ImportedFromSpool)
		}
	})

	t.Run("spool recovery", func(t *testing.T) {
		s := openTest(t)
		legacy := testEvent(uid)
		legacy.Payload = "truncated"
		if _, err := s.insertEventWithFlags(ctx, legacy, false, false); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(t.TempDir(), "spool")
		attachTestSpool(t, s, dir, 64*1024, 128*1024)
		full := testEvent(uid)
		full.Payload = fullPayload
		if err := s.spool.append(full, errors.New("db down")); err != nil {
			t.Fatal(err)
		}
		imported, err := s.ImportSpool(ctx)
		if err != nil || imported != 0 {
			t.Fatalf("补齐已有 UID 不应计作新增: imported=%d err=%v", imported, err)
		}
		got, err := s.GetEvent(ctx, uid)
		if err != nil {
			t.Fatal(err)
		}
		if got.Payload != fullPayload || !got.PayloadComplete || !got.ImportedFromSpool {
			t.Fatalf("spool 恢复应补齐历史原文并保留 spool 标记: len=%d complete=%v spool=%v", len(got.Payload), got.PayloadComplete, got.ImportedFromSpool)
		}
	})
}

// DB 宕机 → 事件转 spool 安全受理;恢复后回填一次(event_uid 幂等),重复导入为空。
func TestIngestEventDBFailureSpoolsThenImportsOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.db")
	s, db := openFileStore(t, path)
	dir := filepath.Join(t.TempDir(), "spool")
	attachTestSpool(t, s, dir, 4096, 4096)
	if err := db.Close(); err != nil { // 模拟 DB 宕机
		t.Fatal(err)
	}

	result, err := s.IngestEvent(ctx, testEvent("hcf:spooled"), errors.New("database closed"))
	if err != nil || !result.Spooled || result.Inserted || result.Duplicate {
		t.Fatalf("DB 失败且 spool 成功应安全受理: %+v err=%v", result, err)
	}

	recovered, rdb := openFileStore(t, path)
	t.Cleanup(func() { recovered.client.Close(); rdb.Close() })
	attachTestSpool(t, recovered, dir, 4096, 4096)
	imported, err := recovered.ImportSpool(ctx)
	if err != nil || imported != 1 {
		t.Fatalf("恢复导入失败: imported=%d err=%v", imported, err)
	}
	got, err := recovered.GetEvent(ctx, "hcf:spooled")
	if err != nil {
		t.Fatal(err)
	}
	if !got.ImportedFromSpool {
		t.Fatal("恢复记录必须标 imported_from_spool")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(got.Payload), &doc); err != nil || doc["event_id"] != "hcf:spooled" {
		t.Fatalf("spool 应保留原始 payload: %q err=%v", got.Payload, err)
	}
	imported, err = recovered.ImportSpool(ctx)
	if err != nil || imported != 0 {
		t.Fatalf("重复导入应为空: imported=%d err=%v", imported, err)
	}
	if n, _ := recovered.TotalEvents(ctx); n != 1 {
		t.Fatalf("event_uid 幂等恢复后应只有 1 行,实际 %d", n)
	}
}

// DB 与 spool 双失败必须返回错误,不得确认受理;spool 写失败计数 +1。
func TestIngestEventDBAndSpoolFailureReturnsError(t *testing.T) {
	s, db := openTestWithDB(t)
	attachTestSpool(t, s, filepath.Join(t.TempDir(), "spool"), 4096, 4096)
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.spool.dir = blocked
	_ = db.Close() // 模拟 DB 宕机

	result, err := s.IngestEvent(context.Background(), testEvent("hcf:lost"), errors.New("database closed"))
	if err == nil {
		t.Fatal("DB 与 spool 双失败必须返回错误,不能确认受理")
	}
	if result.Inserted || result.Duplicate || result.Spooled {
		t.Fatalf("双失败不得报告任何成功状态: %+v", result)
	}
	if got := s.spoolWriteFailed.Load(); got != 1 {
		t.Fatalf("spool 写失败计数应为 1,实际 %d", got)
	}
}

// 非法 UTF-8 原文经 spool 往返保真(base64 扩展编码,风险点⑪)。
func TestSpoolPreservesInvalidUTF8Payload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.db")
	s, db := openFileStore(t, path)
	dir := filepath.Join(t.TempDir(), "spool")
	attachTestSpool(t, s, dir, 4096, 4096)
	ev := testEvent("hcf:invalid-utf8")
	ev.Payload = string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, 0xfe, '"', '}'})
	_ = db.Close() // 模拟 DB 宕机
	if result, err := s.IngestEvent(ctx, ev, nil); err != nil || !result.Spooled {
		t.Fatalf("写入 spool 失败: %+v err=%v", result, err)
	}
	recovered, rdb := openFileStore(t, path)
	t.Cleanup(func() { recovered.client.Close(); rdb.Close() })
	attachTestSpool(t, recovered, dir, 4096, 4096)
	if _, err := recovered.ImportSpool(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := recovered.GetEvent(ctx, ev.UID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Payload != ev.Payload {
		t.Fatalf("非法 UTF-8 原文未保真: got=%v want=%v", []byte(got.Payload), []byte(ev.Payload))
	}
}

// ImportSpool 等待 DB 写锁期间不得阻塞独立的 spool append(claim 只在短临界区改名)。
func TestImportSpoolDoesNotBlockAppendDuringDBWait(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.db")
	s, db := openFileStore(t, path)
	t.Cleanup(func() { s.client.Close(); db.Close() })
	attachTestSpool(t, s, filepath.Join(t.TempDir(), "spool"), 4096, 16*1024)
	if err := s.spool.append(testEvent("hcf:importing"), nil); err != nil {
		t.Fatal(err)
	}
	// 第二条连接持写锁(未提交),模拟长事务阻塞落库
	locker, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close()
	tx, err := locker.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO ceoboard_dead_letters(kind,failed_at) VALUES('lock',datetime('now'))`); err != nil {
		t.Fatal(err)
	}

	importDone := make(chan error, 1)
	go func() {
		_, err := s.ImportSpool(ctx)
		importDone <- err
	}()
	time.Sleep(100 * time.Millisecond)
	appendDone := make(chan error, 1)
	go func() { appendDone <- s.spool.append(testEvent("hcf:during-import"), nil) }()
	blocked := false
	select {
	case err := <-appendDone:
		if err != nil {
			t.Errorf("恢复期间 append 失败: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		blocked = true
	}
	_ = tx.Rollback()
	if blocked {
		select {
		case err := <-appendDone:
			if err != nil {
				t.Errorf("解除 DB 锁后 append 失败: %v", err)
			}
		case <-time.After(6 * time.Second):
			t.Error("解除 DB 锁后 append 仍未返回")
		}
	}
	select {
	case err := <-importDone:
		if err != nil {
			t.Errorf("解除 DB 锁后恢复失败: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Error("解除 DB 锁后恢复仍未返回")
	}
	if blocked {
		t.Fatal("ImportSpool 等待 DB 时阻塞了独立 spool append")
	}
}

func TestSpoolRecordChecksumIsVerifiable(t *testing.T) {
	s := openTest(t)
	dir := filepath.Join(t.TempDir(), "spool")
	attachTestSpool(t, s, dir, 4096, 4096)
	if err := s.spool.append(testEvent("hcf:checksum"), errors.New("db down")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(spoolFiles(t, dir)[0])
	var rec SpoolRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Checksum == "" || rec.Reason != "db down" || rec.SpooledAt.IsZero() {
		t.Fatalf("spool 记录缺原因/时间/checksum: %+v", rec)
	}
	sum := sha256.Sum256([]byte(rec.canonicalWithoutChecksum()))
	if rec.Checksum != hex.EncodeToString(sum[:]) {
		t.Fatal("checksum 未覆盖不含 checksum 的 canonical JSON")
	}
}
