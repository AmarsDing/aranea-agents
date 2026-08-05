package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/schemamigration"
	"aranea-agents/pkg/loggateway"
)

const (
	MigrationLegacyTRPCMemoryFacts           = 20260524
	migrationNameLegacyTRPCMemoryFacts       = "legacy_trpc_memory_facts"
	MigrationTurnIndexToTurnID               = 20260528
	migrationNameTurnIndexToTurnID           = "turn_index_to_turn_id"
	MigrationSessionStatusIdle               = 20260531
	migrationNameSessionStatusIdle           = "session_status_active_to_idle"
	MigrationSessionTurnNumberBackfill       = 20260802
	migrationNameSessionTurnNumberBackfill   = "session_turn_number_backfill"
	MigrationSessionTurnNumberRebackfill     = 20260803
	migrationNameSessionTurnNumberRebackfill = "session_turn_number_rebackfill"
	// 版本取 20261124：原 20260624 被 DDL 迁移 message_fts_schema 抢先占用，
	// 重编号 20261121 又与 DDL 迁移 si_risk_rule_columns 碰撞（20261122/20261123
	// 已被 audit_action_verb_first_normalize/organization_redesign 占用）。
	// 本迁移在任何环境都从未执行（生产待回填 0 行，重编号仅为注册表全局唯一）。
	MigrationTeamCopyOwnership     = 20261124
	migrationNameTeamCopyOwnership = "team_copy_ownership_to_user"
	// 版本取 20261122：原 20260729 被数据迁移 avatar_image_repair 抢先占用，
	// 导致审计 action 规范化从未执行（生产 audit_logs 仍有旧格式 action）。
	MigrationAuditActionNormalize     = 20261122
	migrationNameAuditActionNormalize = "audit_action_verb_first_normalize"
	// schema_migrations 表由 DDL/数据/种子迁移共享，版本必须全局唯一。
	// 唯一性由 TestMigrationVersionsGloballyUnique 守卫（新增迁移前必跑）。
	MigrationMonitorTraceInterruptedBackfill     = 20261115
	migrationNameMonitorTraceInterruptedBackfill = "monitor_trace_interrupted_backfill"
	// 版本取 20261127：20261125/20261126 已被 DDL 迁移 memory_fact_three_counters /
	// memory_profile_cards 占用（FR-12.6/12.7）。
	MigrationL2RecallDefaultOn     = 20261127
	migrationNameL2RecallDefaultOn = "memory_l2_recall_default_on"
)

func isMigrationApplied(ctx context.Context, client *ent.Client, version int, lg loggateway.Logger) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("schema migrations: ent client required")
	}
	return client.SchemaMigration.Query().Where(schemamigration.ID(version)).Exist(ctx)
}

func recordMigrationApplied(ctx context.Context, client *ent.Client, d Dialect, version int, name string, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("schema migrations: ent client required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := client.SchemaMigration.Create().
		SetID(version).
		SetName(name).
		SetAppliedAt(now).
		Save(ctx)
	if err != nil {
		// If the record already exists (duplicate), treat as idempotent success
		if d.AlreadyExistsErr(err) {
			lg.Debug("migration record already exists, skipping",
				loggateway.StepID("data.migration.record"),
				loggateway.Int("version", version))
			return nil
		}
		return fmt.Errorf("record migration %d (%s): %w", version, name, err)
	}
	return nil
}

func IsSeedApplied(ctx context.Context, client *ent.Client, version int, lg loggateway.Logger) (bool, error) {
	return isMigrationApplied(ctx, client, version, lg)
}

func MarkSeedApplied(ctx context.Context, client *ent.Client, d Dialect, version int, name string, lg loggateway.Logger) error {
	return recordMigrationApplied(ctx, client, d, version, name, lg)
}
