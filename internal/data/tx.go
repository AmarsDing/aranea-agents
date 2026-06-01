package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

type txClientKey struct{}

func (d *Data) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if d == nil || d.entClient == nil {
		return fn(ctx)
	}
	tx, err := d.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := context.WithValue(ctx, txClientKey{}, tx.Client())
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (d *Data) clientFromCtx(ctx context.Context) *ent.Client {
	if c, ok := ctx.Value(txClientKey{}).(*ent.Client); ok {
		return c
	}
	return d.entClient
}

func txClientFromCtx(ctx context.Context) *ent.Client {
	if c, ok := ctx.Value(txClientKey{}).(*ent.Client); ok {
		return c
	}
	return nil
}
