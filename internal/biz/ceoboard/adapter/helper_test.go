// 测试辅助:存储基建走 storetest 子包(与 store 包测试同一迁移钩子,防漂移)。
package adapter

import (
	"database/sql"
	"testing"

	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/biz/ceoboard/store/storetest"
)

// openTestStore 打开独立内存库(每测试唯一命名,shared-cache 不串测试)。
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	client, _ := storetest.Open(t)
	return store.New(client, nil)
}

// openFileStore 打开文件库(WAL;"重启"与故障注入用),返回 Ent 客户端与原始
// *sql.DB(触发器注入/模拟宕机用)。不注册 Cleanup——重启用例需要先关再开。
func openFileStore(t *testing.T, path string) (*store.Store, *sql.DB) {
	t.Helper()
	client, db := storetest.OpenFile(t, path)
	t.Cleanup(func() { client.Close(); db.Close() })
	return store.New(client, db), db
}
