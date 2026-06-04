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
	// Use a detached context for transaction begin, body, and commit so that
	// HTTP-request cancellation (client disconnect / axios timeout) does not
	// abort in-flight SQLite operations mid-transaction.  The original ctx is
	// checked before commit; if the caller cancelled we roll back instead.
	detached := context.Background()
	tx, err := d.entClient.Tx(detached)
	if err != nil {
		d.lg.Error("transaction begin failed", loggateway.StepID("data.tx"), loggateway.Err(err))
		return err
	}
	// Carry the tx reference on the detached ctx so that nested calls detect
	// the in-progress transaction and reuse it.
	txCtx := context.WithValue(detached, txClientKey{}, tx)
	txCtx = context.WithValue(txCtx, rawTxKey{}, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		d.lg.Warn("transaction rolled back", loggateway.StepID("data.tx"), loggateway.Err(err))
		return err
	}
	// If the caller's context was cancelled while the transaction was running,
	// roll back instead of committing a result nobody will read.
	if ctx.Err() != nil {
		_ = tx.Rollback()
		d.lg.Warn("transaction rolled back (caller context cancelled)", loggateway.StepID("data.tx"), loggateway.Err(ctx.Err()))
		return ctx.Err()
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
