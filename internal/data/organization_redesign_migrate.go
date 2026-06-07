package data

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

const (
	MigrationOrganizationRedesign     = 20260720
	migrationNameOrganizationRedesign = "organization_redesign"
)

// PreEntOrganizationRedesignMigration runs BEFORE Ent Schema.Create to rename
// the industry_taxonomy table to organizations. This must happen before Ent
// auto-creates the organizations table, otherwise we'd end up with both tables
// and lose the existing data.
//
// This function is idempotent — safe to run multiple times.
func PreEntOrganizationRedesignMigration(ctx context.Context, rawDB *sql.DB, lg loggateway.Logger) {
	if rawDB == nil {
		return
	}

	// Check if industry_taxonomy table exists
	var count int
	err := rawDB.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='industry_taxonomy'`,
	).Scan(&count)
	if err != nil {
		lg.Warn("organization redesign pre-migration: cannot check industry_taxonomy existence",
			loggateway.StepID("migration.organization_redesign.pre"), loggateway.Err(err))
		return
	}
	if count == 0 {
		// No old table — nothing to rename
		return
	}

	// Check if organizations table already exists
	var orgCount int
	err = rawDB.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='organizations'`,
	).Scan(&orgCount)
	if err != nil {
		lg.Warn("organization redesign pre-migration: cannot check organizations existence",
			loggateway.StepID("migration.organization_redesign.pre"), loggateway.Err(err))
		return
	}

	if orgCount > 0 {
		// Both tables exist — Ent already created organizations.
		// Copy any remaining data from industry_taxonomy that isn't in organizations,
		// then drop the old table.
		lg.Info("organization redesign pre-migration: both tables exist, migrating data",
			loggateway.StepID("migration.organization_redesign.pre"))
		migrateIndustryTaxonomyData(ctx, rawDB, lg)
		return
	}

	// Only industry_taxonomy exists — rename it
	lg.Info("organization redesign pre-migration: renaming industry_taxonomy → organizations",
		loggateway.StepID("migration.organization_redesign.pre"))
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE industry_taxonomy RENAME TO organizations`); err != nil {
		lg.Error("organization redesign pre-migration: rename table failed",
			loggateway.StepID("migration.organization_redesign.pre"), loggateway.Err(err))
		return
	}

	// Rename column taxonomy_key → org_key (SQLite 3.25+)
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE organizations RENAME COLUMN taxonomy_key TO org_key`); err != nil {
		if !isColumnExistsErr(err) {
			lg.Warn("organization redesign pre-migration: rename taxonomy_key→org_key failed",
				loggateway.StepID("migration.organization_redesign.pre"), loggateway.Err(err))
		}
	}

	// Rename Agent column taxonomy_position_id → position_id
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE agents RENAME COLUMN taxonomy_position_id TO position_id`); err != nil {
		if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
			lg.Warn("organization redesign pre-migration: rename agents.taxonomy_position_id→position_id failed",
				loggateway.StepID("migration.organization_redesign.pre"), loggateway.Err(err))
		}
	}

	// Rename Team column category_industry_id → department_id
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE teams RENAME COLUMN category_industry_id TO department_id`); err != nil {
		if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
			lg.Warn("organization redesign pre-migration: rename teams.category_industry_id→department_id failed",
				loggateway.StepID("migration.organization_redesign.pre"), loggateway.Err(err))
		}
	}
}

