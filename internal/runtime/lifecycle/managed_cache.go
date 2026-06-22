package lifecycle

import (
	"sync"
	"time"

	"aranea-agents/pkg/safego"
)

// ManagedCache 是带 TTL 的并发安全缓存，实现 Closer 接口。
//
// 设计目标（A3）：替代无 TTL 的包级缓存单例（如 globalBuildCache）。
// 缓存项有过期时间，后台 goroutine 定期清理。注册到 LifecycleManager 后，
// 应用关闭时自动调用 Close 停止清理 goroutine。
//
// 与 ManagedMap 的区别：ManagedCache 专注于缓存语义（key-value + TTL），
// 不提供原子复合操作；ManagedMap 专注于状态跟踪语义（原子 load-modify-store）。
//
// 并发安全：所有方法均可并发调用。
type ManagedCache[K comparable, V any] struct {
	mu         sync.RWMutex
	items      map[K]cacheEntry[V]
	defaultTTL time.Duration
	maxSize    int
	stopOnce   sync.Once
	stopCh     chan struct{}
}

type cacheEntry[V any] struct {
	value     V
	expiresAt time.Time
}

// NewManagedCache 创建一个缓存。defaultTTL 为 0 表示永不过期。maxSize 为 0 表示无大小限制。
//
// 启用 TTL 或 maxSize 后，后台 goroutine 定期清理过期/超量 entry。
// 调用 Close() 停止后台 goroutine。
func NewManagedCache[K comparable, V any](defaultTTL time.Duration, maxSize int) *ManagedCache[K, V] {
	c := &ManagedCache[K, V]{
		items:      make(map[K]cacheEntry[V]),
		defaultTTL: defaultTTL,
		maxSize:    maxSize,
		stopCh:     make(chan struct{}),
	}
	if defaultTTL > 0 || maxSize > 0 {
		safego.GoBackground("managed_cache.cleanup", c.cleanupLoop)
	}
	return c
}

// Get 获取 key 对应的值。若 entry 已过期或不存在返回 false。
func (c *ManagedCache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		var zero V
		return zero, false
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		// 惰性删除
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set 存储 key=value，使用默认 TTL。
func (c *ManagedCache[K, V]) Set(key K, value V) {
	c.setWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL 存储 key=value，使用指定 TTL（覆盖默认 TTL）。ttl 为 0 表示永不过期。
func (c *ManagedCache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) {
	c.setWithTTL(key, value, ttl)
}

func (c *ManagedCache[K, V]) setWithTTL(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}
	c.items[key] = cacheEntry[V]{value: value, expiresAt: expiresAt}
	// 超量时淘汰最旧 entry（按 expiresAt 排序）
	if c.maxSize > 0 && len(c.items) > c.maxSize {
		c.evictOldestLocked()
	}
}

// Delete 删除 key。
func (c *ManagedCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Len 返回未过期 entry 数量。
func (c *ManagedCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Close 停止后台清理 goroutine。实现 Closer 接口。可多次调用。
func (c *ManagedCache[K, V]) Close() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
	return nil
}

// evictOldestLocked 淘汰最旧 entry。调用方必须持有写锁。
func (c *ManagedCache[K, V]) evictOldestLocked() {
	var oldestKey K
	var oldestExpiry time.Time
	first := true
	for k, e := range c.items {
		if first {
			oldestKey = k
			oldestExpiry = e.expiresAt
			first = false
			continue
		}
		// 永不过期的 entry（零值）视为最旧
		if e.expiresAt.Before(oldestExpiry) || (e.expiresAt.IsZero() && !oldestExpiry.IsZero()) {
			oldestKey = k
			oldestExpiry = e.expiresAt
		}
	}
	if !first {
		delete(c.items, oldestKey)
	}
}

// cleanupLoop 后台定期清理过期 entry。
func (c *ManagedCache[K, V]) cleanupLoop() {
	interval := c.defaultTTL / 4
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.purgeExpired()
		}
	}
}

// purgeExpired 删除所有过期 entry。
func (c *ManagedCache[K, V]) purgeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, e := range c.items {
		if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
			delete(c.items, k)
		}
	}
}
