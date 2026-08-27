package data

import (
	"context"
	"strings"
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

// TestToolParamRuleMigrations_SeedAndReseed applies the real 20261262 DDL +
// 20261263 seed + 20261264 reseed to an isolated PG schema and pins:
//  1. seed 落 12 行 builtin：deny 行 pattern 全为正则（re: 前缀）、
//     created_at 落种子发布时点；
//  2. 种子/reseed 重跑幂等（INSERT OR IGNORE / WHERE 旧 pattern 条件）；
//  3. 20261264 把模拟存量的旧 glob 行升级为正则并修 created_at=0，
//     且不回冲用户自改 pattern。
func TestToolParamRuleMigrations_SeedAndReseed(t *testing.T) {
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()
	apply := func(path string) {
		t.Helper()
		if err := executeSQLFileWithDialect(ctx, db, path, DialectPostgres, lg); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}
	apply("sql/migrations/20261262_tool_param_rules.sql")
	apply("sql/migrations/20261263_tool_param_rules_seed.sql")
	apply("sql/migrations/20261264_tool_param_rules_reseed_regex.sql")

	count := func() int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tool_param_rules").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	if n := count(); n != 12 {
		t.Fatalf("seeded rows = %d, want 12", n)
	}
	var denyNonRegex, denyBadTS int
	if err := db.QueryRowContext(ctx, `SELECT
	  COUNT(*) FILTER (WHERE pattern NOT LIKE 're:%'),
	  COUNT(*) FILTER (WHERE created_at <> 1787760000)
	  FROM tool_param_rules WHERE effect = 'deny'`).Scan(&denyNonRegex, &denyBadTS); err != nil {
		t.Fatalf("deny audit: %v", err)
	}
	if denyNonRegex != 0 || denyBadTS != 0 {
		t.Fatalf("deny rows nonRegex=%d badTS=%d, want 0/0", denyNonRegex, denyBadTS)
	}

	// 幂等：种子+reseed 重跑零变化。
	apply("sql/migrations/20261263_tool_param_rules_seed.sql")
	apply("sql/migrations/20261264_tool_param_rules_reseed_regex.sql")
	if n := count(); n != 12 {
		t.Fatalf("rows after rerun = %d, want 12", n)
	}

	// 模拟存量旧 glob 行（created_at=0 占位）→ reseed 升级正则并修时点；
	// 用户自改 pattern 的行不得被回冲。
	if _, err := db.ExecContext(ctx, "UPDATE tool_param_rules SET pattern = 'mkfs*', created_at = 0 WHERE id = 'builtin-exec-deny-mkfs'"); err != nil {
		t.Fatalf("simulate legacy row: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE tool_param_rules SET pattern = 'custom*' WHERE id = 'builtin-exec-deny-halt'"); err != nil {
		t.Fatalf("simulate user-tuned row: %v", err)
	}
	apply("sql/migrations/20261264_tool_param_rules_reseed_regex.sql")

	var mkfsPattern string
	var mkfsTS int64
	if err := db.QueryRowContext(ctx, "SELECT pattern, created_at FROM tool_param_rules WHERE id = 'builtin-exec-deny-mkfs'").Scan(&mkfsPattern, &mkfsTS); err != nil {
		t.Fatalf("read mkfs: %v", err)
	}
	if !strings.HasPrefix(mkfsPattern, "re:") || mkfsTS != 1787760000 {
		t.Fatalf("legacy row not upgraded: pattern=%q created_at=%d", mkfsPattern, mkfsTS)
	}
	var haltPattern string
	if err := db.QueryRowContext(ctx, "SELECT pattern FROM tool_param_rules WHERE id = 'builtin-exec-deny-halt'").Scan(&haltPattern); err != nil {
		t.Fatalf("read halt: %v", err)
	}
	if haltPattern != "custom*" {
		t.Fatalf("user-tuned pattern clobbered: %q", haltPattern)
	}
	// reseed 对已升级行幂等（再跑不变）。
	apply("sql/migrations/20261264_tool_param_rules_reseed_regex.sql")
	var again string
	if err := db.QueryRowContext(ctx, "SELECT pattern FROM tool_param_rules WHERE id = 'builtin-exec-deny-mkfs'").Scan(&again); err != nil {
		t.Fatalf("read mkfs again: %v", err)
	}
	if again != mkfsPattern {
		t.Fatalf("reseed not idempotent: %q -> %q", mkfsPattern, again)
	}
}
