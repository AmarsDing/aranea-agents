package data

import (
	"context"
	"database/sql"
	"errors"

	"aranea-agents/internal/data/ent"
)

// entQueryRowScan runs `query` expecting at most one row, using Ent Client’s promoted
// QueryContext（与 entClient 共用同一 dialect driver / 连接池；无需第二个 sql.Open）.
func entQueryRowScan(client *ent.Client, ctx context.Context, query string, args []any, dest ...any) error {
	if client == nil {
		return errors.New("ent client is nil")
	}
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return sql.ErrNoRows
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	return rows.Err()
}
