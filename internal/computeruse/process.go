// process.go sidecar（aranea-cua-win.exe）子进程生命周期管理。
// 职责：拉起/停止进程、心跳看门狗（僵死自动重启）、K7 进程日志。
// 不 import biz 包——biz 依赖方向由 Gateway 单向建立。
package computeruse

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// 看门狗与停止的默认参数（§2.1：每 5s 心跳，连续 3 次超时判僵死）。
const (
	defaultPingInterval  = 5 * time.Second
	defaultPingTimeout   = 3 * time.Second
	defaultMissThreshold = 3
	defaultStopGrace     = 2 * time.Second
)

// processHandle 抽象一个已拉起的 sidecar 进程（测试可注入 fake）。
type processHandle struct {
	stdin  io.WriteCloser // 写请求帧；关闭触发 sidecar 优雅退出
	stdout io.Reader      // 读响应帧
	wait   func() error   // 等待进程退出
	kill   func() error   // 强杀
}

// starterFunc 进程启动器（生产为真实子进程，测试注入 fake）。
type starterFunc func(ctx context.Context) (*processHandle, error)

// Manager sidecar 进程管理器：拉起、心跳看门狗、优雅停止、自动重启。
type Manager struct {
	path string
	lg   loggateway.Logger

	// starter 为 nil 时按平台使用真实子进程启动器（仅 windows）
	starter starterFunc

	// 看门狗参数（NewManager 赋默认值，测试可覆盖）
	pingInterval  time.Duration
	pingTimeout   time.Duration
	missThreshold int
	stopGrace     time.Duration

	mu     sync.Mutex
	handle *processHandle
	client *Client
	stopCh chan struct{} // 看门狗停止信号（每轮运行一个）
	wg     sync.WaitGroup
}

// NewManager 构造进程管理器；path 为 sidecar 可执行文件路径（如 bin/cua/aranea-cua-win.exe）。
func NewManager(path string, lg loggateway.Logger) *Manager {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Manager{
		path:          path,
		lg:            lg.With(loggateway.Domain("computeruse")),
		pingInterval:  defaultPingInterval,
		pingTimeout:   defaultPingTimeout,
		missThreshold: defaultMissThreshold,
		stopGrace:     defaultStopGrace,
	}
}

// EnsureRunning 幂等确保 sidecar 在运行；非 windows 平台返回明确错误。
func (m *Manager) EnsureRunning(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil {
		return nil
	}
	st, err := m.pickStarter()
	if err != nil {
		return err
	}
	if err := m.startLocked(ctx, st); err != nil {
		return err
	}
	// 拉起看门狗（每轮运行仅一个；重启复用当前看门狗，不重复启动）
	m.stopCh = make(chan struct{})
	m.wg.Add(1)
	go m.watchdog(m.stopCh)
	return nil
}

// Client 返回当前连接的 RPC 客户端；未运行时为 nil。
func (m *Manager) Client() *Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.client
}

// Stop 优雅关闭：先看门狗退出，再关 stdin 等 stopGrace，超时则强杀。幂等。
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if m.stopCh != nil {
		close(m.stopCh)
		m.stopCh = nil
	}
	m.mu.Unlock()

	// 等看门狗退出（不持锁，避免与 restart 死锁）
	m.wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle == nil {
		return nil
	}
	m.stopHandleLocked()
	m.lg.Info("sidecar 已停止", loggateway.StepID("computeruse.sidecar.stop"))
	return nil
}

// pickStarter 选择启动器：注入优先；否则仅 windows 使用真实子进程。
func (m *Manager) pickStarter() (starterFunc, error) {
	if m.starter != nil {
		return m.starter, nil
	}
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("computeruse: sidecar 仅支持 windows 平台（当前 %s）", runtime.GOOS)
	}
	return m.startRealProcess, nil
}

// startLocked 拉起进程并建立 client（调用方持锁；看门狗由 EnsureRunning 统一启动）。
func (m *Manager) startLocked(ctx context.Context, st starterFunc) error {
	h, err := st(ctx)
	if err != nil {
		m.lg.Error("sidecar 启动失败",
			loggateway.StepID("computeruse.sidecar.start"),
			loggateway.Str("path", m.path),
			loggateway.Err(err))
		return fmt.Errorf("computeruse: sidecar 启动失败: %w", err)
	}
	m.handle = h
	m.client = NewClient(h.stdin, h.stdout, m.lg)
	m.lg.Info("sidecar 已启动",
		loggateway.StepID("computeruse.sidecar.start"),
		loggateway.Str("path", m.path))
	return nil
}

