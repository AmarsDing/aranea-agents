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
	Func func(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error
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
	{Version: 20260726, Name: "memory_links", SQL: "sql/migrations/20260726_memory_links.sql"},
	{Version: 20260727, Name: "memory_decay_columns", SQL: "sql/migrations/20260727_memory_decay_columns.sql"},
	{Version: 20260728, Name: "memory_job_deadletter_schema", SQL: "sql/migrations/20260728_memory_job_deadletter_schema.sql"},
	{Version: 20260730, Name: "runtime_profile_schema", SQL: "sql/migrations/20260730_runtime_profile_schema.sql"},
	{Version: 20260731, Name: "heal_record_metadata_column", Func: ddlHealRecordMetadataColumn},
	{Version: 20260801, Name: "memory_job_deadletter_unique", SQL: "sql/migrations/20260801_memory_job_deadletter_unique.sql"},
	{Version: 20260802, Name: "memory_episodes_l1_task_unique", SQL: "sql/migrations/20260802_memory_episodes_l1_task_unique.sql"},
	{Version: 20260803, Name: "cascade_saga_id_type_fix", SQL: "sql/migrations/20260803_cascade_saga_id_type_fix.sql"},
	{Version: 20260804, Name: "planner_model_columns", SQL: "sql/migrations/20260804_planner_model_columns.sql"},
	{Version: 20260825, Name: "activity_session_tree_columns", SQL: "sql/migrations/20260825_activity_session_tree_columns.sql"},
	{Version: 20260901, Name: "drop_event_store_subsystem", SQL: "sql/migrations/20260901_drop_event_store_subsystem.sql"},
	{Version: 20260902, Name: "drop_messages_subsystem", SQL: "sql/migrations/20260902_drop_messages_subsystem.sql"},
	// 20260903 intent_pass_default_on: correct historical false default for non-A2A agents (P1-1).
	// Ent schema default was false (bug); DDL migration 20260607 set column default 1 but Ent always
	// wrote explicit false on create. A2A proxy agents keep false (set explicitly in biz layer).
	{Version: 20260903, Name: "intent_pass_default_on", Func: ddlIntentPassDefaultOnMigration},
	// 20261001 v2_indexes: supplementary single-column indexes for v2 entity tables
	// (LLM Activity Ordering Phase 1). Ent Schema.Create() already creates table columns,
	// primary keys, and composite indexes declared in Indexes(); this migration adds
	// single-column indexes for common query patterns not covered by leftmost-prefix rule.
	{Version: 20261001, Name: "v2_indexes", SQL: "sql/migrations/20261001_v2_indexes.sql"},
	// 20261002 team_stage_team_name: add team_name column to team_stages_v2.
	// Ent Schema.Create() 不会为已存在表新增列，需要 ALTER TABLE 补列。
	{Version: 20261002, Name: "team_stage_team_name", SQL: "sql/migrations/20261002_team_stage_team_name.sql"},
	// 20261003 plan_step_agent_keys: add agent_keys column to plan_steps_v2.
	// PlanStep 携带 LLM 分配的 agent key 列表，RealTeamOrchestrator 优先使用此字段。
	{Version: 20261003, Name: "plan_step_agent_keys", SQL: "sql/migrations/20261003_plan_step_agent_keys.sql"},
}

// RunDDLMigrationsExternal runs DDL migrations with the given dialect.
// This is exported for use by external tools (e.g. cmd/migrate-sqlite-to-postgres)
// to ensure DDL-managed tables (FTS5, monitor, memory, trpc session, etc.) exist
// before data migration. The entClient must be connected to the same database as
// rawDB and must use the same dialect.
func RunDDLMigrationsExternal(rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return runDDLMigrationsWithDialect(rawDB, entClient, d, lg)
}

