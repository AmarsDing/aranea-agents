package biz

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/safego"
)

// ---------------------------------------------------------------------------
// SessionLockManager — in-process per-session mutual exclusion.
//
// Moved from internal/service (TECH-DEBT(BL8) resolution). This is concurrent
// control infrastructure, not transport glue, so it belongs in biz where any
// usecase (ChatUsecase, TeamUsecase, future workers) can consume it via the
// narrow ChatSessionLocker port.
//
// Behavior is unchanged from the previous service-layer implementation:
//   - Lazy allocation: a lock entry is created on first Lock() for a session.
//   - Idle GC: entries unused for SessionLockMaxIdle are reaped by a background
//     goroutine that ticks every SessionLockSweepPeriod.
//   - Stoppable: Close() terminates the GC goroutine and rejects new Lock() calls.
// ---------------------------------------------------------------------------

const (
	// SessionLockMaxIdle is the idle reaping threshold for lock entries.
	SessionLockMaxIdle = 30 * time.Minute
	// SessionLockSweepPeriod is the GC ticker interval.
	SessionLockSweepPeriod = 5 * time.Minute
)

// SessionLockEntry is the per-session lock state. Exported for tests.
type SessionLockEntry struct {
	mu       *sync.Mutex
	lastUsed time.Time
}

// SessionLockManager is the process-local session lock registry. It is safe
// for concurrent use; the manager-level mutex protects the map, and each
// entry has its own per-session mutex that callers receive via Lock().
type SessionLockManager struct {
	mu      sync.Mutex
	entries map[string]*SessionLockEntry
	stopCh  chan struct{}
	stopped bool
}

// NewSessionLockManager constructs a manager and starts its GC goroutine.
func NewSessionLockManager() *SessionLockManager {
	m := &SessionLockManager{
		entries: make(map[string]*SessionLockEntry),
		stopCh:  make(chan struct{}),
	}
	safego.Go(nil, "biz-session-lock-sweep", m.sweepLoop)
	return m
}

// Lock acquires the mutex for the given session. The returned function must
// be called to release the lock; calling it twice is a programmer error and
// may panic (Go mutex semantics).
//
// P1 FR-6：保留原阻塞语义，委托 LockCtx(context.Background())——永不可取消，
// 与之前 entry.mu.Lock() 等价。
func (m *SessionLockManager) Lock(sessionID string) func() {
	unlock, _ := m.LockCtx(context.Background(), sessionID)
	return unlock
}

// LockCtx acquires the per-session mutex honoring ctx cancellation
// (89-session-concurrency-hardening / M-1)：被限流 turn 一旦阻塞在互斥上，
// 用户取消/超时必须可解除——否则取消信号被锁吸收，turn 永远无法终止。
//
// 实现：TryLock 快路径 + 100ms tick 重试转轮（Go sync.Mutex 无条件变量，
// 轮询是可取消等待的唯一可行形态；100ms 与 lane 等待同一取消时延量级）。
// ctx 为 nil 或到期前锁可用时立即返回 (unlock, nil)；ctx 结束返回
// (nil, ctx.Err())，不取得锁、不留等待残留。
//
// Stability:internal
func (m *SessionLockManager) LockCtx(ctx context.Context, sessionID string) (func(), error) {
	entry := m.getOrCreate(sessionID)
	// 快路径：多数调用空锁直通，避免 ticker 分配。
	if entry.mu.TryLock() {
		return entry.mu.Unlock, nil
	}
	if ctx == nil {
		entry.mu.Lock()
		return entry.mu.Unlock, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if entry.mu.TryLock() {
				return entry.mu.Unlock, nil
			}
		}
	}
}

// getOrCreate returns the entry for the session, creating it on first use.
func (m *SessionLockManager) getOrCreate(sessionID string) *SessionLockEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[sessionID]; ok {
		e.lastUsed = time.Now()
		return e
	}
	e := &SessionLockEntry{
		mu:       &sync.Mutex{},
		lastUsed: time.Now(),
	}
	m.entries[sessionID] = e
	return e
}

// sweepLoop is the background GC goroutine. It exits on Close().
func (m *SessionLockManager) sweepLoop() {
	ticker := time.NewTicker(SessionLockSweepPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.sweep()
		case <-m.stopCh:
			return
		}
	}
}

// sweep reaps entries that have been idle for longer than SessionLockMaxIdle.
//
// A lock that is currently held must never be reaped, even if it has been
// held longer than the idle threshold: a long-running turn may legitimately
// hold the per-session mutex for >SessionLockMaxIdle. Reaping a held entry
// would drop it from the map, so the next Lock() for that session would
// create a fresh mutex and run concurrently with the still-running turn,
// breaking per-session mutual exclusion.
//
// TryLock is performed under the manager mutex (getOrCreate also takes the
// manager mutex), so the check-and-delete is race-free against new acquirers.
func (m *SessionLockManager) sweep() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, e := range m.entries {
		if now.Sub(e.lastUsed) <= SessionLockMaxIdle {
			continue
		}
		// Skip (and keep) entries whose per-session mutex is currently held.
		if !e.mu.TryLock() {
			continue
		}
		e.mu.Unlock()
		delete(m.entries, id)
	}
}

// Close stops the GC goroutine. The manager remains usable for Lock() after
// Close, but no further GC will occur. Safe to call multiple times.
func (m *SessionLockManager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	close(m.stopCh)
	m.mu.Unlock()
}

// Len returns the number of tracked lock entries. Useful for tests/metrics.
func (m *SessionLockManager) Len() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

// Compile-time assertion that *SessionLockManager satisfies the existing
// ChatSessionLocker port (the service layer wraps it via sessionLockerAdapter).
var _ ChatSessionLocker = (*SessionLockManager)(nil)
