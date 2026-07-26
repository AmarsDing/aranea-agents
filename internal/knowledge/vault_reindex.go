package knowledge

import (
	"context"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ReindexVault 一键重建派生索引（P1-4）：文件系统为唯一真相源，
// 派生索引（chunks/向量/links）无状态可重建。
//
// 与 SyncOnce 的区别：SyncOnce 以「DB hash 一致且已索引」短路跳过（增量优化）；
// ReindexVault 强制重建所有文档的 chunks——用于索引损坏、embedding 模型升级、
// 分块策略变更后的全量修复。
//
// 流程：
//  1. Scan 文件系统（mtime 预筛不适用：prev 传 nil，全量重算 hash）
//  2. DB 镜像中有、磁盘无 → Deleted 事件
//  3. 磁盘有（无论 DB 是否有镜像）→ Created 事件 + force 重建
//  4. ApplyEventsForced 绕过幂等短路，全部重建
//  5. 更新 sync_state/last_sync_at，prev 快照重置为最新
//
// 失败语义：Scan 失败不破坏现有镜像；Apply 失败已重建的文档保持新索引、
// 未重建的文档因 hash 未落库而在下轮 SyncOnce 自动重试。
func (r *VaultSyncRunner) ReindexVault(ctx context.Context, vault bizknowledge.Collection) error {
	curr, err := r.engine.Scan(vault.RootPath, nil)
	if err != nil {
		r.markState(ctx, vault.ID, "error", time.Time{})
		return err
	}
	docs, _, err := r.uc.ListDocuments(ctx, vault.ID, 10000, 0)
	if err != nil {
		r.markState(ctx, vault.ID, "error", time.Time{})
		return err
	}

	onDisk := make(map[string]bool, len(curr))
	events := make([]bizknowledge.ChangeEvent, 0, len(curr))
	for _, snap := range curr {
		onDisk[snap.RelPath] = true
		events = append(events, bizknowledge.ChangeEvent{
			Type:     bizknowledge.ChangeCreated, // force 模式下 upsert 处理已存在镜像
			RelPath:  snap.RelPath,
			Snapshot: snap,
		})
	}
	for _, d := range docs {
		if d.RelPath == "" || onDisk[d.RelPath] {
			continue
		}
		events = append(events, bizknowledge.ChangeEvent{
			Type:    bizknowledge.ChangeDeleted,
			RelPath: d.RelPath,
		})
	}

	if err := r.applier.ApplyEventsForced(ctx, vault, events); err != nil {
		r.markState(ctx, vault.ID, "error", time.Time{})
		// 失败不保存 prev：下轮 SyncOnce 从旧 prev diff，失败文档自动重试。
		return err
	}
	r.markState(ctx, vault.ID, "active", time.Now().UTC())
	r.savePrev(vault.ID, curr)
	r.lg.Info("vault reindex done",
		loggateway.Str("vault_id", vault.ID),
		loggateway.Int("files", len(curr)),
		loggateway.Int("events", len(events)),
	)
	return nil
}
