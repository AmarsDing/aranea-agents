package data

import (
	"context"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// isColumnExistsErr checks if the error is due to a duplicate column.
func isColumnExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column name") || strings.Contains(msg, "already exists")
}

func isNoSuchTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table")
}

func sqliteTableExists(ctx context.Context, c *ent.Client, lg loggateway.Logger, table string) (bool, error) {
	table = strings.TrimSpace(table)
	if table == "" {
		return false, nil
	}
	rows, err := c.QueryContext(ctx,
		`SELECT COUNT(1) FROM sqlite_master WHERE type IN ('table','view') AND name = ?`, table)
	if err != nil {
		lg.Warn("sqlite table exists check failed", loggateway.StepID("data.startup.sqlite_check"), loggateway.Err(err))
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	var n int
	if err := rows.Scan(&n); err != nil {
		return false, err
	}
	return n > 0, rows.Err()
}

func sqliteColumnExists(ctx context.Context, c *ent.Client, lg loggateway.Logger, table, column string) (bool, error) {
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" {
		return false, nil
	}
	rows, err := c.QueryContext(ctx, `SELECT 1 FROM pragma_table_info(?) WHERE name = ? LIMIT 1`, table, column)
	if err != nil {
		lg.Warn("sqlite column exists check failed", loggateway.StepID("data.startup.sqlite_check"), loggateway.Err(err))
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

func sqliteIndexExists(ctx context.Context, c *ent.Client, lg loggateway.Logger, table, indexName string) (bool, error) {
	indexName = strings.TrimSpace(indexName)
	if indexName == "" {
		return false, nil
	}
	rows, err := c.QueryContext(ctx, `SELECT 1 FROM pragma_index_list(?) WHERE name = ? LIMIT 1`, table, indexName)
	if err != nil {
		lg.Warn("sqlite index exists check failed", loggateway.StepID("data.startup.sqlite_check"), loggateway.Err(err))
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