// watchdog 心跳看门狗：每 pingInterval 发 device.ping，连续 missThreshold 次超时重启。
func (m *Manager) watchdog(stopCh chan struct{}) {
	defer m.wg.Done()
	defer func() {
		if rec := recover(); rec != nil {
			m.lg.Error("sidecar 看门狗 panic",
				loggateway.StepID("computeruse.sidecar.watchdog"),
				loggateway.Any("panic", rec))
		}
	}()

	ticker := time.NewTicker(m.pingInterval)
	defer ticker.Stop()
	misses := 0
	for {
		select {
		case <-stopCh:
			m.lg.Info("sidecar 看门狗退出", loggateway.StepID("computeruse.sidecar.watchdog"))
			return
		case <-ticker.C:
		}

		m.mu.Lock()
		c := m.client
		m.mu.Unlock()
		if c == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), m.pingTimeout)
		_, err := c.Call(ctx, "device.ping", nil)
		cancel()
		if err == nil {
			misses = 0
			continue
		}
		misses++
		m.lg.Warn("sidecar 心跳超时",
			loggateway.StepID("computeruse.sidecar.watchdog"),
			loggateway.Int("misses", misses),
			loggateway.Err(err))
		if misses >= m.missThreshold {
			m.lg.Error("sidecar 心跳连续超时，判定僵死并重启",
				loggateway.StepID("computeruse.sidecar.restart"),
				loggateway.Int("misses", misses))
			m.restart()
			misses = 0
		}
	}
}

// restart 杀死后重新拉起（看门狗调用；失败时保持停止态，等待上层 EnsureRunning 重试）。
func (m *Manager) restart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle == nil {
		return // Stop 已介入
	}
	m.killLocked()
	st, err := m.pickStarter()
	if err != nil {
		m.lg.Error("sidecar 重启失败",
			loggateway.StepID("computeruse.sidecar.restart"),
			loggateway.Err(err))
		return
	}
	if err := m.startLocked(context.Background(), st); err != nil {
		m.lg.Error("sidecar 重启失败",
			loggateway.StepID("computeruse.sidecar.restart"),
			loggateway.Err(err))
		return
	}
	m.lg.Info("sidecar 已自动重启", loggateway.StepID("computeruse.sidecar.restart"))
}

// killLocked 强杀当前进程并关闭 client（调用方持锁）。
func (m *Manager) killLocked() {
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
	if m.handle != nil {
		m.handle.kill()
		m.handle = nil
	}
}

// stopHandleLocked 优雅停止：先关 stdin，等待 stopGrace，超时强杀（调用方持锁）。
func (m *Manager) stopHandleLocked() {
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
	h := m.handle
	m.handle = nil
	if h == nil {
		return
	}
	h.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- h.wait() }()
	select {
	case <-done:
	case <-time.After(m.stopGrace):
		h.kill()
		// 强杀后最多再等一个宽限期，避免进程残留阻塞 Stop
		select {
		case <-done:
		case <-time.After(m.stopGrace):
		}
	}
}

// startRealProcess 真实拉起 sidecar 子进程（仅 windows 会走到这里）。
func (m *Manager) startRealProcess(_ context.Context) (*processHandle, error) {
	cmd := exec.Command(m.path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	// stderr 排入进程日志（sidecar 自身诊断输出）
	go m.drainStderr(stderr)

	return &processHandle{
		stdin:  stdin,
		stdout: stdout,
		wait:   cmd.Wait,
		kill: func() error {
			if cmd.Process == nil {
				return nil
			}
			return cmd.Process.Kill()
		},
	}, nil
}

// drainStderr 逐行读取 sidecar stderr 写入进程日志。
func (m *Manager) drainStderr(r io.Reader) {
	defer func() {
		if rec := recover(); rec != nil {
			m.lg.Error("sidecar stderr 读取 panic",
				loggateway.StepID("computeruse.sidecar.stderr"),
				loggateway.Any("panic", rec))
		}
	}()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		m.lg.Warn("sidecar stderr",
			loggateway.StepID("computeruse.sidecar.stderr"),
			loggateway.Str("line", scanner.Text()))
	}
}
