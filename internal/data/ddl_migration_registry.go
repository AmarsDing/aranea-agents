package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

type ddlMigration struct {
	Version int
	Name    string
	// SQL is an optional path to a SQL file to execute (relative to project root).
	// If set, the SQL file is executed first via rawDB, then Func is called (if not nil).
	SQL string
	Func func(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error
}

var ddlMigrations = []ddlMigration{
	{Version: 20260601, Name: "session_memory_patches", Func: ddlSessionMemoryPatches},
	{Version: 20260602, Name: "memory_facts_index_status", SQL: "sql/migrations/20260602_memory_facts_index_status.sql"},
	{Version: 20260603, Name: "messages_turn_number", Func: ddlMessagesTurnNumber},
	{Version: 20260604, Name: "session_memory_schema", Func: ddlSessionMemorySchema},
	{Version: 20260605, Name: "memory_relation_patches", Func: ddlMemoryRelationPatches},
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
	{Version: 20260701, Name: "session_run_column_patches", SQL: "sql/migrations/20260701_session_run_column_patches.sql"},
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
			lg.Warn("failed to record migration",
				loggateway.StepID("data.ddl_migration.record"),
				loggateway.Int("version", m.Version),
				loggateway.Err(err))
		}
	}
	return nil
}

// executeSQLFile reads a SQL file (path relative to project root), splits it into
// individual statements using splitDDLStatements, and executes each via rawDB.
// "duplicate column name" and "already exists" errors are treated as idempotent successes.
func executeSQLFile(ctx context.Context, rawDB *sql.DB, path string, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	sqlBytes, err := os.ReadFile(path)
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
func ddlSessionRevisionDataMigration(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	if entClient == nil {
		return nil
	}
	_, err := entClient.ExecContext(ctx, `
UPDATE sessions SET session_revision = (
  SELECT COUNT(*) FROM messages WHERE messages.session_id = sessions.id AND role = 'user'
) WHERE session_revision = 0`)
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