// runDDLMigrationsWithDialect runs DDL migrations with dialect-aware error handling.
// Use this when the primary database is Postgres to ensure idempotent error detection
// matches the active dialect.
func runDDLMigrationsWithDialect(rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
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
			if err := executeSQLFileWithDialect(ctx, rawDB, m.SQL, d, lg); err != nil {
				lg.Error("schema step (SQL) failed",
					loggateway.StepID("data.schema."+m.Name),
					loggateway.Int("version", m.Version),
					loggateway.Err(err))
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		}
		// Then execute Func if set
		if m.Func != nil {
			if err := m.Func(ctx, rawDB, entClient, d, lg); err != nil {
				lg.Error("schema step failed",
					loggateway.StepID("data.schema."+m.Name),
					loggateway.Int("version", m.Version),
					loggateway.Err(err))
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		}
		if err := recordMigrationApplied(ctx, entClient, d, m.Version, m.Name, lg); err != nil {
			lg.Error("failed to record migration, aborting to prevent re-execution",
				loggateway.StepID("data.ddl_migration.record"),
				loggateway.Int("version", m.Version),
				loggateway.Err(err))
			return fmt.Errorf("record migration %s: %w", m.Name, err)
		}
	}
	return nil
}

// executeSQLFileWithDialect is the dialect-aware SQL file executor.
// Uses Dialect.AlreadyExistsErr and Dialect.UndefinedObjectErr for idempotent
// error detection across SQLite and Postgres.
func executeSQLFileWithDialect(ctx context.Context, rawDB *sql.DB, path string, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return fmt.Errorf("execute SQL file %s: rawDB is nil", path)
	}
	sqlBytes, err := fs.ReadFile(migrationSQLFS, path)
	if err != nil {
		return fmt.Errorf("read SQL file %s: %w", path, err)
	}
	ddl := strings.TrimPrefix(string(sqlBytes), "\ufeff")
	if d.IsPostgres() {
		// Translate SQLite-specific DDL syntax to Postgres equivalents.
		// INTEGER PRIMARY KEY AUTOINCREMENT -> BIGSERIAL PRIMARY KEY
		ddl = translateSQLiteDDLToPostgres(ddl)
	}
	for _, stmt := range splitDDLStatements(strings.TrimSpace(ddl)) {
		if stmt == "" {
			continue
		}
		if d.IsPostgres() {
			stmt = translateSQLiteStatementToPostgres(stmt)
		}
		// Wrap each DDL statement in a transaction. When a statement fails
		// (e.g. "column already exists"), Postgres aborts the current
		// transaction; without an explicit rollback, the connection is
		// returned to the pool in an aborted state, causing subsequent
		// statements to fail with "could not complete operation in a failed
		// transaction". Using a per-statement transaction ensures the
		// connection is always returned clean.
		tx, txErr := rawDB.BeginTx(ctx, nil)
		if txErr != nil {
			return fmt.Errorf("begin tx for SQL statement in %s: %w", path, txErr)
		}
		_, err := tx.ExecContext(ctx, stmt)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				lg.Debug("rollback after exec failure",
					loggateway.StepID("data.ddl_migration.sql_file"),
					loggateway.Err(rbErr))
			}
			if d.AlreadyExistsErr(err) {
				lg.Debug("ddl patch skipped (already exists)",
					loggateway.StepID("data.ddl_migration.sql_file"),
					loggateway.Str("statement", stmt[:min(len(stmt), 120)]))
				continue
			}
			if d.UndefinedObjectErr(err) {
				lg.Debug("ddl patch skipped (table not yet created, will be created by later migration)",
					loggateway.StepID("data.ddl_migration.sql_file"),
					loggateway.Str("statement", stmt[:min(len(stmt), 120)]))
				continue
			}
			return fmt.Errorf("execute SQL statement in %s: %w\n---\n%s", path, err, stmt)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit SQL statement in %s: %w\n---\n%s", path, err, stmt)
		}
	}
	lg.Info("executed SQL migration file",
		loggateway.StepID("data.ddl_migration.sql_file"),
		loggateway.Str("path", path))
	return nil
}

func ddlSessionMemoryPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return sessionMemoryEnsurePatches(ctx, entClient, d)
}

func ddlMessagesTurnNumber(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureMessagesTurnNumberPatch(ctx, entClient, d, lg)
}

func ddlSessionMemorySchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureSessionMemorySchema(ctx, entClient, d, lg)
}

func ddlMonitorSchemaPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return sessionMemoryEnsureMonitorSchemaPatches(ctx, entClient, d)
}