// migrateIndustryTaxonomyData handles the case where both industry_taxonomy and
// organizations tables exist. It copies data from the old table to the new one
// and drops the old table.
func migrateIndustryTaxonomyData(ctx context.Context, rawDB *sql.DB, lg loggateway.Logger) {
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		lg.Error("organization redesign: begin data migration tx failed",
			loggateway.StepID("migration.organization_redesign.data"), loggateway.Err(err))
		return
	}
	defer tx.Rollback()

	// Copy rows from industry_taxonomy that don't already exist in organizations
	// Map: taxonomy_key → org_key, keep all other columns as-is
	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO organizations (id, org_key, name, description, status, enabled, sort_order,
			parent_id, level, scenario_key, workspace_id, owner_user_id, is_system,
			config_json, metadata_json, created_at, updated_at, deleted_at)
		SELECT id, taxonomy_key, name, description, status, enabled, sort_order,
			parent_id, level, scenario_key, workspace_id, owner_user_id, is_system,
			config_json, metadata_json, created_at, updated_at, deleted_at
		FROM industry_taxonomy
		WHERE id NOT IN (SELECT id FROM organizations)
	`)
	if err != nil {
		lg.Warn("organization redesign: copy data from industry_taxonomy failed (non-fatal)",
			loggateway.StepID("migration.organization_redesign.data"), loggateway.Err(err))
		// Continue — don't block startup
	} else {
		lg.Info("organization redesign: data copied from industry_taxonomy to organizations",
			loggateway.StepID("migration.organization_redesign.data"))
	}

	// Drop old table
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS industry_taxonomy`); err != nil {
		lg.Warn("organization redesign: drop industry_taxonomy failed (non-fatal)",
			loggateway.StepID("migration.organization_redesign.data"), loggateway.Err(err))
	}

	if err := tx.Commit(); err != nil {
		lg.Error("organization redesign: commit data migration tx failed",
			loggateway.StepID("migration.organization_redesign.data"), loggateway.Err(err))
	}
}

// RunOrganizationRedesignMigration performs the post-Ent schema migration:
// adds new columns and updates level values from 'industry' to 'company'.
// The pre-Ent table rename is handled by PreEntOrganizationRedesignMigration.
//
// The migration is idempotent — safe to run multiple times.
func RunOrganizationRedesignMigration(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("organization redesign migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationOrganizationRedesign, lg)
	if err != nil {
		return fmt.Errorf("organization redesign migration: check gate: %w", err)
	}
	if applied {
		return nil
	}
	lg.Info("organization redesign: starting post-Ent schema migration", loggateway.StepID("migration.organization_redesign"))

	// Step 1: Rename column taxonomy_key → org_key (in case pre-migration didn't run)
	if _, err := client.ExecContext(ctx, `ALTER TABLE organizations RENAME COLUMN taxonomy_key TO org_key`); err != nil {
		if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
			lg.Debug("organization redesign: rename org_key skipped", loggateway.StepID("migration.organization_redesign"), loggateway.Err(err))
		}
	}

	// Step 2: Rename Agent column taxonomy_position_id → position_id
	if _, err := client.ExecContext(ctx, `ALTER TABLE agents RENAME COLUMN taxonomy_position_id TO position_id`); err != nil {
		if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
			lg.Debug("organization redesign: rename agents.position_id skipped", loggateway.StepID("migration.organization_redesign"), loggateway.Err(err))
		}
	}

	// Step 3: Rename Team column category_industry_id → department_id
	if _, err := client.ExecContext(ctx, `ALTER TABLE teams RENAME COLUMN category_industry_id TO department_id`); err != nil {
		if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
			lg.Debug("organization redesign: rename teams.department_id skipped", loggateway.StepID("migration.organization_redesign"), loggateway.Err(err))
		}
	}

	// Step 4: Add new columns (idempotent — ignore duplicate column errors)
	newColumns := []struct {
		table string
		col   string
		typ   string
	}{
		{"teams", "deliverables", "TEXT DEFAULT '[]'"},
		{"teams", "input_contract", "TEXT DEFAULT '[]'"},
		{"teams", "dept_lead_agent_id", "TEXT DEFAULT ''"},
		{"teams", "cross_dept_member_ids", "TEXT DEFAULT '[]'"},
		{"graph_definitions", "team_id", "TEXT DEFAULT ''"},
		{"graph_definitions", "is_template", "BOOLEAN DEFAULT 0"},
		{"graph_definitions", "verification_gates", "TEXT DEFAULT '[]'"},
		{"organizations", "dept_lead_agent_id", "TEXT DEFAULT ''"},
		{"organizations", "dept_lead_config_json", "TEXT DEFAULT '{}'"},
	}

	for _, c := range newColumns {
		sql := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, c.table, c.col, c.typ)
		if _, err := client.ExecContext(ctx, sql); err != nil {
			if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
				lg.Debug("organization redesign: add column skipped",
					loggateway.StepID("migration.organization_redesign"),
					loggateway.Str("table", c.table),
					loggateway.Str("column", c.col),
					loggateway.Err(err))
			}
		}
	}

	// Step 5: Update level values: 'industry' → 'company'
	if _, err := client.ExecContext(ctx, `UPDATE organizations SET level = 'company' WHERE level = 'industry'`); err != nil {
		if !isNoSuchTableErr(err) {
			lg.Warn("organization redesign: update level industry→company failed (non-critical)",
				loggateway.StepID("migration.organization_redesign"), loggateway.Err(err))
		}
	}

	if err := recordMigrationApplied(ctx, client, MigrationOrganizationRedesign, migrationNameOrganizationRedesign, lg); err != nil {
		return fmt.Errorf("organization redesign migration: record: %w", err)
	}
	lg.Info("organization redesign: post-Ent schema migration done", loggateway.StepID("migration.organization_redesign"))
	return nil
}

