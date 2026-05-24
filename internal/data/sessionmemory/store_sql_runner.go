package sessionmemory

import (
	"context"
	"database/sql"
)

// sqlRunner is the minimal DB surface used by sessionmemory writes (ent.Client or tx.Client()).
type sqlRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}
