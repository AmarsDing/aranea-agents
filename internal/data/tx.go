package data

import (
	"context"
	"database/sql"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type txClientKey struct{}

type rawTxKey struct{}

func (d *Data) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if d == nil || d.entClient == nil {
		return fn(ctx)
	}
	if _, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
		return fn(ctx)
	}
	tx, err := d.entClient.Tx(ctx)
	if err != nil {
		d.lg.Error("transaction begin failed", loggateway.StepID("data.tx"), loggateway.Err(err))
		return err
	}
	txCtx := context.WithValue(ctx, txClientKey{}, tx)
	txCtx = context.WithValue(txCtx, rawTxKey{}, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		d.lg.Warn("transaction rolled back", loggateway.StepID("data.tx"), loggateway.Err(err))
		return err
	}
	if err := tx.Commit(); err != nil {
		d.lg.Error("transaction commit failed", loggateway.StepID("data.tx"), loggateway.Err(err))
		return err
	}
	return nil
}

func EntClientFromCtx(ctx context.Context, fallback *ent.Client) *ent.Client {
	if tx, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return fallback
}

func (d *Data) clientFromCtx(ctx context.Context) *ent.Client {
	return EntClientFromCtx(ctx, d.entClient)
}

func (d *Data) ClientFromCtx(ctx context.Context) *ent.Client {
	return EntClientFromCtx(ctx, d.entClient)
}

func txClientFromCtx(ctx context.Context) *ent.Client {
	if tx, ok := ctx.Value(txClientKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func TxExecerFromCtx(ctx context.Context, fallback *sql.DB) execer {
	if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return fallback
}

func (d *Data) PostgresExecInTx(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error) error {
	if d == nil {
		return kerrors.InternalServer("DATA", "data not initialized")
	}
	pg := d.Postgres()
	if pg == nil {
		return kerrors.InternalServer("DATA", "postgres not configured")
	}
	tx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		d.lg.Error("postgres transaction begin failed", loggateway.StepID("data.pg_tx"), loggateway.Err(err))
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		d.lg.Error("postgres transaction commit failed", loggateway.StepID("data.pg_tx"), loggateway.Err(err))
		return err
	}
	return nil
}

func queryRowScan(ctx context.Context, e execer, query string, args []any, dest ...any) error {
	rows, err := e.QueryContext(ctx, query, args...)
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
