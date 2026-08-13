package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// I-2 / GAP-01：plugin_cost_guard_usage 表必须经 DDL 迁移创建，
// 且 Repo AddTokens/GetTokens 往返正确（ON CONFLICT 累计 + scope 隔离）。
func TestPluginCostGuardUsage_MigrationAndRepoRoundtrip(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	d := &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}

	// 与生产启动同路径执行迁移 SQL。
	if err := executeSQLFileWithDialect(ctx, db, "sql/migrations/20261210_plugin_cost_guard_usage.sql", DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("execute migration: %v", err)
	}
	// 幂等：重跑不报错。
	if err := executeSQLFileWithDialect(ctx, db, "sql/migrations/20261210_plugin_cost_guard_usage.sql", DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("re-execute migration not idempotent: %v", err)
	}

	repo := NewPluginCostGuardUsageRepo(d)
	if got, err := repo.GetTokens(ctx, "2026-08-13", "global"); err != nil || got != 0 {
		t.Fatalf("empty GetTokens = %d, %v; want 0, nil", got, err)
	}
	if err := repo.AddTokens(ctx, "2026-08-13", "global", 100); err != nil {
		t.Fatalf("AddTokens: %v", err)
	}
	if err := repo.AddTokens(ctx, "2026-08-13", "global", 50); err != nil {
		t.Fatalf("AddTokens second: %v", err)
	}
	if got, err := repo.GetTokens(ctx, "2026-08-13", "global"); err != nil || got != 150 {
		t.Fatalf("GetTokens = %d, %v; want 150, nil", got, err)
	}
	// scope 隔离：其他 scope 不受影响。
	if got, err := repo.GetTokens(ctx, "2026-08-13", "agent-1"); err != nil || got != 0 {
		t.Fatalf("other scope GetTokens = %d, %v; want 0, nil", got, err)
	}
	// 空 scope 归一为 global。
	if err := repo.AddTokens(ctx, "2026-08-13", "", 10); err != nil {
		t.Fatalf("AddTokens empty scope: %v", err)
	}
	if got, err := repo.GetTokens(ctx, "2026-08-13", "global"); err != nil || got != 160 {
		t.Fatalf("GetTokens after empty-scope add = %d, %v; want 160, nil", got, err)
	}
}
