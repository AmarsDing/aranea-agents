package knowledge

import (
	"context"
	"sync"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// Watcher 文件系统变更通知通道（P2 接入 fsnotify；P1 仅接口预留）。
// 事件只作为「提前扫描」的 hint，不作变更真相——Runner 始终以 Scan+Diff 为准。
// 这样 watcher 失效时退化为纯轮询，fsnotify 接入时无需改 diff/apply 路径。
type Watcher interface {
	// Changed 返回 hint 通道；每次 receive 表示「可能变更，请尽快扫描」。
	// 通道关闭表示 watcher 已停止，Runner 退化为纯轮询。
	Changed() <-chan struct{}
	// Close 释放底层资源。幂等。
	Close() error
}

const defaultVaultSyncInterval = 30 * time.Second

// VaultSyncRunner 单 vault 轮询同步循环（P1-3）：
// tick / watcher hint → engine.Scan → DiffSnapshots → applier.ApplyEvents →
// 回写 sync_state + last_sync_at。
//
// prev 快照按 vaultID 内存维护；首次 SyncOnce 前若 prev 为空，会从 DB
// ListDocuments 重建 prev——否则重启前从磁盘删除的文件永远不会产生
// deleted 事件（stale mirror 无法清理）。
type VaultSyncRunner struct {
	engine  *bizknowledge.SyncEngine
	applier *VaultSyncApplier
	uc      *bizknowledge.Usecase
	lg      loggateway.Logger
	// monitorBus 流程日志总线（装配层经 SetMonitorBus 注入；nil 时跳过流程日志）。
	monitorBus contract.MonitorBus

	interval time.Duration
	watcher  Watcher

	mu   sync.Mutex
	prev map[string][]bizknowledge.FileSnapshot // vaultID → 上一轮快照
	// lastConverge 块索引漂移收敛的逐 vault 上次执行时间（SP2 #4 低频门控）。
	lastConverge map[string]time.Time
}

// NewVaultSyncRunner 构造。lg 为 nil 时使用 Noop。
func NewVaultSyncRunner(engine *bizknowledge.SyncEngine, applier *VaultSyncApplier, uc *bizknowledge.Usecase, lg loggateway.Logger) *VaultSyncRunner {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &VaultSyncRunner{
		engine:   engine,
		applier:  applier,
		uc:       uc,
		lg:       lg.With(loggateway.Domain("knowledge")),
		interval: defaultVaultSyncInterval,
		prev:     make(map[string][]bizknowledge.FileSnapshot),
	}
}

// SetInterval 覆盖默认轮询间隔（测试用）。
func (r *VaultSyncRunner) SetInterval(d time.Duration) {
	if d > 0 {
		r.interval = d
	}
}

// SetWatcher 注入 watcher（可选）。Close 由调用方管理。
func (r *VaultSyncRunner) SetWatcher(w Watcher) { r.watcher = w }

// SetMonitorBus 注入流程日志总线（装配层调用；nil = 不发射流程日志）。
func (r *VaultSyncRunner) SetMonitorBus(bus contract.MonitorBus) {
	if r == nil {
		return
	}
	r.monitorBus = bus
}

// SyncOnce 对单个 vault 执行一轮同步；返回首个错误（已回写 sync_state）。
func (r *VaultSyncRunner) SyncOnce(ctx context.Context, vault bizknowledge.Collection) error {
	prev, err := r.loadPrev(ctx, vault)
	if err != nil {
		r.markState(ctx, vault.ID, "error", time.Time{})
		r.logVaultSyncError(ctx, vault.ID, err)
		return err
	}
	curr, err := r.engine.Scan(vault.RootPath, prev)
	if err != nil {
		r.markState(ctx, vault.ID, "error", time.Time{})
		r.logVaultSyncError(ctx, vault.ID, err)
		return err
	}
	events := bizknowledge.DiffSnapshots(prev, curr)
	if len(events) > 0 {
		if applyErr := r.applier.ApplyEvents(ctx, vault, events); applyErr != nil {
			r.markState(ctx, vault.ID, "error", time.Time{})
			r.logVaultSyncError(ctx, vault.ID, applyErr)
			// 失败不保存 prev：下轮 diff 重新生成事件——成功文档走幂等短路
			// （DB hash 已一致，廉价跳过），失败文档自动重试（可靠性契约自愈）。
			return applyErr
		}
		// 仅在确有变更时打 done——30s 轮询的空转轮次不产生流程日志噪声。
		if r.monitorBus != nil {
			newKnowledgeFlow(ctx, r.monitorBus, nil).LogDone("knowledge.vault.sync", "Vault 同步完成",
				event.P("vault_id", vault.ID),
				event.P("changed_files", len(events)))
		}
	}
	// SP2 #9：退避重试熔断中文档的向量补齐。无文件变更时也运行——熔断恢复独立于
	// 文件事件（故障恢复后不应等文件变更才补向量）。无语义层/无降级文档时廉价短路；
	// 扫描失败不拖垮同步轮次（K4：Warn 后照常 active）。
	if err := r.applier.RetryDegradedEmbeddings(ctx, vault); err != nil {
		r.lg.Warn("vault embed retry sweep failed",
			loggateway.Str("vault_id", vault.ID),
			loggateway.Err(err),
		)
	}
	// SP2 #4：块索引漂移收敛（低频，convergeInterval 门控）。rebuildBlockIndex 失败
	// 仅 Warn 降级而 content_hash 已落库、下轮不再重试——靠此校验检出并自动重建。
	if r.convergeDue(vault.ID, time.Now()) {
		if err := r.applier.ConvergeMissingBlockIndex(ctx, vault); err != nil {
			r.lg.Warn("vault block index converge failed",
				loggateway.Str("vault_id", vault.ID),
				loggateway.Err(err),
			)
		}
	}
	r.markState(ctx, vault.ID, "active", time.Now().UTC())
	r.savePrev(vault.ID, curr)
	return nil
}

// convergeInterval 是块索引漂移收敛的最低间隔（SP2 #4 低频校验；30s 轮询不每轮执行）。
const convergeInterval = 10 * time.Minute

// convergeDue 判定该 vault 是否到达收敛窗口并登记本次时间（先登记者执行，
// 并发轮次天然去重）。不同 vault 独立计窗。
func (r *VaultSyncRunner) convergeDue(vaultID string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastConverge == nil {
		r.lastConverge = make(map[string]time.Time)
	}
	last, ok := r.lastConverge[vaultID]
	if ok && now.Sub(last) < convergeInterval {
		return false
	}
	r.lastConverge[vaultID] = now
	return true
}

func (r *VaultSyncRunner) logVaultSyncError(ctx context.Context, vaultID string, err error) {
	if r.monitorBus == nil {
		return
	}
	newKnowledgeFlow(ctx, r.monitorBus, nil).LogError("knowledge.vault.sync", "Vault 同步失败",
		event.P("vault_id", vaultID),
		event.P("error", err.Error()))
}

// RunVault 启动循环：启动即扫一轮，之后按 interval 轮询或 watcher hint 提前扫描。
// 阻塞直到 ctx 取消（返回 nil）。
func (r *VaultSyncRunner) RunVault(ctx context.Context, vault bizknowledge.Collection) error {
	if r.monitorBus != nil {
		newKnowledgeFlow(ctx, r.monitorBus, nil).LogStart("knowledge.vault.sync", "Vault 同步启动",
			event.P("vault_id", vault.ID))
	}
	// 启动即扫一轮，不等 interval。
	if err := r.SyncOnce(ctx, vault); err != nil {
		r.lg.Warn("vault sync initial scan failed",
			loggateway.Str("vault_id", vault.ID),
			loggateway.Err(err),
		)
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	var watcherCh <-chan struct{}
	if r.watcher != nil {
		watcherCh = r.watcher.Changed()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.syncOnceLog(ctx, vault, "tick")
		case _, ok := <-watcherCh:
			if !ok {
				// watcher 关闭：退化为纯轮询。
				watcherCh = nil
				continue
			}
			r.syncOnceLog(ctx, vault, "watcher")
		}
	}
}

func (r *VaultSyncRunner) syncOnceLog(ctx context.Context, vault bizknowledge.Collection, trigger string) {
	if err := r.SyncOnce(ctx, vault); err != nil {
		r.lg.Warn("vault sync failed",
			loggateway.Str("vault_id", vault.ID),
			loggateway.Str("trigger", trigger),
			loggateway.Err(err),
		)
	}
}

// loadPrev 取出上一轮 prev；为空时从 DB ListDocuments 重建（重启场景）。
// 重建的 prev 仅 relPath+hash 有效——mtime/size 不参与 diff 判等（engine.Scan 的
// mtime 预筛会回退到重算 hash，行为正确）。
func (r *VaultSyncRunner) loadPrev(ctx context.Context, vault bizknowledge.Collection) ([]bizknowledge.FileSnapshot, error) {
	r.mu.Lock()
	cached, ok := r.prev[vault.ID]
	r.mu.Unlock()
	if ok {
		return cached, nil
	}
	docs, _, err := r.uc.ListDocuments(ctx, vault.ID, 10000, 0)
	if err != nil {
		return nil, err
	}
	rebuilt := make([]bizknowledge.FileSnapshot, 0, len(docs))
	for _, d := range docs {
		if d.RelPath == "" {
			continue // 非 vault 镜像（UI 上传等），不参与 diff
		}
		rebuilt = append(rebuilt, bizknowledge.FileSnapshot{
			RelPath: d.RelPath,
			Hash:    d.ContentHash,
		})
	}
	return rebuilt, nil
}

func (r *VaultSyncRunner) savePrev(vaultID string, curr []bizknowledge.FileSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prev[vaultID] = curr
}

func (r *VaultSyncRunner) markState(ctx context.Context, vaultID, state string, lastSyncAt time.Time) {
	if err := r.uc.UpdateCollectionSyncState(ctx, vaultID, state, lastSyncAt); err != nil {
		r.lg.Warn("vault sync: mark state failed",
			loggateway.Str("vault_id", vaultID),
			loggateway.Str("state", state),
			loggateway.Err(err),
		)
	}
}
