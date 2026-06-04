package data

import (
	"context"
	"database/sql"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type txClientKey struct{}

type rawTxKey struct{}

func (d *Data) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	// #region debug-point data.tx.trace
	// DEBUG ONLY: timing trace to identify which step inside ExecInTx hangs.
	// (uses Info because smoke.yaml logging.level=info filters Debug)
	t0 := time.Now()
	tl := d.lg.With(loggateway.StepID("data.tx.trace"))
	tl.Info("enter ExecInTx", loggateway.Bool("nested", ctx.Value(txClientKey{}) != nil))
	defer func() { tl.Info("exit ExecInTx", loggateway.Duration(time.Since(t0).Milliseconds())) }()
	// #endregion debug-point
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
	//
	// 2026-06-04 fix for agent-save-timeout: the detached context previously
	// had no deadline, so any single blocking SQL statement would hold the
	// write connection forever and starve every other writer.  The 30s
	// hard cap forces the tx to surface as a timeout error and release the
	// connection back to the pool.
	detached, detachedCancel := context.WithTimeout(context.Background(), 30*time.Second)
	// #region debug-point data.tx.trace
	tl.Info("before d.entClient.Tx(detached)", loggateway.Duration(time.Since(t0).Milliseconds()))
	// #endregion debug-point
	tx, err := d.entClient.Tx(detached)
	// #region debug-point data.tx.trace
	tl.Info("after d.entClient.Tx(detached)", loggateway.Duration(time.Since(t0).Milliseconds()), loggateway.Err(err))
	// #endregion debug-point
	if err != nil {
		detachedCancel()
		d.lg.Error("transaction begin failed", loggateway.StepID("data.tx"), loggateway.Err(err))
		return err
	}
	// Carry the tx reference on the detached ctx so that nested calls detect
	// the in-progress transaction and reuse it.
	txCtx := context.WithValue(detached, txClientKey{}, tx)
	txCtx = context.WithValue(txCtx, rawTxKey{}, tx)
	// #region debug-point data.tx.trace
	tl.Info("before fn(txCtx)", loggateway.Duration(time.Since(t0).Milliseconds()))
	// #endregion debug-point
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		d.lg.Warn("transaction rolled back", loggateway.StepID("data.tx"), loggateway.Err(err))
		return err
	}
	// #region debug-point data.tx.trace
	tl.Info("after fn(txCtx) - caller ctx err", loggateway.Duration(time.Since(t0).Milliseconds()), loggateway.Err(ctx.Err()))
	// #endregion debug-point
	// If the caller's context was cancelled while the transaction was running,
	// roll back instead of committing a result nobody will read.
	if ctx.Err() != nil {
		_ = tx.Rollback()
		detachedCancel()
		d.lg.Warn("transaction rolled back (caller context cancelled)", loggateway.StepID("data.tx"), loggateway.Err(ctx.Err()))
		return ctx.Err()
	}
	// #region debug-point data.tx.trace
	tl.Info("before tx.Commit", loggateway.Duration(time.Since(t0).Milliseconds()))
	// #endregion debug-point
	if err := tx.Commit(); err != nil {
		detachedCancel()
		d.lg.Error("transaction commit failed", loggateway.StepID("data.tx"), loggateway.Err(err))
		return err
	}
	detachedCancel()
	// #region debug-point data.tx.trace
	tl.Info("after tx.Commit", loggateway.Duration(time.Since(t0).Milliseconds()))
	// #endregion debug-point
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