func ddlBuiltinPlatformTools(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureBuiltinPlatformTools(ctx, entClient, d, lg)
}

func ddlDefaultSystemSetting(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureDefaultSystemSetting(ctx, entClient)
}

func ddlCredentialEncryptionKey(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureDefaultCredentialEncryptionKey(ctx, entClient, d)
}

func ddlEvalSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureEvalSchema(ctx, rawDB)
}

func ddlA2ASchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureA2ASchema(ctx, rawDB, d)
}

// ddlSessionRevisionDataMigration backfills session_revision from message counts.
// The ALTER TABLE is handled by the SQL file; this Func only does the data migration.
// Uses dialect-aware WHERE clause: SQLite needs `OR session_revision = ”` for
// type-coerced empty strings; Postgres uses proper bigint type and only needs IS NULL.
func ddlSessionRevisionDataMigration(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	if entClient == nil {
		return nil
	}
	whereClause := "WHERE session_revision IS NULL OR session_revision = ''"
	if d.IsPostgres() {
		whereClause = "WHERE session_revision IS NULL"
	}
	_, err := entClient.ExecContext(ctx, fmt.Sprintf(`
UPDATE sessions SET session_revision = (
  SELECT COUNT(*) FROM messages WHERE messages.session_id = sessions.id AND role = 'user'
) %s`, whereClause))
	return err
}

func ddlPluginRunSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsurePluginRunSchema(ctx, entClient)
}

func ddlFlowLogSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureFlowLogSchema(ctx, entClient)
}

func ddlMessageFTSSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	// Phase 1c-3: messages_fts schema maintenance removed. The FTS5 virtual
	// table and triggers are dropped by migration 20260902_drop_messages_subsystem.sql.
	// This migration entry is retained for gate continuity on existing deployments.
	return nil
}

func ddlChannelInboundSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureChannelInboundSchema(ctx, rawDB)
}

func ddlChannelTurnJobSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureChannelTurnJobSchema(ctx, rawDB)
}

func ddlChannelRuntimeLeaseSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureChannelRuntimeLeaseSchema(ctx, rawDB)
}

func ddlSessionRunSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureSessionRunSchema(ctx, rawDB, lg)
}

func ddlSessionParticipantSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureSessionParticipantSchema(ctx, rawDB, lg)
}

func ddlMonitorAlertSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureMonitorAlertSchema(ctx, entClient, d)
}

func ddlEcosystemSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureEcosystemSchema(ctx, entClient)
}

func ddlTeamGraphSessionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureTeamGraphSessionSchema(ctx, rawDB)
}

func ddlCompiledTeamSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureCompiledTeamSchema(ctx, rawDB)
}

func ddlSkillEvolutionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureSkillEvolutionSchema(ctx, entClient)
}

func ddlTaskPlanSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureTaskPlanSchema(ctx, rawDB, lg)
}

func ddlAllocationPlanSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureAllocationPlanSchema(ctx, rawDB, lg)
}

func ddlAgentPerformanceSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureAgentPerformanceSchema(ctx, rawDB, lg)
}

func ddlOrchestrationSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureOrchestrationSchema(ctx, rawDB, d, lg)
}

func ddlEcosystemPresetDataMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
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
func ddlAgentSourceDataMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent source migration: %w", err)
	}

	// Fix over-broad migration from 20260718: teams with kind='ecosystem_preset' but
	// no ecosystem_preset agent members should be 'user'. The 20260718 migration set
	// kind='ecosystem_preset' for ALL source='imported' teams, including user-imported packs.
	// Heuristic: if a team has NO members referencing ecosystem_preset agents, revert to 'user'.
	// Use dialect-aware JSON extraction: SQLite json_extract / json_each vs Postgres ->> / json_array_elements.
	//
	// This runs in a separate transaction because the complex JSON query may fail
	// on some rows (e.g. malformed JSON); we treat it as non-critical and don't
	// want to abort the entire migration.
	//
	// TECH-DEBT(debt): This migration is best-effort: errors are logged but not
	// returned, which means the migration version is still recorded as applied
	// even if the fix failed. This could leave teams with incorrect kind on
	// retry. Acceptable because the heuristic is non-destructive (only reverts
	// kind to 'user', never corrupts data). Issue: track via TECH-DEBT log.
	jsonExtract := d.JSONExtract
	jsonEach := d.JSONEach
	tx2, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		lg.Warn("ddl migration: begin tx for team kind fix failed", loggateway.Err(err))
		return nil
	}
	defer tx2.Rollback()
	if _, err := tx2.ExecContext(ctx, fmt.Sprintf(`
		UPDATE teams SET kind = 'user', source = 'user'
		WHERE kind = 'ecosystem_preset'
		  AND deleted_at = ''
		  AND id NOT IN (
		    SELECT DISTINCT t2.id
		    FROM teams t2, %s AS tm
		    INNER JOIN agents a ON a.id = %s AND a.kind = 'ecosystem_preset' AND a.deleted_at = ''
		    WHERE t2.kind = 'ecosystem_preset' AND t2.deleted_at = ''
		  )
	`, jsonEach("t2.definition_json"), jsonExtract("tm.value", "agent_id"))); err != nil {
		// Non-critical: log the error but don't fail the entire migration
		lg.Warn("ddl migration: fix over-broad team kind migration failed", loggateway.Err(err))
		return nil
	}
	if err := tx2.Commit(); err != nil {
		lg.Warn("ddl migration: commit team kind fix failed", loggateway.Err(err))
	}
	return nil
}

func ddlUnifiedEvolutionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureUnifiedEvolutionSchema(ctx, entClient)
}

func ddlEvolutionSuggestionPreApplySnapshot(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE evolution_suggestions ADD COLUMN pre_apply_snapshot TEXT NOT NULL DEFAULT ''`); err != nil && !d.AlreadyExistsErr(err) {
		return fmt.Errorf("add evolution_suggestions.pre_apply_snapshot: %w", err)
	}
	return nil
}

// ddlHealRecordMetadataColumn adds the metadata column to heal_records table
// for persisting extra context (root cause metadata, event metadata, etc.)
// as a JSON-encoded TEXT field. Idempotent: "duplicate column" errors are
// treated as success per DB-N6.
func ddlHealRecordMetadataColumn(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE heal_records ADD COLUMN metadata TEXT NOT NULL DEFAULT '{}'`); err != nil && !d.AlreadyExistsErr(err) {
		return fmt.Errorf("add heal_records.metadata: %w", err)
	}
	return nil
}

// ddlIntentPassDefaultOnMigration corrects the historical false default of
// intent_pass_enabled for non-A2A agents. A2A proxy agents (config_json contains
// "a2a_proxy") keep false (set explicitly in biz.validateAgentUpdate when
// IsA2AProxyAgent(agent) is true).
// Idempotent: only updates rows where intent_pass_enabled = FALSE.
func ddlIntentPassDefaultOnMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, _ Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin intent_pass_default_on tx: %w", err)
	}
	defer tx.Rollback()
	// Flip false → true for non-A2A agents. A2A proxy agents are identified by
	// config_json containing "a2a_proxy" (agent_kind=a2a_proxy embedded by
	// EmbedAgentKindInConfigJSON). Excluding them preserves their explicit false.
	// Use TRUE/FALSE literals (not 1/0): Ent field.Bool maps to PostgreSQL boolean,
	// which rejects "boolean = integer" with pq: operator does not exist (42883).
	// SQLite 3.23+ also accepts TRUE/FALSE as aliases for 1/0, so this is portable.
	res, err := tx.ExecContext(ctx, `
		UPDATE agent_runtime_settings
		SET intent_pass_enabled = TRUE
		WHERE intent_pass_enabled = FALSE
		  AND agent_id IN (
		    SELECT id FROM agents
		    WHERE config_json NOT LIKE '%a2a_proxy%'
		      AND deleted_at = ''
		  )
	`)
	if err != nil {
		return fmt.Errorf("migrate intent_pass_enabled default on: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		lg.Info("intent_pass_default_on migration applied",
			loggateway.StepID("data.migration.intent_pass_default_on"),
			loggateway.Int("rows_updated", int(n)))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit intent_pass_default_on migration: %w", err)
	}
	return nil
}
