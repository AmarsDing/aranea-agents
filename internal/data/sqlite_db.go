package data

import (
	"context"
	"errors"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"
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
		// rows.Next 返回 false 时必须检查 rows.Err()，否则会隐藏真实错误
		// （例如 PostgreSQL 的语法错误/类型错误在迭代时才暴露），
		// 错误地返回 NOT_FOUND。
		if rerr := rows.Err(); rerr != nil {
			return rerr
		}
		return apierror.NotFound(apierror.DomainData, "not found")
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	return rows.Err()
}
