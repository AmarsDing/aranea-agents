package a2a

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowLuaScript implements an atomic sliding-window rate limit using
// a Redis sorted set. The script:
//  1. Removes entries older than (now - window).
//  2. Counts remaining entries.
//  3. If count >= max, returns 0 (denied) without adding.
//  4. Otherwise adds the new request and refreshes the key TTL.
//
// The "member" argument guarantees ZSET uniqueness when two requests arrive
// in the same millisecond, preventing the older ZADD from overwriting the
// newer one.
const slidingWindowLuaScript = `
local key      = KEYS[1]
local now      = tonumber(ARGV[1])
local window   = tonumber(ARGV[2])
local max      = tonumber(ARGV[3])
local ttl      = tonumber(ARGV[4])
local member   = ARGV[5]

redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
local count = redis.call('ZCARD', key)
if count >= max then
  return 0
end
redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, ttl)
return 1
`

// redisSlidingWindowLimiter is a distributed A2A rate limiter using a Redis
// sorted set and an atomic Lua script. It implements Limiter and is safe
// for multi-pod deployments because all read-modify-write happens server-side.
type redisSlidingWindowLimiter struct {
	client *redis.Client
	cfg    LimiterConfig
	script *redis.Script
}

// NewRedisSlidingWindowLimiter constructs a Redis-backed limiter. A nil
// client yields a non-functional limiter whose Allow returns (true, nil),
// preserving the in-process "always allow" semantics for misconfiguration.
func NewRedisSlidingWindowLimiter(client *redis.Client, cfg LimiterConfig) *redisSlidingWindowLimiter {
	cfg = cfg.applyDefaults()
	return &redisSlidingWindowLimiter{
		client: client,
		cfg:    cfg,
		script: redis.NewScript(slidingWindowLuaScript),
	}
}

func (l *redisSlidingWindowLimiter) Allow(ctx context.Context, caller, callee string) (bool, error) {
	if l == nil || l.client == nil {
		return true, nil
	}
	key := l.cfg.KeyPrefix + caller + "->" + callee
	nowMs := time.Now().UnixMilli()
	windowMs := l.cfg.WindowSize.Milliseconds()
	ttlMs := windowMs * 2
	if ttlMs < time.Second.Milliseconds() {
		ttlMs = time.Second.Milliseconds()
	}
	member, err := uniqueMember(nowMs)
	if err != nil {
		return false, fmt.Errorf("a2a limiter: generate member: %w", err)
	}

	res, err := l.script.Run(ctx, l.client, []string{key},
		nowMs, windowMs, l.cfg.MaxInvokes, ttlMs, member,
	).Result()
	if err != nil {
		return false, fmt.Errorf("a2a limiter: redis eval: %w", err)
	}
	n, ok := res.(int64)
	if !ok {
		return false, fmt.Errorf("a2a limiter: unexpected redis return type %T", res)
	}
	return n == 1, nil
}

// uniqueMember returns a string that combines the millisecond timestamp with
// a 4-byte random suffix. The random suffix prevents ZADD overwrites when
// two requests fall into the same millisecond on the same pod.
func uniqueMember(nowMs int64) (string, error) {
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	return strconv.FormatInt(nowMs, 10) + "-" + hex.EncodeToString(rnd[:]), nil
}
