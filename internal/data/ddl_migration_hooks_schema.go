package data

import (
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
	"context"
	"database/sql"
)

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
	return EnsureEvalSchema(ctx, rawDB, d)
}

func ddlA2ASchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureA2ASchema(ctx, rawDB, d)
}

func ddlPluginRunSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsurePluginRunSchema(ctx, entClient)
}

func ddlFlowLogSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureFlowLogSchema(ctx, entClient)
}

func ddlMessageFTSSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {

	return nil
}

func ddlChannelInboundSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureChannelInboundSchema(ctx, rawDB)
}

func ddlChatTurnSubmitClaimSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureChatTurnSubmitClaimSchema(ctx, rawDB)
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

func ddlLearningLoopSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureLearningLoopSchema(ctx, entClient)
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
