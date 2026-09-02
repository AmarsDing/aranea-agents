package data

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

// TestMigrationVersionsGloballyUnique guards the schema_migrations version
// namespace: DDL migrations (ddlMigrations), data migrations (Migration*
// constants) and seed versions (Seed* constants) share one table whose
// version is the PRIMARY KEY — whichever runs first claims the version and
// the others are silently skipped forever. 2026-07-30 audit found five such
// collisions in production (memory_episodes missing its partial unique
// indexes, cascade_saga_steps stuck with INTEGER id, audit action normalize
// never applied, etc.), all fixed by renumbering the loser.
//
// When adding a new Migration*/Seed* constant, add it to the list below.
func TestMigrationVersionsGloballyUnique(t *testing.T) {
	type owner struct {
		kind, name string
	}
	owners := map[int][]owner{}
	add := func(kind, name string, version int) {
		owners[version] = append(owners[version], owner{kind, name})
	}

	for _, m := range ddlMigrations {
		add("ddl", m.Name, m.Version)
	}
	for _, dm := range []struct {
		name    string
		version int
	}{
		{"legacy_trpc_memory_facts", MigrationLegacyTRPCMemoryFacts},
		{"turn_index_to_turn_id", MigrationTurnIndexToTurnID},
		{"session_status_active_to_idle", MigrationSessionStatusIdle},
		{"session_turn_number_backfill", MigrationSessionTurnNumberBackfill},
		{"session_turn_number_rebackfill", MigrationSessionTurnNumberRebackfill},
		{"team_copy_ownership_to_user", MigrationTeamCopyOwnership},
		{"audit_action_verb_first_normalize", MigrationAuditActionNormalize},
		{"monitor_trace_interrupted_backfill", MigrationMonitorTraceInterruptedBackfill},
		{"avatar_image_repair", MigrationAvatarImageRepair},
		{"organization_redesign", MigrationOrganizationRedesign},
	} {
		add("data", dm.name, dm.version)
	}
	for _, sv := range []struct {
		name    string
		version int
	}{
		{"SeedPackBuiltinV1", SeedPackBuiltinV1},
		{"SeedPackBuiltinV2", SeedPackBuiltinV2},
		{"SeedCleanupNonSystemV1", SeedCleanupNonSystemV1},
		{"SeedPackAgencyV1", SeedPackAgencyV1},
		{"SeedPackItOpsV1", SeedPackItOpsV1},
		{"SeedPackTwinMonitorV1", SeedPackTwinMonitorV1},
	} {
		add("seed", sv.name, sv.version)
	}

	for version, list := range owners {
		if len(list) > 1 {
			t.Errorf("schema_migrations version %d claimed by %d migrations: %v", version, len(list), list)
		}
	}
}

// TestMigrationRegistryCoversSQLFiles 钉死「文件在、注册丢」事故类（已三次：
// project memory 两次 + 20261248/20261261 在 48ddb3001 被误删，2026-08-27 二轮
// 审查 H1 恢复）。双向对账：
//   - 正向：sql/migrations 目录下每个 NNNN_name.sql 的版本前缀必须有注册条目
//     （Func 型迁移内部经 executeSQLFileWithDialect 消费的额外文件如
//     20261252_decision_records_gin.sql，其版本前缀同样有 Func 型条目覆盖）；
//   - 反向：SQL 型注册条目指向的文件必须存在于 embed FS，且路径符合
//     sql/migrations/{version}_{name}.sql 规范（防指错文件/文件名漂移）。
func TestMigrationRegistryCoversSQLFiles(t *testing.T) {
	registered := map[int]string{}
	for _, m := range ddlMigrations {
		registered[m.Version] = m.Name
	}

	entries, err := fs.ReadDir(migrationSQLFS, "sql/migrations")
	if err != nil {
		t.Fatalf("ReadDir sql/migrations: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		v, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			t.Errorf("migration file %q does not start with a numeric version", name)
			continue
		}
		if _, ok := registered[v]; !ok {
			t.Errorf("sql/migrations/%s exists but version %d is not registered in ddlMigrations (file-in-registry-lost accident class)", name, v)
		}
	}

	for _, m := range ddlMigrations {
		if m.SQL == "" {
			continue
		}
		if _, err := migrationSQLFS.Open(m.SQL); err != nil {
			t.Errorf("ddlMigrations entry %d (%s) points to missing file %q", m.Version, m.Name, m.SQL)
			continue
		}
		want := fmt.Sprintf("sql/migrations/%d_%s.sql", m.Version, m.Name)
		if m.SQL == want {
			continue
		}
		// 路径偏离规范必须显式登记例外（含理由），防止新的无意漂移混入：
		// 例外表即文档。历史例外：
		//   20261014 media_providers：release/installer-0.1.35 上原为 20261008，
		//     dev 撞号重编号后保留已发布文件名（registry 注释明载）；
		//   20261210 plugin_cost_guard_usage_schema：文件名少 _schema 后缀，
		//     版本前缀一致，仅命名风格差异。
		legacyExceptions := map[int]string{
			20261014: "sql/migrations/20261008_media_providers.sql",
			20261210: "sql/migrations/20261210_plugin_cost_guard_usage.sql",
		}
		if legacyExceptions[m.Version] != m.SQL {
			t.Errorf("ddlMigrations entry %d (%s) SQL path %q does not match canonical %q (or register a documented legacy exception in this test)", m.Version, m.Name, m.SQL, want)
		}
	}
}
