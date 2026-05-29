package data

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/event"
)

func entColumnExists(ctx context.Context, client *ent.Client, table, column string) (bool, error) {
	var count int
	err := entQueryRowScan(client, ctx,
		`SELECT COUNT(1) FROM pragma_table_info(?) WHERE name = ?`,
		[]any{table, column}, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func entAddColumnIfMissing(ctx context.Context, client *ent.Client, table, column, ddl string) error {
	exists, err := entColumnExists(ctx, client, table, column)
	if err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	if exists {
		return nil
	}
	if _, err := client.ExecContext(ctx, ddl); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add column %s.%s: %w", table, column, err)
		}
	}
	return nil
}

func ensureMessagesTurnNumberPatch(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	hasTable, err := sqliteTableExists(ctx, client, "messages")
	if err != nil {
		return err
	}
	if !hasTable {
		return nil
	}
	return entAddColumnIfMissing(ctx, client, "messages", "turn_number",
		`ALTER TABLE messages ADD COLUMN turn_number INTEGER NOT NULL DEFAULT 0`)
}

func RunTurnIndexToTurnIDMigration(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return fmt.Errorf("turn_index migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationTurnIndexToTurnID)
	if err != nil {
		return fmt.Errorf("turn_index migration: check gate: %w", err)
	}
	if applied {
		return nil
	}
	event.SysLogInfo("migration.turn_index", "turn_index -> turn_id/turn_number/seq_in_turn: starting")

	if err := entAddColumnIfMissing(ctx, client, "messages", "turn_id",
		`ALTER TABLE messages ADD COLUMN turn_id VARCHAR(256) NOT NULL DEFAULT ''`); err != nil {
		return err
	}

	if err := entAddColumnIfMissing(ctx, client, "messages", "turn_number",
		`ALTER TABLE messages ADD COLUMN turn_number INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}

	if err := entAddColumnIfMissing(ctx, client, "messages", "seq_in_turn",
		`ALTER TABLE messages ADD COLUMN seq_in_turn INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}

	hasOldIndex, _ := entColumnExists(ctx, client, "messages", "turn_index")
	hasSTOldIndex, _ := entColumnExists(ctx, client, "session_turns", "turn_index")

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
			event.SysLogWarn("migration.turn_index", "backfill from session_turns failed (may be expected on fresh DB)", event.P("error", err.Error()))
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
			event.SysLogWarn("migration.turn_index", "seq_in_turn backfill failed (may be expected on fresh DB)", event.P("error", err.Error()))
		}
	}

	if hasSTOldIndex {
		if _, err := client.ExecContext(ctx,
			`ALTER TABLE session_turns RENAME COLUMN turn_index TO turn_number`); err != nil {
			event.SysLogWarn("migration.turn_index", "rename session_turns.turn_index failed (may be expected if already renamed)", event.P("error", err.Error()))
		}
	}

	if err := recordMigrationApplied(ctx, client, MigrationTurnIndexToTurnID, migrationNameTurnIndexToTurnID); err != nil {
		return fmt.Errorf("turn_index migration: record: %w", err)
	}
	event.SysLogInfo("migration.turn_index", "turn_index -> turn_id/turn_number/seq_in_turn: done")
	return nil
}
