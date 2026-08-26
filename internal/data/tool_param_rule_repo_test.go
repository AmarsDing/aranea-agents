package data

import (
	"context"
	"testing"

	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// newToolParamRuleTestRepo builds the param-rule repo over an isolated PG
// schema with the real 20261262 migration DDL applied.
func newToolParamRuleTestRepo(t *testing.T) biztool.ToolParamRuleStore {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()
	if err := executeSQLFileWithDialect(ctx, db, "sql/migrations/20261262_tool_param_rules.sql", DialectPostgres, lg); err != nil {
		t.Fatalf("migrate 20261262: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: lg, dialect: DialectPostgres}
	return NewToolParamRuleRepo(d, lg)
}

// TestToolParamRuleRepo_Roundtrip covers upsert → list(enabled/all) → update
// in place → delete, including evaluation ordering (priority asc, id asc).
func TestToolParamRuleRepo_Roundtrip(t *testing.T) {
	repo := newToolParamRuleTestRepo(t)
	ctx := context.Background()

	rules := []biztool.ToolParamRule{
		{ID: "builtin-gns3-fallback-ask", ToolKey: "gns3_exec", Pattern: "*", Effect: "ask", Priority: 900, Enabled: true, CreatedAt: 1},
		{ID: "builtin-gns3-allow-show", ToolKey: "gns3_exec", Pattern: "show *", Effect: "allow", Priority: 10, Enabled: true, CreatedAt: 1},
		{ID: "custom-gns3-deny-reload", ToolKey: "gns3_exec", Pattern: "reload*", Effect: "deny", Priority: 5, Enabled: false, CreatedAt: 1},
		{ID: "other-tool", ToolKey: "exec_command", Pattern: "rm -rf*", Effect: "deny", Priority: 1, Enabled: true, CreatedAt: 1},
	}
	for _, r := range rules {
		if err := repo.UpsertParamRule(ctx, r); err != nil {
			t.Fatalf("upsert %s: %v", r.ID, err)
		}
	}

	// 全部规则（含 disabled），priority 升序。
	all, err := repo.ListParamRules(ctx, "gns3_exec")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 || all[0].ID != "custom-gns3-deny-reload" || all[1].ID != "builtin-gns3-allow-show" || all[2].ID != "builtin-gns3-fallback-ask" {
		t.Fatalf("list all order = %+v", all)
	}

	// 仅启用行；其他工具的规则不混入。
	enabled, err := repo.ListEnabledParamRules(ctx, "gns3_exec")
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 2 || enabled[0].ID != "builtin-gns3-allow-show" || enabled[1].ID != "builtin-gns3-fallback-ask" {
		t.Fatalf("list enabled = %+v", enabled)
	}

	// 同 ID upsert = 就地更新（不新增行）。
	if err := repo.UpsertParamRule(ctx, biztool.ToolParamRule{
		ID: "custom-gns3-deny-reload", ToolKey: "gns3_exec", Pattern: "reload*", Effect: "deny", Priority: 5, Enabled: true, CreatedAt: 2,
	}); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	enabled, err = repo.ListEnabledParamRules(ctx, "gns3_exec")
	if err != nil {
		t.Fatalf("list enabled after enable: %v", err)
	}
	if len(enabled) != 3 {
		t.Fatalf("enabled after enable = %+v, want 3 rows", enabled)
	}

	// 删除幂等。
	if err := repo.DeleteParamRule(ctx, "custom-gns3-deny-reload"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.DeleteParamRule(ctx, "custom-gns3-deny-reload"); err != nil {
		t.Fatalf("delete again (idempotent): %v", err)
	}
	all, err = repo.ListParamRules(ctx, "gns3_exec")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all after delete = %+v, want 2 rows", all)
	}
}
