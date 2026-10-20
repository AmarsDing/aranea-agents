package acpshim

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// guard.go — 合成中继防自批守卫（fail-closed，设计 §E）。
//
// shim 的权限请求是「自己合成」的，相比普通中继多一个「宿主被蒙骗」面，
// 因此默认全部按最保守路径处理：
//   - 宿主审批超时 → Broker 收到 reject/超时 → 中止当前 loop（permission.go）；
//   - 权限等待期间宿主失联（conn.Done）→ Call 立即失败，不发任何自动决策；
//   - 权限请求速率限制 + 挂起上限（防失控模型「刷卡」掩护）；
//   - 「无人值守」与「审批中继」配置互斥，启动即校验拒绝（防静默降权）。

// ErrUnattendedForbidden 是无人值守×审批互斥的启动校验错误。
var ErrUnattendedForbidden = errors.New("acpshim: unattended mode is forbidden with approval relay (mutually exclusive)")

// CheckConfig 启动校验（NewServer 调用）。fail-closed：缺装配即拒。
func CheckConfig(cfg *Config) error {
	if cfg.Unattended {
		return ErrUnattendedForbidden
	}
	if cfg.LLM == nil {
		return errors.New("acpshim: LLM client required")
	}
	if cfg.Tools == nil {
		return errors.New("acpshim: tool executor required")
	}
	if cfg.Approver == nil {
		return errors.New("acpshim: approval broker required")
	}
	return nil
}

// PermissionGuard 权限请求速率限制 + 挂起上限。
// 零值不可用；NewPermissionGuard 构造。
type PermissionGuard struct {
	maxPerWindow int           // 窗口内最大请求数（<=0 → 默认 20）
	window       time.Duration // 滚动窗口（<=0 → 默认 1min）
	maxPending   int           // 挂起上限（<=0 → 默认 1）

	mu      sync.Mutex
	times   []time.Time // 窗口内请求时刻（滚动清理）
	pending int
	nowFn   func() time.Time // 测试注入
}

// NewPermissionGuard 构造守卫；三个参数均 <=0 时取默认值（20/min、挂起 1）。
func NewPermissionGuard(maxPerWindow int, window time.Duration, maxPending int) *PermissionGuard {
	if maxPerWindow <= 0 {
		maxPerWindow = 20
	}
	if window <= 0 {
		window = time.Minute
	}
	if maxPending <= 0 {
		maxPending = 1
	}
	return &PermissionGuard{
		maxPerWindow: maxPerWindow,
		window:       window,
		maxPending:   maxPending,
		nowFn:        time.Now,
	}
}

// ErrPermissionStorm 速率超限错误（Broker 返回 → loop 中止，fail-closed）。
var ErrPermissionStorm = errors.New("acpshim: permission request rate exceeded")

// ErrPermissionPendingCap 挂起上限错误。
var ErrPermissionPendingCap = errors.New("acpshim: too many pending permission requests")

// Enter 请求准入：挂起 +1 并记一次速率。越限返回 error（不发生状态变化）。
func (g *PermissionGuard) Enter() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.nowFn()
	// 滚动窗口清理
	cutoff := now.Add(-g.window)
	keep := g.times[:0]
	for _, t := range g.times {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	g.times = keep
	if len(g.times) >= g.maxPerWindow {
		return fmt.Errorf("%w (max %d per %s)", ErrPermissionStorm, g.maxPerWindow, g.window)
	}
	if g.pending >= g.maxPending {
		return fmt.Errorf("%w (max %d)", ErrPermissionPendingCap, g.maxPending)
	}
	g.times = append(g.times, now)
	g.pending++
	return nil
}

// Leave 挂起 -1（与 Enter 成功配对；Broker 在 Call 返回后调用）。
func (g *PermissionGuard) Leave() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending > 0 {
		g.pending--
	}
}

// SetClock 注入测试时钟（须在首次 Enter 前调用，否则 panic 防御）。
func (g *PermissionGuard) SetClock(fn func() time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.times) > 0 || g.pending > 0 {
		panic("acpshim: SetClock after first Enter")
	}
	g.nowFn = fn
}
