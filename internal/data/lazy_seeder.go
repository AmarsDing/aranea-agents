package data

import (
	"context"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/data/ent"
)

type LazySeeder struct {
	once   sync.Once
	client *ent.Client
	fn     func(context.Context, *ent.Client) error
	err    atomic.Pointer[error]
}

func NewLazySeeder(client *ent.Client, fn func(context.Context, *ent.Client) error) *LazySeeder {
	return &LazySeeder{client: client, fn: fn}
}

func (s *LazySeeder) SeedIfNeeded(ctx context.Context) error {
	s.once.Do(func() {
		e := s.fn(ctx, s.client)
		s.err.Store(&e)
	})
	if p := s.err.Load(); p != nil {
		return *p
	}
	return nil
}
