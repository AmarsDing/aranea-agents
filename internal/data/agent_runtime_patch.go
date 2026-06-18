package data

import (
	"context"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// isColumnExistsErr checks if the error is due to a duplicate column.
// Deprecated: Use Dialect.AlreadyExistsErr with an explicit dialect instead.
// This function is retained for backward compatibility with callers that have
// not yet been migrated to pass an explicit Dialect.
func isColumnExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column name") || strings.Contains(msg, "already exists")
}

// isNoSuchTableErr checks if the error is due to a missing table.
// Deprecated: Use Dialect.UndefinedObjectErr with an explicit dialect instead.
// This function is retained for backward compatibility with callers that have
// not yet been migrated to pass an explicit Dialect.
func isNoSuchTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table")
}

// sqliteTableExists checks whether a table exists using SQLite-specific
// sqlite_master catalog.
//
// Deprecated: Use tableExistsWithDialect with an explicit Dialect instead.
// This wrapper retains the SQLite-only behavior for callers that have not yet
// been migrated. Note: the dialect-aware variant uses Dialect.TableExistsQuery
// which only matches type='table' (the original also matched 'view'); no
// existing caller relies on view matching.
func sqliteTableExists(ctx context.Context, c *ent.Client, lg loggateway.Logger, table string) (bool, error) {
	return tableExistsWithDialect(ctx, c, lg, table, DialectSQLite)
}

// sqliteColumnExists checks whether a column exists using SQLite-specific
// pragma_table_info.
//
// Deprecated: Use columnExistsWithDialect (monitor_trace.go) or entColumnExists
// (turn_index_migrate.go) with an explicit Dialect instead. This wrapper
// retains the SQLite-only behavior for callers that have not yet been migrated.
func sqliteColumnExists(ctx context.Context, c *ent.Client, lg loggateway.Logger, table, column string) (bool, error) {
	has, err := columnExistsWithDialect(ctx, c, table, column, DialectSQLite)
	if err != nil {
		lg.Warn("sqlite column exists check failed", loggateway.StepID("data.startup.sqlite_check"), loggateway.Err(err))
	}
	return has, err
}

// sqliteIndexExists checks whether an index exists using SQLite-specific
// pragma_index_list.
//
// Deprecated: Use indexExistsWithDialect with an explicit Dialect instead.
// This wrapper retains the SQLite-only behavior for callers that have not yet
// been migrated.
func sqliteIndexExists(ctx context.Context, c *ent.Client, lg loggateway.Logger, table, indexName string) (bool, error) {
	return indexExistsWithDialect(ctx, c, lg, table, indexName, DialectSQLite)
}

// tableExistsWithDialect checks whether a table exists using dialect-aware
// catalog queries (sqlite_master for SQLite, information_schema for Postgres).
// Prefer this over sqliteTableExists for new code.
//
// The query is sourced from Dialect.TableExistsQuery and executed via the
// *ent.Client's QueryContext (shares the same connection pool as the Ent
// client, no separate *sql.DB required).
func tableExistsWithDialect(ctx context.Context, c *ent.Client, lg loggateway.Logger, table string, d Dialect) (bool, error) {
	table = strings.TrimSpace(table)
	if table == "" {
		return false, nil
	}
	q, args := d.TableExistsQuery(table)
	rows, err := c.QueryContext(ctx, q, args...)
	if err != nil {
		lg.Warn("table exists check failed", loggateway.StepID("data.startup.table_check"), loggateway.Err(err))
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

// indexExistsWithDialect checks whether an index exists using dialect-aware
// catalog queries (pragma_index_list for SQLite, pg_indexes for Postgres).
// Prefer this over sqliteIndexExists for new code.
//
// The query is sourced from Dialect.IndexExistsQuery and executed via the
// *ent.Client's QueryContext (shares the same connection pool as the Ent
// client, no separate *sql.DB required).
func indexExistsWithDialect(ctx context.Context, c *ent.Client, lg loggateway.Logger, table, indexName string, d Dialect) (bool, error) {
	indexName = strings.TrimSpace(indexName)
	if indexName == "" {
		return false, nil
	}
	q, args := d.IndexExistsQuery(table, indexName)
	rows, err := c.QueryContext(ctx, q, args...)
	if err != nil {
		lg.Warn("index exists check failed", loggateway.StepID("data.startup.index_check"), loggateway.Err(err))
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}
