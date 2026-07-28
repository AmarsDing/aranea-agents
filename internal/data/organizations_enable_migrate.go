package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// ddlOrganizationsEnableCopyPositionBackfill (migration 20261112) fixes two
// legacy data issues on pre-existing databases:
//
//  1. organizations.enabled = false for all seeded agency-agent hierarchy
//     nodes (3 company / 26 department / 239 position). The legacy
//     industry_taxonomy seed wrote enabled=false; the Ent schema default is
//     true, so fresh installs are unaffected. The frontend taxonomy filter
//     (industryNodes) requires enabled=true, so nothing renders without this.
//  2. Copy agents created before the agent_duplicate.go fix lost their
//     position_id/position_key (the old code cleared them). Backfill by
//     matching display_name: a copy is named "<source> Copy" (possibly
//     nested, e.g. "<source> Copy Copy"); strip all trailing " Copy"
//     segments to find the source agent and inherit its position.
//
// Idempotent: the enabled update only touches enabled=FALSE rows; the copy
// backfill only touches rows with position_key='' whose source has a
// non-empty position_key. Re-running is a no-op.
//
// The unique index on (position_key, agent_variant) is safe because copy
// agents carry agent_variant = their own unique agent_key (see
// biz/agent_duplicate.go).
func ddlOrganizationsEnableCopyPositionBackfill(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin organizations_enable_copy_position_backfill tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Enable all organization nodes. TRUE/FALSE literals are accepted by
	// both Postgres booleans and SQLite 3.23+ (precedent:
	// ddlIntentPassDefaultOnMigration).
	res, err := tx.ExecContext(ctx, `UPDATE organizations SET enabled = TRUE WHERE enabled = FALSE`)
	if err != nil {
		return fmt.Errorf("enable organizations: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		lg.Info("organizations enabled migration applied",
			loggateway.StepID("data.migration.organizations_enable_copy_position_backfill"),
			loggateway.Int("rows_updated", int(n)))
	}

	// 2. Backfill copy agent positions from their source agents.
	rows, err := tx.QueryContext(ctx, `SELECT id, display_name FROM agents
		WHERE agent_key LIKE '%-copy-%' AND position_key = '' AND deleted_at = ''`)
	if err != nil {
		return fmt.Errorf("query copy agents: %w", err)
	}
	type copyRow struct{ id, displayName string }
	var copies []copyRow
	for rows.Next() {
		var c copyRow
		if err := rows.Scan(&c.id, &c.displayName); err != nil {
			rows.Close()
			return fmt.Errorf("scan copy agent: %w", err)
		}
		copies = append(copies, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate copy agents: %w", err)
	}
	rows.Close()

	backfilled := 0
	for _, c := range copies {
		srcName := c.displayName
		for strings.HasSuffix(srcName, " Copy") {
			srcName = strings.TrimSuffix(srcName, " Copy")
		}
		if srcName == "" || srcName == c.displayName {
			continue
		}
		var pid, pkey string
		err := tx.QueryRowContext(ctx, d.RenumberPlaceholders(`SELECT position_id, position_key FROM agents
			WHERE display_name = ? AND position_key <> '' AND deleted_at = ''
			ORDER BY created_at ASC LIMIT 1`), srcName).Scan(&pid, &pkey)
		if err == sql.ErrNoRows {
			lg.Warn("copy agent position backfill: no source match",
				loggateway.StepID("data.migration.organizations_enable_copy_position_backfill"),
				loggateway.Str("agent_id", c.id),
				loggateway.Str("source_display_name", srcName))
			continue
		}
		if err != nil {
			return fmt.Errorf("find source for copy %s: %w", c.id, err)
		}
		if _, err := tx.ExecContext(ctx, d.RenumberPlaceholders(`UPDATE agents SET position_id = ?, position_key = ? WHERE id = ?`),
			pid, pkey, c.id); err != nil {
			return fmt.Errorf("backfill copy %s: %w", c.id, err)
		}
		backfilled++
	}
	if backfilled > 0 {
		lg.Info("copy agent position backfill applied",
			loggateway.StepID("data.migration.organizations_enable_copy_position_backfill"),
			loggateway.Int("rows_updated", backfilled))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit organizations_enable_copy_position_backfill: %w", err)
	}
	return nil
}
