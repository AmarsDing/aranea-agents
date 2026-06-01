package compress

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	compressCacheDefaultCap = 256
	compressCacheDefaultTTL = 10 * time.Minute
)

type compressCtxKeyType struct{}

var compressCtxSessionIDKey = compressCtxKeyType{}

func ContextWithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, compressCtxSessionIDKey, sessionID)
}

func SessionIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(compressCtxSessionIDKey).(string); ok {
		return v
	}
	return ""
}

func CompressCacheKey(sessionID string, req Request) string {
	h := sha256.New()
	h.Write([]byte(sessionID))
	h.Write([]byte(req.Transcript))
	h.Write([]byte(req.PriorSummary))
	h.Write([]byte(req.Provider))
	h.Write([]byte(req.Model))
	h.Write([]byte(req.SystemPrompt))
	h.Write([]byte(PromptVersion))
	return hex.EncodeToString(h.Sum(nil))
}

type compressCacheEntry struct {
	key       string
	result    Result
	createdAt time.Time
	element   *list.Element
}

type CompressCache struct {
	mu      sync.Mutex
	cap     int
	ttl     time.Duration
	items   map[string]*compressCacheEntry
	lruList *list.List
	lg      loggateway.Logger
}

func NewCompressCache(cap int, ttl time.Duration, lg loggateway.Logger) *CompressCache {
	if cap <= 0 {
		cap = compressCacheDefaultCap
	}
	if ttl <= 0 {
		ttl = compressCacheDefaultTTL
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &CompressCache{
		cap:     cap,
		ttl:     ttl,
		items:   make(map[string]*compressCacheEntry),
		lruList: list.New(),
		lg:      lg,
	}
}

func (c *CompressCache) Get(key string) (Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return Result{}, false
	}
	if time.Now().After(entry.createdAt.Add(c.ttl)) {
		c.evict(key)
		return Result{}, false
	}
	c.lruList.MoveToFront(entry.element)
	return entry.result, true
}

func (c *CompressCache) Put(key string, result Result) {
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
			c.lg.Debug("L0 压缩缓存淘汰", loggateway.StepID("compress.cache_evict"))
			c.evict(back.Value.(*compressCacheEntry).key)
		}
	}
	entry := &compressCacheEntry{
		key:       key,
		result:    result,
		createdAt: time.Now(),
	}
	entry.element = c.lruList.PushFront(entry)
	c.items[key] = entry
}

func (c *CompressCache) evict(key string) {
	if e, ok := c.items[key]; ok {
		c.lruList.Remove(e.element)
		delete(c.items, key)
	}
}

func (c *CompressCache) evictExpired() {
	now := time.Now()
	for k, e := range c.items {
		if now.After(e.createdAt.Add(c.ttl)) {
			c.evict(k)
		}
	}
}

func (c *CompressCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

type CachingCompressor struct {
	inner Compressor
	cache *CompressCache
	lg    loggateway.Logger
}

func NewCachingCompressor(inner Compressor, cache *CompressCache, lg loggateway.Logger) *CachingCompressor {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &CachingCompressor{inner: inner, cache: cache, lg: lg}
}

var _ Compressor = (*CachingCompressor)(nil)

func (c *CachingCompressor) Compress(ctx context.Context, req Request) (Result, error) {
	sessionID := SessionIDFromCtx(ctx)
	key := CompressCacheKey(sessionID, req)
	if hit, ok := c.cache.Get(key); ok {
		c.lg.Info("L0 压缩缓存命中", loggateway.StepID("compress.cache_hit"), loggateway.SessionID(sessionID))
		return hit, nil
	}
	c.lg.Info("L0 压缩缓存未命中", loggateway.StepID("compress.cache_miss"), loggateway.SessionID(sessionID))
	result, err := c.inner.Compress(ctx, req)
	if err != nil {
		return result, err
	}
	c.cache.Put(key, result)
	return result, nil
}

func (c *CachingCompressor) CompressWithCacheHit(ctx context.Context, req Request) (Result, bool, error) {
	sessionID := SessionIDFromCtx(ctx)
	key := CompressCacheKey(sessionID, req)
	if hit, ok := c.cache.Get(key); ok {
		c.lg.Info("L0 压缩缓存命中", loggateway.StepID("compress.cache_hit"), loggateway.SessionID(sessionID))
		return hit, true, nil
	}
	c.lg.Info("L0 压缩缓存未命中", loggateway.StepID("compress.cache_miss"), loggateway.SessionID(sessionID))
	result, err := c.inner.Compress(ctx, req)
	if err != nil {
		return result, false, err
	}
	c.cache.Put(key, result)
	return result, false, nil
}
