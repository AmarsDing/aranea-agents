package service

import (
	"sync"
	"time"
)

// TypedSyncMap is a type-safe, TTL-aware wrapper around sync.Map.
// It replaces the raw sync.Map + timestampedEntry pattern used in ChatOrchestrator,
// eliminating double type assertions and providing automatic expiry.
//
// Stability:internal
type TypedSyncMap[K comparable, V any] struct {
	mu  sync.Map
	ttl time.Duration
}

type typedEntry[V any] struct {
	value     V
	createdAt time.Time
}

func NewTypedSyncMap[K comparable, V any](ttl time.Duration) *TypedSyncMap[K, V] {
	return &TypedSyncMap[K, V]{ttl: ttl}
}

func (m *TypedSyncMap[K, V]) Store(key K, value V) {
	m.mu.Store(key, typedEntry[V]{value: value, createdAt: time.Now()})
}

func (m *TypedSyncMap[K, V]) Load(key K) (V, bool) {
	v, ok := m.mu.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	entry, ok := v.(typedEntry[V])
	if !ok {
		var zero V
		return zero, false
	}
	if m.ttl > 0 && time.Since(entry.createdAt) > m.ttl {
		m.mu.Delete(key)
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (m *TypedSyncMap[K, V]) Delete(key K) {
	m.mu.Delete(key)
}

func (m *TypedSyncMap[K, V]) LoadOrStore(key K, value V) (V, bool) {
	existing, loaded := m.mu.LoadOrStore(key, typedEntry[V]{value: value, createdAt: time.Now()})
	if !loaded {
		return value, false
	}
	entry, ok := existing.(typedEntry[V])
	if !ok {
		var zero V
		return zero, false
	}
	return entry.value, true
}

// Sweep removes expired entries. Call periodically from a background goroutine.
func (m *TypedSyncMap[K, V]) Sweep() {
	now := time.Now()
	m.mu.Range(func(key, value any) bool {
		entry, ok := value.(typedEntry[V])
		if !ok || (m.ttl > 0 && now.Sub(entry.createdAt) > m.ttl) {
			m.mu.Delete(key)
		}
		return true
	})
}
