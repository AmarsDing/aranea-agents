package lifecycle

import (
	"sync"

	"aranea-agents/pkg/loggateway"
)

// Closer 是可关闭的资源接口。注册到 LifecycleManager 的资源必须实现此接口。
type Closer interface {
	Close() error
}

// LifecycleManager 统一管理进程级资源的生命周期。
//
// 设计目标（A3）：替代散落的包级 var + 无调用点的 Close() 方法。
// 资源在构造时注册到 LifecycleManager，应用关闭时按 LIFO 顺序调用 Close。
//
// 使用方式：
//
//	mgr := NewLifecycleManager(lg)
//	cache := newBuildCache(...)
//	mgr.Register("build-cache", cache)
//	// 应用关闭时
//	mgr.Close() // 按 LIFO 顺序调用 cache.Close()
//
// 并发安全：所有方法均可并发调用。但 Close() 应在应用 shutdown 时串行调用。
type LifecycleManager struct {
	mu      sync.Mutex
	closers []namedCloser
	lg      loggateway.Logger
	closed  bool
}

type namedCloser struct {
	name   string
	closer Closer
}

// NewLifecycleManager 创建一个 LifecycleManager。
func NewLifecycleManager(lg loggateway.Logger) *LifecycleManager {
	if lg != nil {
		lg = lg.With(loggateway.Domain("lifecycle"))
	}
	return &LifecycleManager{lg: lg}
}

// Register 注册一个资源。name 用于日志标识，应唯一。
// 重复注册同名资源会追加（按 LIFO 顺序关闭）。
func (m *LifecycleManager) Register(name string, c Closer) {
	if m == nil || c == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		// 已关闭，立即关闭新注册的资源
		if err := c.Close(); err != nil && m.lg != nil {
			m.lg.Warn("close resource failed during late register",
				loggateway.Str("name", name), loggateway.Err(err))
		}
		return
	}
	m.closers = append(m.closers, namedCloser{name: name, closer: c})
}

// Close 按 LIFO 顺序关闭所有已注册资源。可多次调用，后续调用无操作。
//
// 任一资源 Close 出错不会中断后续资源的关闭，错误会被记录到日志。
func (m *LifecycleManager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	closers := m.closers
	m.closers = nil
	m.mu.Unlock()

	// LIFO 顺序关闭
	for i := len(closers) - 1; i >= 0; i-- {
		nc := closers[i]
		if err := nc.closer.Close(); err != nil && m.lg != nil {
			m.lg.Warn("close resource failed",
				loggateway.Str("name", nc.name), loggateway.Err(err))
		}
	}
}
