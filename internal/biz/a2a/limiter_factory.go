package a2a

import (
	"aranea-agents/pkg/loggateway"

	"github.com/redis/go-redis/v9"
)

// NewLimiter returns the most appropriate Limiter given the runtime
// environment. When `client` is non-nil the Redis-backed implementation is
// returned; otherwise an in-memory limiter is used and a warning is logged so
// that operators can detect accidental single-pod deployments.
//
// Callers should always go through this factory rather than constructing a
// concrete limiter — it keeps the storage decision at the composition root
// and prevents accidental coupling to a specific backend.
func NewLimiter(cfg LimiterConfig, client *redis.Client, lg loggateway.Logger) Limiter {
	cfg = cfg.applyDefaults()
	if client != nil {
		if lg != nil {
			lg.Info("A2A limiter using Redis distributed backend",
				loggateway.StepID("a2a.limiter.init"),
				loggateway.Str("window", cfg.WindowSize.String()),
				loggateway.Int("max", cfg.MaxInvokes),
			)
		}
		return NewRedisSlidingWindowLimiter(client, cfg)
	}
	if lg != nil {
		lg.Warn("A2A limiter using in-memory backend; not safe for multi-pod deployments. Configure data.redis to enable distributed limiting.",
			loggateway.StepID("a2a.limiter.init"),
			loggateway.Str("window", cfg.WindowSize.String()),
			loggateway.Int("max", cfg.MaxInvokes),
		)
	}
	return NewMemorySlidingWindowLimiter(cfg)
}
