// 测试辅助:内存/文件 SQLite 基建已收敛到 storetest 子包(与 adapter 等下游包
// 测试共用,防迁移钩子漂移);本文件只留 store 包内部断言/构造辅助。
package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"aranea-agents/internal/biz/ceoboard/store/storetest"
	"aranea-agents/internal/data/ent"
)

// openTest 打开独立内存库(每测试唯一命名,shared-cache 不串测试)。
func openTest(t *testing.T) *Store {
	t.Helper()
	client, _ := storetest.Open(t)
	return New(client, nil)
}

// openTestWithDB 同 openTest,但返回原始 *sql.DB(模拟"DB 宕机"用)。
func openTestWithDB(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	client, db := storetest.Open(t)
	return New(client, nil), db
}

// openFileStore 打开文件库(WAL;并发/锁行为测试用),返回原始 *sql.DB 供
// 模拟"DB 宕机"(Close)或外部写锁。Ent 客户端经 s.client 由调用方关闭。
func openFileStore(t *testing.T, path string) (*Store, *sql.DB) {
	t.Helper()
	client, db := storetest.OpenFile(t, path)
	return New(client, db), db
}

// eventExists 报告事件是否存在(GetEvent NotFound → false)。
func eventExists(t *testing.T, s *Store, uid string) bool {
	t.Helper()
	_, err := s.GetEvent(context.Background(), uid)
	if err == nil {
		return true
	}
	if ent.IsNotFound(err) {
		return false
	}
	t.Fatalf("GetEvent(%s): %v", uid, err)
	return false
}

// attachTestSpool 直接挂一个自定义参数的 spool(等价原测试同名辅助)。
func attachTestSpool(t *testing.T, s *Store, dir string, segment, total int64) *spool {
	t.Helper()
	sp, err := newSpool(dir, segment, total)
	if err != nil {
		t.Fatal(err)
	}
	s.spool = sp
	return sp
}

// testEvent 构造一条标准测试事件。
func testEvent(uid string) *Event {
	return &Event{
		UID: uid, Platform: "hcf", Subject: "trade.order.v1", SubjectKey: "trade.order",
		EventID: uid, ReceivedAt: time.Now().UTC(), Payload: `{"event_id":"` + uid + `"}`,
	}
}

// spoolFiles 列出 spool 目录下的 pending 段。
func spoolFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}
