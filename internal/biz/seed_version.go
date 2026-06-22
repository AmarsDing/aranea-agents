package biz

import "context"

type SeedVersionRepo interface {
	IsApplied(ctx context.Context, version int) (bool, error)
	MarkApplied(ctx context.Context, version int, name string) error
}
