package data

import (
	"context"

	"aranea-agents/internal/biz"
)

type seedVersionRepo struct {
	data *Data
}

var _ biz.SeedVersionRepo = (*seedVersionRepo)(nil)

func NewSeedVersionRepo(d *Data) biz.SeedVersionRepo {
	return &seedVersionRepo{data: d}
}

func (r *seedVersionRepo) IsApplied(ctx context.Context, version int) (bool, error) {
	applied, err := IsSeedApplied(ctx, r.data.RW().Read(ctx), version, r.data.lg)
	if err != nil {
		return false, entErrToBizErr(err, "SEED_VERSION")
	}
	return applied, nil
}

func (r *seedVersionRepo) MarkApplied(ctx context.Context, version int, name string) error {
	err := MarkSeedApplied(ctx, r.data.RW().Write(ctx), r.data.Dialect(), version, name, r.data.lg)
	return entErrToBizErr(err, "SEED_VERSION")
}
