package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// RunTeamCopyOwnershipMigration converts duplicated builtin/ecosystem team copies
// to user-owned teams so they can be deleted. Duplicating a protected team via
// TeamUsecase.Duplicate previously inherited kind/source from the source, leaving
// the copy protected and producing 403 on delete.
func RunTeamCopyOwnershipMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("team copy ownership migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationTeamCopyOwnership, lg)
	if err != nil {
		return fmt.Errorf("team copy ownership migration: check gate: %w", err)
	}
	if applied {
		return nil
	}
	lg.Info("team copy ownership: converting protected copies to user-owned: starting",
		loggateway.StepID("migration.team_copy_ownership"))

	hasTable, err := tableExistsWithDialect(ctx, client, lg, "teams", d)
	if err != nil {
		return fmt.Errorf("team copy ownership migration: check table: %w", err)
	}
	if !hasTable {
		if err := recordMigrationApplied(ctx, client, d, MigrationTeamCopyOwnership, migrationNameTeamCopyOwnership, lg); err != nil {
			return fmt.Errorf("team copy ownership migration: record: %w", err)
		}
		return nil
	}

	res, err := client.ExecContext(ctx,
		`UPDATE teams
		 SET kind = 'user',
		     source = 'user'
		 WHERE team_key LIKE '%-copy-%'
		   AND kind IN ('system_builtin', 'ecosystem_preset', 'marketplace', 'certified')`)
	if err != nil {
		return fmt.Errorf("team copy ownership migration: update: %w", err)
	}
	affected, _ := res.RowsAffected()
	lg.Info("team copy ownership: converted protected copies to user-owned: done",
		loggateway.StepID("migration.team_copy_ownership"),
		loggateway.Int("affected", int(affected)))

	if err := recordMigrationApplied(ctx, client, d, MigrationTeamCopyOwnership, migrationNameTeamCopyOwnership, lg); err != nil {
		return fmt.Errorf("team copy ownership migration: record: %w", err)
	}
	return nil
}
