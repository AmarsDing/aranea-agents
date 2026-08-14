// Package lifecycle 提供统一的生命周期管理抽象：LifecycleManager、ManagedMap、
// DeadLetterQueue。
//
// 设计目标（对应 docs/reports/2026-06-18-review-issues-and-solutions.md 方案 A）：
//   - ManagedMap：替代裸 sync.Map + 手写 mutex，提供原子 LoadOrStore / UpdateOrStore，
//     内置可选 TTL 清理与终态清理，根治 TOCTOU 与 map 无限增长问题（A1/A5/A6）。
//   - LifecycleManager：统一注册/销毁进程级资源（A3）。
//   - DeadLetterQueue：失败消息死信缓冲（A4）。
package lifecycle

import (
	"sync"
	"time"

	"aranea-agents/pkg/safego"
)

// ManagedMap 是带内置锁的并发安全 map，提供原子复合操作与可选 TTL 清理。
//
// 适用场景（替代裸 sync.Map + 手写 mutex）：
//   - 需要原子 load-modify-store 的场景（A1 RunRegistry TOCTOU）
//   - 按 key 跟踪状态且需要在终态清理的场景（A5 attemptCount、A6 addedEdges）
//   - 需要防止长生命周期进程内存缓慢增长的场景
//
// 不适用场景：
//   - 纯读多写少且无复合操作 → 直接用 sync.Map
//   - 高频路径且无需 TTL → mutex 开销可能不必要
//
// 并发安全：所有方法均可并发调用。
type ManagedMap[K comparable, V any] struct {
	mu       sync.Mutex
	items    map[K]managedEntry[V]
	ttl      time.Duration // 0 表示无 TTL
	stopOnce sync.Once
	stopCh   chan struct{}
}

// managedEntry 包装值与过期时间。
type managedEntry[V any] struct {
	value     V
	expiresAt time.Time // 零值表示不过期
}

// NewManagedMap 创建一个 ManagedMap。ttl 为 0 表示不启用 TTL 清理。
//
// 启用 TTL 后，后台 goroutine 定期扫描并删除过期 entry。
// 调用 Close() 停止后台 goroutine。
func NewManagedMap[K comparable, V any](ttl time.Duration) *ManagedMap[K, V] {
	m := &ManagedMap[K, V]{
		items:  make(map[K]managedEntry[V]),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	if ttl > 0 {
		safego.GoBackground("managed_map.cleanup", m.cleanupLoop)
	}
	return m
}

// Load 返回 key 对应的值。若 entry 已过期或不存在返回 false。
func (m *ManagedMap[K, V]) Load(key K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		// 惰性删除过期 entry
		delete(m.items, key)
		var zero V
		return zero, false
	}
	return e.value, true
}

// Store 存储 key=value。若启用 TTL，按 m.ttl 设置过期时间。
func (m *ManagedMap[K, V]) Store(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeLocked(key, value)
}

func (m *ManagedMap[K, V]) storeLocked(key K, value V) {
	var expiresAt time.Time
	if m.ttl > 0 {
		expiresAt = time.Now().Add(m.ttl)
	}
	m.items[key] = managedEntry[V]{value: value, expiresAt: expiresAt}
}

// Delete 删除 key。若 key 不存在则无操作。
func (m *ManagedMap[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
}

// DeleteIf 原子地删除 key：仅当 pred(value) 为 true 时删除。
// 返回是否实际删除。用于 CAS 清理（如 RunRegistry.Finish 按 runID 匹配）。
func (m *ManagedMap[K, V]) DeleteIf(key K, pred func(V) bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if !ok {
		return false
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		delete(m.items, key)
		return false
	}
	if pred == nil || !pred(e.value) {
		return false
	}
	delete(m.items, key)
	return true
}

// LoadOrStore 原子地加载或存储：若 key 存在返回 (现有值, true)；否则存储 value 并返回 (value, false)。
//
// 此方法替代裸 sync.Map 的 load+store 复合操作，根治 TOCTOU 窗口。
func (m *ManagedMap[K, V]) LoadOrStore(key K, value V) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.items[key]; ok {
		if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
			// 过期则视为不存在，存储新值
			m.storeLocked(key, value)
			return value, false
		}
		return e.value, true
	}
	m.storeLocked(key, value)
	return value, false
}

// UpdateOrStore 原子地加载现有 entry（若有），应用 update 回调派生新值，再存储。
//
// update 回调接收 (existing, ok)，其中 ok 为 false 表示无现有 entry。
// 回调必须无副作用——在 mutex 下被调用，可能因 TTL 过期被多次调用。
//
// 此方法替代手写 load-modify-store 序列，根治并发覆盖问题（A1 RunRegistry）。
func (m *ManagedMap[K, V]) UpdateOrStore(key K, update func(existing V, ok bool) V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if ok && !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		// 过期则视为不存在
		ok = false
	}
	newValue := update(e.value, ok)
	m.storeLocked(key, newValue)
}

// UpdateOrStoreWithResult 原子地加载现有 entry（若有），应用 update 回调派生新值与结果，再存储。
//
// update 回调返回 (newValue, result)，其中 result 透传给调用方（例如表示是否允许操作）。
// 适用于需要原子 check-and-increment 且要根据结果决策的场景（A5 attemptCount）。
//
// update 回调必须无副作用——在 mutex 下被调用。
func (m *ManagedMap[K, V]) UpdateOrStoreWithResult(key K, update func(existing V, ok bool) (V, bool)) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if ok && !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		// 过期则视为不存在
		ok = false
	}
	newValue, result := update(e.value, ok)
	m.storeLocked(key, newValue)
	return newValue, result
}

// LoadAndDelete 原子地加载并删除。若 key 不存在返回 false。
func (m *ManagedMap[K, V]) LoadAndDelete(key K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	delete(m.items, key)
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		// 已过期，视为不存在
		var zero V
		return zero, false
	}
	return e.value, true
}

// Range 遍历所有未过期的 entry。f 返回 false 停止遍历。
//
// 注意：Range 在 mutex 下执行，f 内禁止调用 m 的其他方法（会死锁）。
func (m *ManagedMap[K, V]) Range(f func(key K, value V) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, e := range m.items {
		if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
			delete(m.items, k)
			continue
		}
		if !f(k, e.value) {
			break
		}
	}
}

// Len 返回未过期 entry 数量。
func (m *ManagedMap[K, V]) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	count := 0
	for k, e := range m.items {
		if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
			delete(m.items, k)
			continue
		}
		count++
	}
	return count
}

// Close 停止后台 TTL 清理 goroutine。可多次调用。
func (m *ManagedMap[K, V]) Close() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

// cleanupLoop 后台定期清理过期 entry。仅在 ttl > 0 时启动。
func (m *ManagedMap[K, V]) cleanupLoop() {
	// 清理间隔为 TTL 的 1/4，最小 1 分钟，避免高频路径开销。
	interval := m.ttl / 4
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.purgeExpired()
		}
	}
}

// purgeExpired 删除所有过期 entry。
func (m *ManagedMap[K, V]) purgeExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, e := range m.items {
		if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
			delete(m.items, k)
		}
	}
}
