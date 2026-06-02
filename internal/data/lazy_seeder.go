package data

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

type LazySeeder struct {
	once   sync.Once
	client *ent.Client
	fn     func(context.Context, *ent.Client) error
	err    atomic.Pointer[error]
	lg     loggateway.Logger
}

func NewLazySeeder(client *ent.Client, fn func(context.Context, *ent.Client) error, lg loggateway.Logger) *LazySeeder {
	return &LazySeeder{client: client, fn: fn, lg: lg}
}

func (s *LazySeeder) SeedIfNeeded(ctx context.Context) error {
	s.once.Do(func() {
		var e error
		func() {
			defer func() {
				if r := recover(); r != nil {
					e = fmt.Errorf("panic in lazy seeder: %v", r)
					s.lg.Warn("lazy seeder panicked", loggateway.StepID("data.seed.lazy"), loggateway.Err(e))
				}
			}()
			e = s.fn(ctx, s.client)
		}()
		s.err.Store(&e)
	})
	if p := s.err.Load(); p != nil && *p != nil {
		s.lg.Warn("seed step failed", loggateway.StepID("data.seed.lazy"), loggateway.Err(*p))
		return *p
	}
	return nil
}
