// Package intent runs an optional LLM pass to refine user goals before main ADK execution.
package intent

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	intentCacheDefaultCap = 256
	intentCacheDefaultTTL = 30 * time.Second
)

// CacheKey builds a deterministic cache key from the intent-pass inputs.
// History is intentionally excluded: the cache targets "same message within a
// short window" (e.g. S09 progressive追问), where history drift is negligible.
func CacheKey(sessionID, userText, provider, model, systemPrompt string) string {
	h := sha256.New()
	h.Write([]byte(sessionID))
	h.Write([]byte(userText))
	h.Write([]byte(provider))
	h.Write([]byte(model))
	h.Write([]byte(systemPrompt))
	return hex.EncodeToString(h.Sum(nil))
}

type cacheEntry struct {
	key       string
	result    RunResult
	createdAt time.Time
	element   *list.Element
}

// RunCache is a process-wide LRU+TTL cache for intent-pass results.
// It eliminates duplicate LLM calls when the same user message arrives in
// rapid succession (e.g. follow-up turns with identical or near-identical
// content, or client retry storms).
type RunCache struct {
	mu      sync.Mutex
	cap     int
	ttl     time.Duration
	items   map[string]*cacheEntry
	lruList *list.List
	lg      loggateway.Logger
}

// NewRunCache creates a new intent-pass result cache.
func NewRunCache(cap int, ttl time.Duration, lg loggateway.Logger) *RunCache {
	if cap <= 0 {
		cap = intentCacheDefaultCap
	}
	if ttl <= 0 {
		ttl = intentCacheDefaultTTL
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &RunCache{
		cap:     cap,
		ttl:     ttl,
		items:   make(map[string]*cacheEntry),
		lruList: list.New(),
		lg:      lg,
	}
}

// Get looks up a cached RunResult. Returns ok=false when missing or expired.
func (c *RunCache) Get(key string) (RunResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return RunResult{}, false
	}
	if time.Now().After(entry.createdAt.Add(c.ttl)) {
		c.evict(key)
		return RunResult{}, false
	}
	c.lruList.MoveToFront(entry.element)
	return entry.result, true
}

// Put stores a RunResult in the cache.
func (c *RunCache) Put(key string, result RunResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.lruList.MoveToFront(e.element)
		e.result = result
		e.createdAt = time.Now()
		return
	}
	for len(c.items) >= c.cap {
		c.evictExpired()
		if len(c.items) >= c.cap {
			back := c.lruList.Back()
			if back == nil {
				break
			}
			c.lg.Debug("intent-pass cache evict", loggateway.StepID("intent.cache_evict"))
			c.evict(back.Value.(*cacheEntry).key)
		}
	}
	entry := &cacheEntry{
		key:       key,
		result:    result,
		createdAt: time.Now(),
	}
	entry.element = c.lruList.PushFront(entry)
	c.items[key] = entry
}

// Len returns the current number of cached entries.
func (c *RunCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *RunCache) evict(key string) {
	if e, ok := c.items[key]; ok {
		c.lruList.Remove(e.element)
		delete(c.items, key)
	}
}

func (c *RunCache) evictExpired() {
	now := time.Now()
	for k, e := range c.items {
		if now.After(e.createdAt.Add(c.ttl)) {
			c.evict(k)
		}
	}
}

// globalCache is the process-wide singleton. It is lazily initialized on first
// use so that tests that never touch the cache pay zero cost.
var globalCache struct {
	once  sync.Once
	cache *RunCache
}

func globalRunCache() *RunCache {
	globalCache.once.Do(func() {
		globalCache.cache = NewRunCache(intentCacheDefaultCap, intentCacheDefaultTTL, nil)
	})
	return globalCache.cache
}
