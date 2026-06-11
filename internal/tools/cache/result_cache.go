package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const defaultCacheTTLSec = 300

type entry struct {
	result    any
	expiresAt time.Time
	accessedAt time.Time
}

type ResultCache struct {
	mu    sync.RWMutex
	items map[string]entry
	max   int
}

func NewResultCache(maxEntries int) *ResultCache {
	if maxEntries <= 0 {
		maxEntries = 512
	}
	return &ResultCache{items: make(map[string]entry, maxEntries/4), max: maxEntries}
}

func (c *ResultCache) key(toolName string, args []byte) string {
	h := sha256.Sum256(append([]byte(toolName+"\n"), args...))
	return fmt.Sprintf("%x", h)
}

func (c *ResultCache) Get(toolName string, args []byte) (any, bool) {
	if c == nil {
		return nil, false
	}
	k := c.key(toolName, args)

	// Fast path: read-only check under RLock.
	c.mu.RLock()
	e, ok := c.items[k]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	now := time.Now()
	if now.After(e.expiresAt) {
		// Slow path: expired entry needs deletion under write lock.
		c.mu.Lock()
		// Re-check after acquiring write lock (another goroutine may have evicted it).
		if e2, ok2 := c.items[k]; ok2 && now.After(e2.expiresAt) {
			delete(c.items, k)
		}
		c.mu.Unlock()
		return nil, false
	}
	// Update accessedAt under write lock for LRU tracking.
	c.mu.Lock()
	if e2, ok2 := c.items[k]; ok2 {
		e2.accessedAt = now
		c.items[k] = e2
	}
	c.mu.Unlock()
	return e.result, true
}

func (c *ResultCache) Put(toolName string, args []byte, result any, ttl time.Duration) {
	if c == nil || ttl <= 0 || result == nil {
		return
	}
	k := c.key(toolName, args)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		c.evictExpiredLocked(now)
		if len(c.items) >= c.max {
			c.evictLRULocked()
		}
	}
	c.items[k] = entry{result: result, expiresAt: now.Add(ttl), accessedAt: now}
}

func (c *ResultCache) evictExpiredLocked(now time.Time) {
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
		}
	}
}

func (c *ResultCache) evictLRULocked() {
	var oldestKey string
	var oldest time.Time
	for k, e := range c.items {
		if oldestKey == "" || e.accessedAt.Before(oldest) {
			oldestKey = k
			oldest = e.accessedAt
		}
	}
	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

// CachePolicy describes whether a catalog tool may cache results.
type CachePolicy struct {
	Enabled bool
	TTL     time.Duration
}

// PolicyFromToolJSON reads cache_enabled / cache_ttl_sec from metadata_json then config_json.
func PolicyFromToolJSON(metadataJSON, configJSON string) CachePolicy {
	for _, raw := range []string{metadataJSON, configJSON} {
		if p := policyFromObject(raw); p.Enabled {
			return p
		}
	}
	return CachePolicy{}
}

func policyFromObject(raw string) CachePolicy {
	raw = trimJSON(raw)
	if raw == "" {
		return CachePolicy{}
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return CachePolicy{}
	}
	enabled := boolish(m["cache_enabled"])
	ttlSec := numberish(m["cache_ttl_sec"])
	if !enabled && ttlSec <= 0 {
		return CachePolicy{}
	}
	if ttlSec <= 0 {
		ttlSec = defaultCacheTTLSec
	}
	if !enabled {
		enabled = ttlSec > 0
	}
	return CachePolicy{Enabled: enabled, TTL: time.Duration(ttlSec) * time.Second}
}

func trimJSON(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n') {
		s = s[1:]
	}
	return s
}

func boolish(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	default:
		return false
	}
}

func numberish(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

var (
	globalResultCache = NewResultCache(512)
	globalMu          sync.RWMutex
)

func Global() *ResultCache {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalResultCache
}

func SetGlobal(c *ResultCache) {
	if c != nil {
		globalMu.Lock()
		globalResultCache = c
		globalMu.Unlock()
	}
}
