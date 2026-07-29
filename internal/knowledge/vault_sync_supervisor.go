package knowledge

import (
	"context"
	"strings"
	"sync"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// VaultSyncSupervisor 管理每 vault 一个 RunVault goroutine 的生命周期（P1-3 生产装配）：
// 启动时 StartAll 拉起全部存量 vault；CreateCollection 成功后 StartVault；
// DeleteCollection 前 StopVault；进程关闭 Stop 统一取消。
//
// goroutine 挂在 supervisor 自持的 base ctx（Background 派生）上——不依赖启动期
// kratos ctx 的生命周期，Stop 时统一回收。RunVault 启动即扫一轮，新建 vault 立即有数据。
type VaultSyncSupervisor struct {
	runner *VaultSyncRunner
	uc     *bizknowledge.Usecase
	lg     loggateway.Logger

	mu      sync.Mutex
	base    context.Context
	cancel  context.CancelFunc
	running map[string]context.CancelFunc // vaultID → cancel
}

// NewVaultSyncSupervisor 构造。lg 为 nil 时使用 Noop。
func NewVaultSyncSupervisor(runner *VaultSyncRunner, uc *bizknowledge.Usecase, lg loggateway.Logger) *VaultSyncSupervisor {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	base, cancel := context.WithCancel(context.Background())
	return &VaultSyncSupervisor{
		runner:  runner,
		uc:      uc,
		lg:      lg.With(loggateway.Domain("knowledge")),
		base:    base,
		cancel:  cancel,
		running: make(map[string]context.CancelFunc),
	}
}

// StartAll 拉起所有存量 vault（root_path 非空）的同步循环；单个失败不阻塞其余。
// 历史 collection（root_path 空）跳过——不属于 vault 同步范围。
func (s *VaultSyncSupervisor) StartAll(ctx context.Context) {
	cols, _, err := s.uc.ListCollections(ctx, "", 10000, 0)
	if err != nil {
		s.lg.Warn("vault sync: list collections failed", loggateway.Err(err))
		return
	}
	n := 0
	for _, c := range cols {
		if strings.TrimSpace(c.RootPath) == "" {
			continue
		}
		s.StartVault(c)
		n++
	}
	s.lg.Info("vault sync supervisor started", loggateway.Int("vaults", n))
}

// StartVault 幂等启动单 vault 同步循环（RunVault 启动即扫一轮）。
func (s *VaultSyncSupervisor) StartVault(vault bizknowledge.Collection) {
	s.mu.Lock()
	if _, ok := s.running[vault.ID]; ok {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(s.base)
	s.running[vault.ID] = cancel
	s.mu.Unlock()
	safego.Go(ctx, "knowledge.vault_sync."+vault.ID, func() {
		_ = s.runner.RunVault(ctx, vault)
	})
}

// StopVault 停止并移除单 vault 同步循环（幂等）。
func (s *VaultSyncSupervisor) StopVault(vaultID string) {
	s.mu.Lock()
	cancel, ok := s.running[vaultID]
	if ok {
		delete(s.running, vaultID)
	}
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

// Stop 停止全部同步循环（进程关闭时调用；幂等）。
func (s *VaultSyncSupervisor) Stop() {
	s.mu.Lock()
	for id, cancel := range s.running {
		cancel()
		delete(s.running, id)
	}
	s.mu.Unlock()
	s.cancel()
}

// RunningCount 当前运行中的同步循环数（测试/观测用）。
func (s *VaultSyncSupervisor) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}
