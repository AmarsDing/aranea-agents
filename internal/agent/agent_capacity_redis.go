package agent

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis concurrent-slot scripts for durable Capacity hard gate (DEV-03).
// Keys expire so a crashed process cannot permanently pin slots.
const (
	capacityKeyPrefix     = "aranea:agent:capacity:"
	capacitySlotTTL       = 2 * time.Hour
	capacityIncrLua       = `
local key = KEYS[1]
local n = redis.call('INCR', key)
redis.call('PEXPIRE', key, ARGV[1])
return n
`
	capacityTryAcquireLua = `
local key = KEYS[1]
local max = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local cur = tonumber(redis.call('GET', key) or '0')
if max > 0 and cur >= max then
  return 0
end
local n = redis.call('INCR', key)
redis.call('PEXPIRE', key, ttl)
return n
`
	capacityDecrLua = `
local key = KEYS[1]
local cur = tonumber(redis.call('GET', key) or '0')
if cur <= 1 then
  redis.call('DEL', key)
  return 0
end
return redis.call('DECR', key)
`
)

// redisCapacityBackend dual-writes capacity slots so Allocator Snapshot and
// PlanExecutor Acquire/Release stay coherent across admin replicas when Redis
// is configured. Nil client disables the backend (process-local soft gate only).
type redisCapacityBackend struct {
	client     *redis.Client
	incrScr    *redis.Script
	tryScr     *redis.Script
	decrScr    *redis.Script
	ttlMS      int64
}

func newRedisCapacityBackend(client *redis.Client) *redisCapacityBackend {
	if client == nil {
		return nil
	}
	return &redisCapacityBackend{
		client:  client,
		incrScr: redis.NewScript(capacityIncrLua),
		tryScr:  redis.NewScript(capacityTryAcquireLua),
		decrScr: redis.NewScript(capacityDecrLua),
		ttlMS:   capacitySlotTTL.Milliseconds(),
	}
}

func capacityRedisKey(agentKey string) string {
	return capacityKeyPrefix + agentKey
}

func (b *redisCapacityBackend) load(ctx context.Context, agentKey string) int {
	if b == nil || b.client == nil || agentKey == "" {
		return 0
	}
	s, err := b.client.Get(ctx, capacityRedisKey(agentKey)).Result()
	if err != nil || s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (b *redisCapacityBackend) acquire(ctx context.Context, agentKeys ...string) {
	if b == nil || b.client == nil {
		return
	}
	for _, k := range agentKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		_, _ = b.incrScr.Run(ctx, b.client, []string{capacityRedisKey(k)}, b.ttlMS).Result()
	}
}

// tryAcquire increments only when under capacity. capacity<=0 means unlimited
// (always increments). Returns false when the hard gate denies the slot.
func (b *redisCapacityBackend) tryAcquire(ctx context.Context, agentKey string, capacity int) bool {
	if b == nil || b.client == nil || agentKey == "" {
		return true
	}
	if capacity <= 0 {
		b.acquire(ctx, agentKey)
		return true
	}
	res, err := b.tryScr.Run(ctx, b.client, []string{capacityRedisKey(agentKey)}, capacity, b.ttlMS).Result()
	if err != nil {
		// Fail open to process-local soft gate on Redis errors.
		return true
	}
	n, ok := res.(int64)
	return ok && n > 0
}

func (b *redisCapacityBackend) release(ctx context.Context, agentKeys ...string) {
	if b == nil || b.client == nil {
		return
	}
	for _, k := range agentKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		_, _ = b.decrScr.Run(ctx, b.client, []string{capacityRedisKey(k)}).Result()
	}
}
