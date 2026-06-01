package data

import (
	"context"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func ensureSessionRevisionPatches(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	if c == nil {
		return nil
	}
	if err := ensureSessionColumn(ctx, c, lg, "session_revision", `ALTER TABLE sessions ADD COLUMN session_revision INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	_, err := c.ExecContext(ctx, `
UPDATE sessions SET session_revision = (
  SELECT COUNT(*) FROM messages WHERE messages.session_id = sessions.id AND role = 'user'
) WHERE session_revision = 0`)
	return err
}

func ensureSessionColumn(ctx context.Context, c *ent.Client, lg loggateway.Logger, column, ddl string) error {
	has, err := sqliteColumnExists(ctx, c, lg, "sessions", column)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = c.ExecContext(ctx, ddl)
	return err
}
