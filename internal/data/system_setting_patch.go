package data

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/data/ent"
)

func ensureSystemSettingPatches(ctx context.Context, c *ent.Client) error {
	if c == nil {
		return nil
	}
	patches := []struct {
		col string
		ddl string
	}{
		{"a2a_public_base_url", `ALTER TABLE system_settings ADD COLUMN a2a_public_base_url TEXT NOT NULL DEFAULT ''`},
		{"credential_encryption_key", `ALTER TABLE system_settings ADD COLUMN credential_encryption_key TEXT NOT NULL DEFAULT ''`},
		{"knowledge_embed_provider", `ALTER TABLE system_settings ADD COLUMN knowledge_embed_provider TEXT NOT NULL DEFAULT ''`},
		{"knowledge_embed_base_url", `ALTER TABLE system_settings ADD COLUMN knowledge_embed_base_url TEXT NOT NULL DEFAULT ''`},
		{"knowledge_embed_api_key", `ALTER TABLE system_settings ADD COLUMN knowledge_embed_api_key TEXT NOT NULL DEFAULT ''`},
		{"knowledge_embed_model", `ALTER TABLE system_settings ADD COLUMN knowledge_embed_model TEXT NOT NULL DEFAULT ''`},
		{"knowledge_embed_dim", `ALTER TABLE system_settings ADD COLUMN knowledge_embed_dim INTEGER NOT NULL DEFAULT 0`},
		{"mcp_allow_adhoc_http", `ALTER TABLE system_settings ADD COLUMN mcp_allow_adhoc_http INTEGER NOT NULL DEFAULT 0`},
		{"eval_sim_provider", `ALTER TABLE system_settings ADD COLUMN eval_sim_provider TEXT NOT NULL DEFAULT ''`},
		{"eval_sim_model", `ALTER TABLE system_settings ADD COLUMN eval_sim_model TEXT NOT NULL DEFAULT ''`},
		{"eval_judge_provider", `ALTER TABLE system_settings ADD COLUMN eval_judge_provider TEXT NOT NULL DEFAULT ''`},
		{"eval_judge_model", `ALTER TABLE system_settings ADD COLUMN eval_judge_model TEXT NOT NULL DEFAULT ''`},
	}
	for _, p := range patches {
		has, err := sqliteTableHasColumn(ctx, c, "system_settings", p.col)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := c.ExecContext(ctx, p.ddl); err != nil {
			return err
		}
	}
	return nil
}

func sqliteTableHasColumn(ctx context.Context, c *ent.Client, table, column string) (bool, error) {
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" {
		return false, nil
	}
	query := fmt.Sprintf(`SELECT 1 FROM pragma_table_info('%s') WHERE name = ? LIMIT 1`, table)
	rows, err := c.QueryContext(ctx, query, column)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, nil
	}
	var one int
	if err := rows.Scan(&one); err != nil {
		return false, err
	}
	return true, nil
}
