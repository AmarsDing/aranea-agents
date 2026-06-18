package data

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

//go:embed sql/migrations/*.sql
var migrationSQLFS embed.FS

type ddlMigration struct {
	Version int
	Name    string
	// SQL is an optional path to a SQL file to execute (relative to project root).
	// If set, the SQL file is executed first via rawDB, then Func is called (if not nil).
	SQL  string
	Func func(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error
}

var ddlMigrations = []ddlMigration{
	{Version: 20260601, Name: "session_memory_patches", Func: ddlSessionMemoryPatches},
	{Version: 20260604, Name: "session_memory_schema", Func: ddlSessionMemorySchema},
	// 20260602 memory_facts_index_status: columns already included in memory_chain.sql CREATE TABLE (20260604).
	{Version: 20260602, Name: "memory_facts_index_status"},
	{Version: 20260603, Name: "messages_turn_number", Func: ddlMessagesTurnNumber},
	// 20260605 memory_relation_patches: valid_from/valid_to already included in memory_chain.sql CREATE TABLE (20260604).
	{Version: 20260605, Name: "memory_relation_patches"},
	{Version: 20260606, Name: "monitor_schema_patches", Func: ddlMonitorSchemaPatches},
	{Version: 20260607, Name: "agent_runtime_patches", SQL: "sql/migrations/20260607_agent_runtime_patches.sql"},
	{Version: 20260608, Name: "entity_reinforcements_schema", SQL: "sql/migrations/20260608_entity_reinforcements_schema.sql"},
	{Version: 20260609, Name: "cascade_saga_patches", SQL: "sql/migrations/20260609_cascade_saga_patches.sql"},
	{Version: 20260610, Name: "builtin_platform_tools", Func: ddlBuiltinPlatformTools},
	{Version: 20260611, Name: "system_setting_patches", SQL: "sql/migrations/20260611_system_setting_patches.sql"},
	{Version: 20260612, Name: "pricing_rule_patches", SQL: "sql/migrations/20260612_pricing_rule_patches.sql"},
	{Version: 20260613, Name: "llm_provider_model_capability", SQL: "sql/migrations/20260613_llm_provider_model_capability.sql"},
	{Version: 20260614, Name: "default_system_setting", Func: ddlDefaultSystemSetting},
	{Version: 20260615, Name: "credential_encryption_key", Func: ddlCredentialEncryptionKey},
	{Version: 20260616, Name: "eval_schema", Func: ddlEvalSchema},
	{Version: 20260617, Name: "a2a_schema", Func: ddlA2ASchema},
	{Version: 20260618, Name: "a2a_remote_health_patches", SQL: "sql/migrations/20260618_a2a_remote_health_patches.sql"},
	{Version: 20260619, Name: "team_run_summary_patches", SQL: "sql/migrations/20260619_team_run_summary_patches.sql"},
	{Version: 20260620, Name: "session_revision_patches", SQL: "sql/migrations/20260620_session_revision_patches.sql", Func: ddlSessionRevisionDataMigration},
	{Version: 20260621, Name: "plugin_run_schema", Func: ddlPluginRunSchema},
	{Version: 20260622, Name: "hook_delivery_schema", SQL: "sql/migrations/20260622_hook_delivery_schema.sql"},
	{Version: 20260623, Name: "flow_log_schema", Func: ddlFlowLogSchema},
	{Version: 20260624, Name: "message_fts_schema", Func: ddlMessageFTSSchema},
	{Version: 20260625, Name: "channel_inbound_schema", Func: ddlChannelInboundSchema},
	{Version: 20260626, Name: "channel_turn_job_schema", Func: ddlChannelTurnJobSchema},
	{Version: 20260627, Name: "channel_runtime_lease_schema", Func: ddlChannelRuntimeLeaseSchema},
	{Version: 20260628, Name: "session_run_schema", Func: ddlSessionRunSchema},
	{Version: 20260629, Name: "session_participant_schema", Func: ddlSessionParticipantSchema},
	{Version: 20260630, Name: "session_run_checkpoint_schema", SQL: "sql/migrations/20260630_session_run_checkpoint_schema.sql"},
	// 20260701 session_run_column_patches: checkpoint_id/agent_id/resume_started_at already in session_run_schema.go CREATE TABLE (20260628).
	{Version: 20260701, Name: "session_run_column_patches"},
	{Version: 20260702, Name: "monitor_alert_schema", Func: ddlMonitorAlertSchema},
	{Version: 20260703, Name: "ecosystem_schema", Func: ddlEcosystemSchema},
	{Version: 20260704, Name: "team_graph_session_schema", Func: ddlTeamGraphSessionSchema},
	{Version: 20260705, Name: "compiled_team_schema", Func: ddlCompiledTeamSchema},
	{Version: 20260706, Name: "skill_evolution_schema", Func: ddlSkillEvolutionSchema},
	{Version: 20260707, Name: "memory_facts_extra_patches", SQL: "sql/migrations/20260707_memory_facts_extra_patches.sql"},
	{Version: 20260708, Name: "session_table_split", SQL: "sql/migrations/20260708_session_table_split.sql"},
	{Version: 20260709, Name: "vector_embedding_ref", SQL: "sql/migrations/20260709_vector_embedding_ref.sql"},
	{Version: 20260710, Name: "task_plan_schema", Func: ddlTaskPlanSchema},
	{Version: 20260711, Name: "allocation_plan_schema", Func: ddlAllocationPlanSchema},
	{Version: 20260712, Name: "agent_performance_schema", Func: ddlAgentPerformanceSchema},
	{Version: 20260713, Name: "orchestration_schema", Func: ddlOrchestrationSchema},
	// 20260714 compiled_team_session_id: session_id + index already in compiled_team_schema.go CREATE TABLE (20260705).
	{Version: 20260714, Name: "compiled_team_session_id"},
	{Version: 20260715, Name: "self_check_report_schema", SQL: "sql/migrations/20260715_self_check_report_schema.sql"},
	{Version: 20260716, Name: "missing_indexes", SQL: "sql/migrations/20260716_missing_indexes.sql"},
	{Version: 20260717, Name: "usage_events_schema", SQL: "sql/migrations/20260717_usage_events_schema.sql"},
	{Version: 20260718, Name: "ecosystem_preset_schema", SQL: "sql/migrations/20260718_ecosystem_preset_schema.sql", Func: ddlEcosystemPresetDataMigration},
	{Version: 20260719, Name: "agent_source_column", SQL: "sql/migrations/20260719_agent_source_column.sql", Func: ddlAgentSourceDataMigration},
	{Version: 20260720, Name: "unified_evolution_schema", Func: ddlUnifiedEvolutionSchema},
	{Version: 20260721, Name: "evolution_suggestion_pre_apply_snapshot", Func: ddlEvolutionSuggestionPreApplySnapshot},
	{Version: 20260722, Name: "activity_schema", SQL: "sql/migrations/20260722_activity_schema.sql"},
	{Version: 20260723, Name: "activity_token_columns", SQL: "sql/migrations/20260723_activity_token_columns.sql"},
	{Version: 20260724, Name: "invariant_constraints", SQL: "sql/migrations/20260724_invariant_constraints.sql"},
	{Version: 20260725, Name: "memory_bitemporal", SQL: "sql/migrations/20260725_memory_bitemporal.sql"},
}

func runDDLMigrations(rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	ctx := context.Background()
	for _, m := range ddlMigrations {
		applied, err := isMigrationApplied(ctx, entClient, m.Version, lg)
		if err != nil {
			lg.Warn("migration check failed, running patch anyway",
				loggateway.StepID("data.ddl_migration.check"),
				loggateway.Int("version", m.Version),
				loggateway.Err(err))
			applied = false
		}
		if applied {
			continue
		}
		// Execute SQL file first if set
		if m.SQL != "" {
			if err := executeSQLFile(ctx, rawDB, m.SQL, lg); err != nil {
				lg.Error("schema step (SQL) failed",
					loggateway.StepID("data.schema."+m.Name),
					loggateway.Int("version", m.Version),
					loggateway.Err(err))
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		}
		// Then execute Func if set
		if m.Func != nil {
			if err := m.Func(ctx, rawDB, entClient, lg); err != nil {
				lg.Error("schema step failed",
					loggateway.StepID("data.schema."+m.Name),
					loggateway.Int("version", m.Version),
					loggateway.Err(err))
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		}
		if err := recordMigrationApplied(ctx, entClient, m.Version, m.Name, lg); err != nil {
			lg.Error("failed to record migration, aborting to prevent re-execution",
				loggateway.StepID("data.ddl_migration.record"),
				loggateway.Int("version", m.Version),
				loggateway.Err(err))
			return fmt.Errorf("record migration %s: %w", m.Name, err)
		}
	}
	return nil
}

// executeSQLFile reads a SQL file (path relative to project root), splits it into
// individual statements using splitDDLStatements, and executes each via rawDB.
// "duplicate column name", "already exists", and "no such table" errors are treated
// as idempotent successes (the table/column will be created by a later migration).
func executeSQLFile(ctx context.Context, rawDB *sql.DB, path string, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	sqlBytes, err := fs.ReadFile(migrationSQLFS, path)
	if err != nil {
		return fmt.Errorf("read SQL file %s: %w", path, err)
	}
	ddl := strings.TrimPrefix(string(sqlBytes), "\ufeff")
	for _, stmt := range splitDDLStatements(strings.TrimSpace(ddl)) {
		if stmt == "" {
			continue
		}
		if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
			if isColumnExistsErr(err) {
				lg.Debug("ddl patch skipped (already exists)",
					loggateway.StepID("data.ddl_migration.sql_file"),
					loggateway.Str("statement", stmt[:min(len(stmt), 120)]))
				continue
			}
			if isNoSuchTableErr(err) {
				lg.Debug("ddl patch skipped (table not yet created, will be created by later migration)",
					loggateway.StepID("data.ddl_migration.sql_file"),
					loggateway.Str("statement", stmt[:min(len(stmt), 120)]))
				continue
			}
			return fmt.Errorf("execute SQL statement in %s: %w\n---\n%s", path, err, stmt)
		}
	}
	lg.Info("executed SQL migration file",
		loggateway.StepID("data.ddl_migration.sql_file"),
		loggateway.Str("path", path))
	return nil
}

func ddlSessionMemoryPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return sessionMemoryEnsurePatches(ctx, entClient)
}

func ddlMessagesTurnNumber(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureMessagesTurnNumberPatch(ctx, entClient, lg)
}

func ddlSessionMemorySchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureSessionMemorySchema(ctx, entClient, lg)
}

func ddlMemoryRelationPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return sessionMemoryEnsureMemoryRelationPatches(ctx, entClient)
}

func ddlMonitorSchemaPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return sessionMemoryEnsureMonitorSchemaPatches(ctx, entClient)
}

func ddlBuiltinPlatformTools(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureBuiltinPlatformTools(ctx, entClient, lg)
}

func ddlDefaultSystemSetting(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureDefaultSystemSetting(ctx, entClient)
}

func ddlCredentialEncryptionKey(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureDefaultCredentialEncryptionKey(ctx, entClient)
}

func ddlEvalSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureEvalSchema(ctx, rawDB)
}

func ddlA2ASchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureA2ASchema(ctx, rawDB)
}

// ddlSessionRevisionDataMigration backfills session_revision from message counts.
// The ALTER TABLE is handled by the SQL file; this Func only does the data migration.
// Uses WHERE session_revision IS NULL OR session_revision = ” to ensure idempotency.
func ddlSessionRevisionDataMigration(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	if entClient == nil {
		return nil
	}
	_, err := entClient.ExecContext(ctx, `
UPDATE sessions SET session_revision = (
  SELECT COUNT(*) FROM messages WHERE messages.session_id = sessions.id AND role = 'user'
) WHERE session_revision IS NULL OR session_revision = ''`)
	return err
}

func ddlPluginRunSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsurePluginRunSchema(ctx, entClient)
}

func ddlFlowLogSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureFlowLogSchema(ctx, entClient)
}

func ddlMessageFTSSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureMessageFTSSchema(ctx, rawDB)
}

func ddlChannelInboundSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureChannelInboundSchema(ctx, rawDB)
}

func ddlChannelTurnJobSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureChannelTurnJobSchema(ctx, rawDB)
}

func ddlChannelRuntimeLeaseSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureChannelRuntimeLeaseSchema(ctx, rawDB)
}

func ddlSessionRunSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureSessionRunSchema(ctx, rawDB, lg)
}

func ddlSessionParticipantSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureSessionParticipantSchema(ctx, rawDB, lg)
}

func ddlMonitorAlertSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureMonitorAlertSchema(ctx, entClient)
}

func ddlEcosystemSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureEcosystemSchema(ctx, entClient)
}

func ddlTeamGraphSessionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureTeamGraphSessionSchema(ctx, rawDB)
}

func ddlCompiledTeamSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureCompiledTeamSchema(ctx, rawDB)
}

func ddlSkillEvolutionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureSkillEvolutionSchema(ctx, entClient)
}

func ddlTaskPlanSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureTaskPlanSchema(ctx, rawDB, lg)
}

func ddlAllocationPlanSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureAllocationPlanSchema(ctx, rawDB, lg)
}

func ddlAgentPerformanceSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureAgentPerformanceSchema(ctx, rawDB, lg)
}

func ddlOrchestrationSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureOrchestrationSchema(ctx, rawDB, lg)
}

func ddlCompiledTeamSessionID(ctx context.Context, rawDB *sql.DB, _ *ent.Client, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	// Add session_id column if it doesn't exist (SQLite ALTER TABLE ADD COLUMN is safe if column exists)
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE compiled_teams ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`); err != nil && !isColumnExistsErr(err) {
		return fmt.Errorf("add compiled_teams.session_id: %w", err)
	}
	if _, err := rawDB.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_compiled_teams_session_id ON compiled_teams(session_id)`); err != nil {
		return fmt.Errorf("create idx_compiled_teams_session_id: %w", err)
	}
	return nil
}

func ddlEcosystemPresetDataMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	// Wrap in transaction for atomicity
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()
	// Migrate agent kind: system -> system_builtin
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET kind = 'system_builtin' WHERE kind = 'system'`); err != nil {
		return fmt.Errorf("migrate agent kind system->system_builtin: %w", err)
	}
	// Migrate agent kind: industry_template -> ecosystem_preset
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET kind = 'ecosystem_preset' WHERE kind = 'industry_template'`); err != nil {
		return fmt.Errorf("migrate agent kind industry_template->ecosystem_preset: %w", err)
	}
	// Migrate team kind: source=imported -> kind=ecosystem_preset
	if _, err := tx.ExecContext(ctx, `UPDATE teams SET kind = 'ecosystem_preset' WHERE source = 'imported'`); err != nil {
		return fmt.Errorf("migrate team kind imported->ecosystem_preset: %w", err)
	}
	return tx.Commit()
}

// ddlAgentSourceDataMigration populates the new agents.source column from existing kind values.
// Mapping: kind=user -> source=user, kind=system_builtin -> source=system, kind=ecosystem_preset/marketplace/certified -> source=imported.
// Also fixes Team source and corrects over-broad Team kind migration from 20260718.
func ddlAgentSourceDataMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	// Wrap in transaction for atomicity
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()
	// Agent source migration
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET source = 'system' WHERE kind = 'system_builtin' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate agent source system_builtin->system: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET source = 'imported' WHERE kind IN ('ecosystem_preset', 'marketplace', 'certified') AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate agent source ecosystem->imported: %w", err)
	}
	// Team source migration: align with Agent source semantics
	// kind=system_builtin -> source=system
	if _, err := tx.ExecContext(ctx, `UPDATE teams SET source = 'system' WHERE kind = 'system_builtin' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate team source system_builtin->system: %w", err)
	}
	// kind=ecosystem_preset -> source=imported
	if _, err := tx.ExecContext(ctx, `UPDATE teams SET source = 'imported' WHERE kind = 'ecosystem_preset' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate team source ecosystem_preset->imported: %w", err)
	}
	// Fix over-broad migration from 20260718: teams with kind='ecosystem_preset' but
	// no ecosystem_preset agent members should be 'user'. The 20260718 migration set
	// kind='ecosystem_preset' for ALL source='imported' teams, including user-imported packs.
	// Heuristic: if a team has NO members referencing ecosystem_preset agents, revert to 'user'.
	if _, err := tx.ExecContext(ctx, `
		UPDATE teams SET kind = 'user', source = 'user'
		WHERE kind = 'ecosystem_preset'
		  AND deleted_at = ''
		  AND id NOT IN (
		    SELECT DISTINCT t2.id
		    FROM teams t2, json_each(t2.definition_json, '$.members') tm
		    INNER JOIN agents a ON a.id = json_extract(tm.value, '$.agent_id') AND a.kind = 'ecosystem_preset' AND a.deleted_at = ''
		    WHERE t2.kind = 'ecosystem_preset' AND t2.deleted_at = ''
		  )
	`); err != nil {
		// Non-critical: log the error but don't fail the entire migration
		lg.Warn("ddl migration: fix over-broad team kind migration failed", loggateway.Err(err))
	}
	return tx.Commit()
}

func ddlUnifiedEvolutionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureUnifiedEvolutionSchema(ctx, entClient)
}

func ddlEvolutionSuggestionPreApplySnapshot(ctx context.Context, rawDB *sql.DB, _ *ent.Client, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE evolution_suggestions ADD COLUMN pre_apply_snapshot TEXT NOT NULL DEFAULT ''`); err != nil && !isColumnExistsErr(err) {
		return fmt.Errorf("add evolution_suggestions.pre_apply_snapshot: %w", err)
	}
	return nil
}
