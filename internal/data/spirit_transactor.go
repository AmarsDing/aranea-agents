package data

import (
	"context"

	"aranea-agents/internal/biz"
)

// spiritTransactorAdapter adapts Data.ExecInTx to biz.SpiritTransactor.
type spiritTransactorAdapter struct {
	data *Data
}

// NewSpiritTransactor creates a biz.SpiritTransactor backed by Data.ExecInTx.
func NewSpiritTransactor(d *Data) biz.SpiritTransactor {
	if d == nil {
		return nil
	}
	return &spiritTransactorAdapter{data: d}
}

func (a *spiritTransactorAdapter) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return a.data.ExecInTx(ctx, fn)
}
