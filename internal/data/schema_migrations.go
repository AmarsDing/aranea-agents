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
	MigrationTeamCopyOwnership               = 20260624
	migrationNameTeamCopyOwnership           = "team_copy_ownership_to_user"
	MigrationAuditActionNormalize            = 20260729
	migrationNameAuditActionNormalize        = "audit_action_verb_first_normalize"
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
