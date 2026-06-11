package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/durationpb"
)

// RedisClient wraps the underlying redis.Client to allow nil-safe usage and to
// keep a single shared connection pool for the process. Methods on the
// embedded *redis.Client are reachable via promotion; callers should treat
// nil-RedisClient as "feature disabled" and fall back to the in-process
// implementation.
type RedisClient struct {
	*redis.Client
}

// IsEnabled reports whether the Redis client is connected. False values mean
// "feature is not available; consumers must use the in-process fallback".
func (r *RedisClient) IsEnabled() bool { return r != nil && r.Client != nil }

// NewRedisClient constructs a Redis client from the bootstrap configuration.
// The returned *RedisClient is nil when Redis is not configured (empty addr)
// or when the initial PING fails. The caller is expected to log the disabled
// state and use the in-process fallback.
func NewRedisClient(c *conf.Data, lg loggateway.Logger) *RedisClient {
	if c == nil || c.Redis == nil || strings.TrimSpace(c.Redis.Addr) == "" {
		return nil
	}
	rdb := redis.NewClient(&redis.Options{
		Network:      pickRedisNetwork(c.Redis.Network),
		Addr:         strings.TrimSpace(c.Redis.Addr),
		ReadTimeout:  pickDuration(c.Redis.ReadTimeout, 2*time.Second),
		WriteTimeout: pickDuration(c.Redis.WriteTimeout, 2*time.Second),
		DialTimeout:  2 * time.Second,
		PoolSize:     16,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		if lg != nil {
			lg.Warn("Redis unavailable; distributed features (e.g. A2A limiter) will fall back to in-process backend",
				loggateway.StepID("redis.ping"),
				loggateway.Str("addr", c.Redis.Addr),
				loggateway.Err(err),
			)
		}
		_ = rdb.Close()
		return nil
	}
	return &RedisClient{Client: rdb}
}

func pickRedisNetwork(network string) string {
	if strings.TrimSpace(network) == "" {
		return "tcp"
	}
	return network
}

func pickDuration(d *durationpb.Duration, fallback time.Duration) time.Duration {
	if d == nil {
		return fallback
	}
	v := d.AsDuration()
	if v <= 0 {
		return fallback
	}
	return v
}
