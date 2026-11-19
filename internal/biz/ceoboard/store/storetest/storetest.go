// Package storetest 提供 ceoboard 存储的测试基建:内存/文件 SQLite(modernc 纯 Go 驱动,
// 无 cgo,F 盘 PG 环境窗口外可跑)+ 收窄到 ceoboard_* 五表的 Ent 迁移钩子。
// 供 store 包自身测试与 adapter 等下游包测试共用(单一事实源,防钩子漂移)。
// 仅供测试导入;Ent 方言显式 SQLite,时间一律 UTC(见 store 包注释)。
//
// 注意:本包不 import store(会形成测试导入环);调用方用返回的 client 自行
// store.New(client, rawDB)。rawDB 仅 PG 容量指标用,内存库传 nil、文件库可传 db。
package storetest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"testing"

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

// ceoboardDeadLetterIdemIndexDDL 与生产迁移
// sql/migrations/20261310_ceoboard_dead_letters_unreplayed_ref_uidx.sql 同语法
// (评审 2026-11-09 M2:同 (kind,source,ref_key) 未重放死信库层唯一兜底)。
// 部分索引无法经 ent schema 表达(ent 迁移会按 model 收敛),故在 Schema.Create
// 后裸 DDL 补建;语法双方言(pg/sqlite)一致。store 测试拿不到该索引时
// AddDeadLetter 的约束兜底分支不可触发——漂移即红。
const ceoboardDeadLetterIdemIndexDDL = `CREATE UNIQUE INDEX IF NOT EXISTS ceoboard_dead_letters_unreplayed_ref_uidx
  ON ceoboard_dead_letters ("kind", "source", "ref_key")
  WHERE "ref_key" <> '' AND "replayed_at" IS NULL`

// newEntOnDB 在给定 *sql.DB 上建 Ent 客户端并迁移 ceoboard schema。
func newEntOnDB(t testing.TB, db *sql.DB) *ent.Client {
	t.Helper()
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := client.Schema.Create(context.Background(), sqlschema.WithHooks(ceoboardOnly)); err != nil {
		db.Close()
		t.Fatalf("建测试 schema: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), ceoboardDeadLetterIdemIndexDDL); err != nil {
		client.Close()
		db.Close()
		t.Fatalf("建死信幂等部分唯一索引(对齐迁移 20261310): %v", err)
	}
	return client
}

// Open 打开独立内存库(每测试唯一命名,shared-cache 不串测试),注册 Cleanup 关闭。
func Open(t testing.TB) (*ent.Client, *sql.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
		url.PathEscape(t.Name()))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := newEntOnDB(t, db)
	t.Cleanup(func() { client.Close(); db.Close() })
	return client, db
}

// OpenFile 打开文件库(WAL;并发/锁行为与"重启"测试用)。
// 注意:不注册 Cleanup——"重启"与"宕机"用例需要先关再开;调用方自行关闭(client 先行)。
func OpenFile(t testing.TB, path string) (*ent.Client, *sql.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := newEntOnDB(t, db)
	return client, db
}
