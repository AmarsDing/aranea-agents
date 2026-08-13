package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// createPluginRunTable 按生产 DDL（sql/plugin_run.sql）建基表。
func createPluginRunTable(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	for _, stmt := range splitDDLStatements(pluginRunDDL) {
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("create plugin_runs: %v\n---\n%s", err, stmt)
		}
	}
}

// applyPluginRunWorkspaceMigration 应用 N-B5 workspace 迁移文件。
func applyPluginRunWorkspaceMigration(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if err := executeSQLFileWithDialect(ctx, db, "sql/migrations/20261211_plugin_runs_workspace.sql", DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("apply plugin_runs workspace migration: %v", err)
	}
}

func newPluginRunTestRepo(t *testing.T) biz.PluginRunRepo {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	createPluginRunTable(t, ctx, db)
	applyPluginRunWorkspaceMigration(t, ctx, db)
	return NewPluginRunRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres})
}

// N-B5：迁移文件必须幂等，重跑不报错且列可用。
func TestPluginRunWorkspaceMigration_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	createPluginRunTable(t, ctx, db)

	applyPluginRunWorkspaceMigration(t, ctx, db)
	applyPluginRunWorkspaceMigration(t, ctx, db) // 重跑必须幂等

	if _, err := db.ExecContext(ctx, `INSERT INTO plugin_runs (id, plugin_key, workspace_id) VALUES ('m1', 'k', 'ws-a')`); err != nil {
		t.Fatalf("insert with workspace_id after double migration: %v", err)
	}
}

// N-B5：Insert 写入 workspace_id；List 按租户可见性过滤
// （租户 = 共享行 ”+ 自身行；系统空调用 = 全部）。
func TestPluginRunRepo_WorkspaceIsolation(t *testing.T) {
	ctx := context.Background()
	repo := newPluginRunTestRepo(t)

	seed := []biz.PluginRun{
		{ID: "r-a", PluginKey: "audit_log", Status: "success", WorkspaceID: "ws-a"},
		{ID: "r-b", PluginKey: "audit_log", Status: "blocked", WorkspaceID: "ws-b"},
		{ID: "r-shared", PluginKey: "audit_log", Status: "success", WorkspaceID: ""},
	}
	for _, r := range seed {
		if err := repo.Insert(ctx, r); err != nil {
			t.Fatalf("insert %s: %v", r.ID, err)
		}
	}

	// 租户 A：可见自身 + 共享行，不可见租户 B。
	res, err := repo.List(ctx, biz.PluginRunQuery{WorkspaceID: "ws-a"})
	if err != nil {
		t.Fatalf("list ws-a: %v", err)
	}
	got := map[string]string{}
	for _, it := range res.Items {
		got[it.ID] = it.WorkspaceID
	}
	if res.Total != 2 || got["r-a"] != "ws-a" || got["r-shared"] != "" {
		t.Errorf("ws-a visibility = %v (total %d), want r-a + r-shared", got, res.Total)
	}
	if _, ok := got["r-b"]; ok {
		t.Errorf("ws-a must not see ws-b run")
	}

	// 系统调用（空 WorkspaceID）：看全部。
	res, err = repo.List(ctx, biz.PluginRunQuery{})
	if err != nil {
		t.Fatalf("list system: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("system total = %d, want 3", res.Total)
	}
}

// N-B5：DeleteAll 按同一可见性语义过滤——租户只删共享行 + 自身行，系统调用全删。
func TestPluginRunRepo_DeleteAllWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	repo := newPluginRunTestRepo(t)

	seed := []biz.PluginRun{
		{ID: "r-a", PluginKey: "audit_log", Status: "success", WorkspaceID: "ws-a"},
		{ID: "r-b", PluginKey: "audit_log", Status: "blocked", WorkspaceID: "ws-b"},
		{ID: "r-shared", PluginKey: "audit_log", Status: "success", WorkspaceID: ""},
	}
	for _, r := range seed {
		if err := repo.Insert(ctx, r); err != nil {
			t.Fatalf("insert %s: %v", r.ID, err)
		}
	}

	// 租户 A：删共享 + 自身，保留 ws-b。
	n, err := repo.DeleteAll(ctx, "ws-a")
	if err != nil {
		t.Fatalf("delete ws-a: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted = %d, want 2 (shared + own)", n)
	}
	res, err := repo.List(ctx, biz.PluginRunQuery{})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if res.Total != 1 || len(res.Items) != 1 || res.Items[0].ID != "r-b" {
		t.Errorf("remaining = %+v, want only r-b", res.Items)
	}

	// 系统调用：全删。
	n, err = repo.DeleteAll(ctx, "")
	if err != nil {
		t.Fatalf("delete system: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
}
