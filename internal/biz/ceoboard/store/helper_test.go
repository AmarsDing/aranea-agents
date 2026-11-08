// 测试辅助:enttest 风格的内存/文件 SQLite(modernc 纯 Go 驱动,无 cgo 依赖,
// F 盘 PG 环境窗口外可跑)。Ent 方言显式 SQLite,时间一律 UTC(见 store 包注释)。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	sqlschema "entgo.io/ent/dialect/sql/schema"
	_ "modernc.org/sqlite"

	"aranea-agents/internal/data/ent"
)

// ceoboardTables 是测试迁移范围:仅 ceoboard_* 五表。其余 schema 含 PG 专属
// 索引定义(如 mcp_server 的部分索引 :: 转换),sqlite 方言下无法生成合法 DDL;
// 生产 PG 走全量迁移,不受影响。
var ceoboardTables = map[string]bool{
	"ceoboard_events":             true,
	"ceoboard_webhook_deliveries": true,
	"ceoboard_dead_letters":       true,
	"ceoboard_audit":              true,
	"ceoboard_latency_samples":    true,
}

func ceoboardOnly(next sqlschema.Creator) sqlschema.Creator {
	return sqlschema.CreateFunc(func(ctx context.Context, tables ...*sqlschema.Table) error {
		keep := make([]*sqlschema.Table, 0, len(tables))
		for _, tbl := range tables {
			if ceoboardTables[tbl.Name] {
				keep = append(keep, tbl)
			}
		}
		return next.Create(ctx, keep...)
	})
}

// newEntOnDB 在给定 *sql.DB 上建 Ent 客户端并迁移 ceoboard schema。
func newEntOnDB(t *testing.T, db *sql.DB) *ent.Client {
	t.Helper()
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := client.Schema.Create(context.Background(), sqlschema.WithHooks(ceoboardOnly)); err != nil {
		db.Close()
		t.Fatalf("建测试 schema: %v", err)
	}
	return client
}

// openTest 打开独立内存库(每测试唯一命名,shared-cache 不串测试)。
func openTest(t *testing.T) *Store {
	t.Helper()
	s, _ := openTestWithDB(t)
	return s
}

// openTestWithDB 同 openTest,但返回原始 *sql.DB(模拟"DB 宕机"用)。
func openTestWithDB(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
		url.PathEscape(t.Name()))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := newEntOnDB(t, db)
	t.Cleanup(func() { client.Close(); db.Close() })
	return New(client, nil), db
}

// openFileStore 打开文件库(WAL;并发/锁行为测试用),返回原始 *sql.DB 供
// 模拟"DB 宕机"(Close)或外部写锁。
func openFileStore(t *testing.T, path string) (*Store, *sql.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := newEntOnDB(t, db)
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
