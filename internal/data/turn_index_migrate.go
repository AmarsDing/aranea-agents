package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func entColumnExists(ctx context.Context, client *ent.Client, d Dialect, table, column string) (bool, error) {
	var query string
	var args []any
	if d.IsPostgres() {
		query = `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`
		args = []any{table, column}
	} else {
		query = `SELECT COUNT(1) FROM pragma_table_info(?) WHERE name = ?`
		args = []any{table, column}
	}
	var count int
	err := entQueryRowScan(client, ctx, query, args, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func entAddColumnIfMissing(ctx context.Context, client *ent.Client, d Dialect, table, column, ddl string) error {
	exists, err := entColumnExists(ctx, client, d, table, column)
	if err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	if exists {
		return nil
	}
	if _, err := client.ExecContext(ctx, ddl); err != nil {
		if !d.AlreadyExistsErr(err) {
			return fmt.Errorf("add column %s.%s: %w", table, column, err)
		}
	}
	return nil
}

func ensureMessagesTurnNumberPatch(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	hasTable, err := tableExistsWithDialect(ctx, client, lg, "messages", d)
	if err != nil {
		return err
	}
	if !hasTable {
		return nil
	}
	return entAddColumnIfMissing(ctx, client, d, "messages", "turn_number",
		`ALTER TABLE messages ADD COLUMN turn_number INTEGER NOT NULL DEFAULT 0`)
}

func RunTurnIndexToTurnIDMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("turn_index migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationTurnIndexToTurnID, lg)
	if err != nil {
		return fmt.Errorf("turn_index migration: check gate: %w", err)
	}
	if applied {
		return nil
	}

	// Skip on fresh databases where the messages table was dropped by migration
	// 20260902 (superseded by activities). Record as applied so it doesn't retry.
	hasTable, err := tableExistsWithDialect(ctx, client, lg, "messages", d)
	if err != nil {
		return fmt.Errorf("turn_index migration: check messages table: %w", err)
	}
	if !hasTable {
		lg.Info("turn_index migration: messages table not found, skipping", loggateway.StepID("migration.turn_index"))
		return recordMigrationApplied(ctx, client, d, MigrationTurnIndexToTurnID, migrationNameTurnIndexToTurnID, lg)
	}

	lg.Info("turn_index -> turn_id/turn_number/seq_in_turn: starting", loggateway.StepID("migration.turn_index"))

	if err := entAddColumnIfMissing(ctx, client, d, "messages", "turn_id",
		`ALTER TABLE messages ADD COLUMN turn_id VARCHAR(256) NOT NULL DEFAULT ''`); err != nil {
		return err
	}

	if err := entAddColumnIfMissing(ctx, client, d, "messages", "turn_number",
		`ALTER TABLE messages ADD COLUMN turn_number INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}

	if err := entAddColumnIfMissing(ctx, client, d, "messages", "seq_in_turn",
		`ALTER TABLE messages ADD COLUMN seq_in_turn INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}

	hasOldIndex, _ := entColumnExists(ctx, client, d, "messages", "turn_index")
	hasSTOldIndex, _ := entColumnExists(ctx, client, d, "session_turns", "turn_index")

	if hasOldIndex {
		stCol := "turn_index"
		if !hasSTOldIndex {
			stCol = "turn_number"
		}
		if _, err := client.ExecContext(ctx, fmt.Sprintf(`
UPDATE messages m
SET turn_id    = COALESCE(st.id, ''),
    turn_number = COALESCE(st.%s, 0)
FROM session_turns st
WHERE st.session_id = m.session_id
  AND st.turn_index = m.turn_index
`, stCol)); err != nil {
			lg.Warn("backfill from session_turns failed (may be expected on fresh DB)", loggateway.StepID("migration.turn_index"), loggateway.Err(err))
		}

		if _, err := client.ExecContext(ctx, `
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY session_id, turn_index ORDER BY created_at) AS rn
  FROM messages
  WHERE turn_id != ''
)
UPDATE messages m
SET seq_in_turn = r.rn
FROM ranked r
WHERE m.id = r.id
`); err != nil {
			lg.Warn("seq_in_turn backfill failed (may be expected on fresh DB)", loggateway.StepID("migration.turn_index"), loggateway.Err(err))
		}
	}

	if hasSTOldIndex {
		if _, err := client.ExecContext(ctx,
			`ALTER TABLE session_turns RENAME COLUMN turn_index TO turn_number`); err != nil {
			lg.Warn("rename session_turns.turn_index failed (may be expected if already renamed)", loggateway.StepID("migration.turn_index"), loggateway.Err(err))
		}
	}

	if err := recordMigrationApplied(ctx, client, d, MigrationTurnIndexToTurnID, migrationNameTurnIndexToTurnID, lg); err != nil {
		return fmt.Errorf("turn_index migration: record: %w", err)
	}
	lg.Info("turn_index -> turn_id/turn_number/seq_in_turn: done", loggateway.StepID("migration.turn_index"))
	return nil
}

func RunSessionTurnNumberBackfillMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("session_turn_number backfill: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationSessionTurnNumberBackfill, lg)
	if err != nil {
		return fmt.Errorf("session_turn_number backfill: check gate: %w", err)
	}
	if applied {
		return nil
	}
	lg.Info("session_turn_number backfill: starting", loggateway.StepID("migration.session_turn_number"))

	hasTable, err := tableExistsWithDialect(ctx, client, lg, "session_turns", d)
	if err != nil {
		return err
	}
	if !hasTable {
		return recordMigrationApplied(ctx, client, d, MigrationSessionTurnNumberBackfill, migrationNameSessionTurnNumberBackfill, lg)
	}

	var query string
	if d.IsPostgres() {
		query = `
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at, id) AS rn
  FROM session_turns
  WHERE turn_number = 0
)
UPDATE session_turns st
SET turn_number = r.rn
FROM ranked r
WHERE st.id = r.id`
	} else {
		query = `
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at, id) AS rn
  FROM session_turns
  WHERE turn_number = 0
)
UPDATE session_turns
SET turn_number = COALESCE((SELECT rn FROM ranked WHERE ranked.id = session_turns.id), turn_number)
WHERE id IN (SELECT id FROM ranked)`
	}

	res, err := client.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("session_turn_number backfill: %w", err)
	}
	affected, _ := res.RowsAffected()
	lg.Info("session_turn_number backfill: done", loggateway.StepID("migration.session_turn_number"), loggateway.Int64("affected", affected))

	return recordMigrationApplied(ctx, client, d, MigrationSessionTurnNumberBackfill, migrationNameSessionTurnNumberBackfill, lg)
}

func RunSessionTurnNumberRebackfillMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("session_turn_number rebackfill: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationSessionTurnNumberRebackfill, lg)
	if err != nil {
		return fmt.Errorf("session_turn_number rebackfill: check gate: %w", err)
	}
	if applied {
		return nil
	}
	lg.Info("session_turn_number rebackfill: starting", loggateway.StepID("migration.session_turn_number_rebackfill"))

	hasTable, err := tableExistsWithDialect(ctx, client, lg, "session_turns", d)
	if err != nil {
		return err
	}
	if !hasTable {
		return recordMigrationApplied(ctx, client, d, MigrationSessionTurnNumberRebackfill, migrationNameSessionTurnNumberRebackfill, lg)
	}

	var query string
	if d.IsPostgres() {
		query = `
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at, id) AS rn
  FROM session_turns
)
UPDATE session_turns st
SET turn_number = r.rn
FROM ranked r
WHERE st.id = r.id`
	} else {
		query = `
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at, id) AS rn
  FROM session_turns
)
UPDATE session_turns
SET turn_number = COALESCE((SELECT rn FROM ranked WHERE ranked.id = session_turns.id), turn_number)
WHERE id IN (SELECT id FROM ranked)`
	}

	res, err := client.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("session_turn_number rebackfill: %w", err)
	}
	affected, _ := res.RowsAffected()
	lg.Info("session_turn_number rebackfill: done", loggateway.StepID("migration.session_turn_number_rebackfill"), loggateway.Int64("affected", affected))

	return recordMigrationApplied(ctx, client, d, MigrationSessionTurnNumberRebackfill, migrationNameSessionTurnNumberRebackfill, lg)
}