// RollbackOrganizationRedesignMigration reverses the organization redesign migration.
// WARNING: This is a destructive operation — it will rename organizations back to
// industry_taxonomy and revert column names. Use only for emergency rollback.
//
// The rollback is idempotent — safe to run multiple times.
func RollbackOrganizationRedesignMigration(ctx context.Context, rawDB *sql.DB, lg loggateway.Logger) error {
	if rawDB == nil {
		return fmt.Errorf("organization redesign rollback: rawDB required")
	}

	lg.Info("organization redesign: starting rollback", loggateway.StepID("migration.organization_redesign.rollback"))

	// Step 1: Revert level values: 'company' → 'industry'
	if _, err := rawDB.ExecContext(ctx, `UPDATE organizations SET level = 'industry' WHERE level = 'company'`); err != nil {
		if !isNoSuchTableErr(err) {
			lg.Warn("organization redesign rollback: revert level company→industry failed (non-critical)",
				loggateway.StepID("migration.organization_redesign.rollback"), loggateway.Err(err))
		}
	}

	// Step 2: Rename column org_key → taxonomy_key
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE organizations RENAME COLUMN org_key TO taxonomy_key`); err != nil {
		if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
			lg.Warn("organization redesign rollback: rename org_key→taxonomy_key failed",
				loggateway.StepID("migration.organization_redesign.rollback"), loggateway.Err(err))
		}
	}

	// Step 3: Rename Agent column position_id → taxonomy_position_id
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE agents RENAME COLUMN position_id TO taxonomy_position_id`); err != nil {
		if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
			lg.Warn("organization redesign rollback: rename agents.position_id→taxonomy_position_id failed",
				loggateway.StepID("migration.organization_redesign.rollback"), loggateway.Err(err))
		}
	}

	// Step 4: Rename Team column department_id → category_industry_id
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE teams RENAME COLUMN department_id TO category_industry_id`); err != nil {
		if !isColumnExistsErr(err) && !isNoSuchTableErr(err) {
			lg.Warn("organization redesign rollback: rename teams.department_id→category_industry_id failed",
				loggateway.StepID("migration.organization_redesign.rollback"), loggateway.Err(err))
		}
	}

	// Step 5: Rename table organizations → industry_taxonomy
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE organizations RENAME TO industry_taxonomy`); err != nil {
		if !isNoSuchTableErr(err) {
			return fmt.Errorf("organization redesign rollback: rename table: %w", err)
		}
	}

	lg.Info("organization redesign: rollback done", loggateway.StepID("migration.organization_redesign.rollback"))
	return nil
}
