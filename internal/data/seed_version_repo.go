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
	return IsSeedApplied(ctx, r.data.Ent(), version, r.data.lg)
}

func (r *seedVersionRepo) MarkApplied(ctx context.Context, version int, name string) error {
	return MarkSeedApplied(ctx, r.data.Ent(), version, name, r.data.lg)
}
