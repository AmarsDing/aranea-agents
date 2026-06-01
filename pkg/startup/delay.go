package startup

import (
	"context"
	"math/rand"
	"time"

	"aranea-agents/pkg/safego"
)

type DelayConfig struct {
	InitialDelay time.Duration
	Jitter       time.Duration
}

func StartDelayed(ctx context.Context, name string, cfg DelayConfig, fn func()) {
	delay := cfg.InitialDelay
	if cfg.Jitter > 0 {
		delay += time.Duration(rand.Int63n(int64(cfg.Jitter)))
	}
	if delay <= 0 {
		fn()
		return
	}
	safego.Go(ctx, "startup.delay."+name, func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			fn()
		}
	})
}
