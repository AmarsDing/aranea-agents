package data

import (
	"context"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// tableExistsWithDialect checks whether a table exists using dialect-aware
// catalog queries (sqlite_master for SQLite, information_schema for Postgres).
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
