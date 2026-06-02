package data

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/pkg/loggateway"
)

type ddlMigration struct {
	Version int
	Name    string
	Func    func(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error
}

var ddlMigrations = []ddlMigration{
	{Version: 20260601, Name: "session_memory_patches", Func: ddlSessionMemoryPatches},
	{Version: 20260602, Name: "memory_facts_index_status", Func: ddlMemoryFactsIndexStatus},
	{Version: 20260603, Name: "messages_turn_number", Func: ddlMessagesTurnNumber},
	{Version: 20260604, Name: "session_memory_schema", Func: ddlSessionMemorySchema},
	{Version: 20260605, Name: "memory_relation_patches", Func: ddlMemoryRelationPatches},
	{Version: 20260606, Name: "monitor_schema_patches", Func: ddlMonitorSchemaPatches},
	{Version: 20260607, Name: "agent_runtime_patches", Func: ddlAgentRuntimePatches},
	{Version: 20260608, Name: "entity_reinforcements_schema", Func: ddlEntityReinforcementsSchema},
	{Version: 20260609, Name: "cascade_saga_patches", Func: ddlCascadeSagaPatches},
	{Version: 20260610, Name: "builtin_platform_tools", Func: ddlBuiltinPlatformTools},
	{Version: 20260611, Name: "system_setting_patches", Func: ddlSystemSettingPatches},
	{Version: 20260612, Name: "pricing_rule_patches", Func: ddlPricingRulePatches},
	{Version: 20260613, Name: "llm_provider_model_capability", Func: ddlLlmProviderModelCapability},
	{Version: 20260614, Name: "default_system_setting", Func: ddlDefaultSystemSetting},
	{Version: 20260615, Name: "credential_encryption_key", Func: ddlCredentialEncryptionKey},
	{Version: 20260616, Name: "eval_schema", Func: ddlEvalSchema},
	{Version: 20260617, Name: "a2a_schema", Func: ddlA2ASchema},
	{Version: 20260618, Name: "a2a_remote_health_patches", Func: ddlA2ARemoteHealthPatches},
	{Version: 20260619, Name: "team_run_summary_patches", Func: ddlTeamRunSummaryPatches},
	{Version: 20260620, Name: "session_revision_patches", Func: ddlSessionRevisionPatches},
	{Version: 20260621, Name: "plugin_run_schema", Func: ddlPluginRunSchema},
	{Version: 20260622, Name: "hook_delivery_schema", Func: ddlHookDeliverySchema},
	{Version: 20260623, Name: "flow_log_schema", Func: ddlFlowLogSchema},
	{Version: 20260624, Name: "message_fts_schema", Func: ddlMessageFTSSchema},
	{Version: 20260625, Name: "channel_inbound_schema", Func: ddlChannelInboundSchema},
	{Version: 20260626, Name: "channel_turn_job_schema", Func: ddlChannelTurnJobSchema},
	{Version: 20260627, Name: "channel_runtime_lease_schema", Func: ddlChannelRuntimeLeaseSchema},
	{Version: 20260628, Name: "session_run_schema", Func: ddlSessionRunSchema},
	{Version: 20260629, Name: "session_participant_schema", Func: ddlSessionParticipantSchema},
	{Version: 20260630, Name: "session_run_checkpoint_schema", Func: ddlSessionRunCheckpointSchema},
	{Version: 20260701, Name: "session_run_column_patches", Func: ddlSessionRunColumnPatches},
	{Version: 20260702, Name: "monitor_alert_schema", Func: ddlMonitorAlertSchema},
	{Version: 20260703, Name: "ecosystem_schema", Func: ddlEcosystemSchema},
	{Version: 20260704, Name: "team_graph_session_schema", Func: ddlTeamGraphSessionSchema},
	{Version: 20260705, Name: "compiled_team_schema", Func: ddlCompiledTeamSchema},
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
		if err := m.Func(ctx, rawDB, entClient, lg); err != nil {
			lg.Error("schema step failed",
				loggateway.StepID("data.schema."+m.Name),
				loggateway.Int("version", m.Version),
				loggateway.Err(err))
			return fmt.Errorf("%s: %w", m.Name, err)
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

func ddlSessionMemoryPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return sessionmemory.EnsurePatches(ctx, entClient)
}

func ddlMemoryFactsIndexStatus(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureMemoryFactsIndexStatusPatches(ctx, entClient, lg)
}

func ddlMessagesTurnNumber(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureMessagesTurnNumberPatch(ctx, entClient, lg)
}

func ddlSessionMemorySchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureSessionMemorySchema(ctx, entClient, lg)
}

func ddlMemoryRelationPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return sessionmemory.EnsureMemoryRelationPatches(ctx, entClient)
}

func ddlMonitorSchemaPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return sessionmemory.EnsureMonitorSchemaPatches(ctx, entClient)
}

func ddlAgentRuntimePatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureAgentRuntimePatches(ctx, entClient, lg)
}

func ddlEntityReinforcementsSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureEntityReinforcementsSchema(ctx, entClient, lg)
}

func ddlCascadeSagaPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureCascadeSagaPatches(ctx, entClient, lg)
}

func ddlBuiltinPlatformTools(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureBuiltinPlatformTools(ctx, entClient, lg)
}

func ddlSystemSettingPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureSystemSettingPatches(ctx, entClient)
}

func ddlPricingRulePatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensurePricingRulePatches(ctx, entClient, lg)
}

func ddlLlmProviderModelCapability(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureLlmProviderModelCapabilityPatches(ctx, entClient, lg)
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

func ddlA2ARemoteHealthPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureA2ARemoteHealthPatches(ctx, entClient, lg)
}

func ddlTeamRunSummaryPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureTeamRunSummaryPatches(ctx, entClient, lg)
}

func ddlSessionRevisionPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureSessionRevisionPatches(ctx, entClient, lg)
}

func ddlPluginRunSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsurePluginRunSchema(ctx, entClient)
}

func ddlHookDeliverySchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return EnsureHookDeliverySchema(ctx, entClient, lg)
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

func ddlSessionRunCheckpointSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureSessionRunCheckpointSchema(ctx, rawDB, lg)
}

func ddlSessionRunColumnPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, lg loggateway.Logger) error {
	return ensureSessionRunColumnPatches(ctx, rawDB, lg)
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
