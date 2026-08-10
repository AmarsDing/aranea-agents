package knowledge

import (
	"context"
	"time"
)

// SP1-H RebuildIndex（设计 S9 / US-29）：集合块级派生索引（blocks/refs）流式重建。
//
// 语义：
//   - 按文档分批：每文档一事务（ReplaceDocBlocks 删了重插），幂等可重入——
//     中断后重跑等价于从头执行，已重建文档产出与未重建前一致（验收 43）。
//   - sync_state 进入 rebuilding、结束恢复原态（崩溃残留的 rebuilding 视为
//     active 恢复）。期间检索走旧 chunks/FTS 不受影响（本流程不动 chunks），降级可用。
//   - 单文档失败计数后继续（同 vault sync 降级哲学），最终一致；调用方据此
//     上报进度/失败数。
//   - chunks/FTS/向量重建不在本流程：local 库走 ReindexVault（P1-4 既有），
//     team 库内容重摄取走 Ingest 路径。

// SyncStateRebuilding 集合块索引重建中（SP1-H 新增 sync_state 取值）。
const SyncStateRebuilding = "rebuilding"

// rebuildPageSize 流式重建的文档分页大小。
const rebuildPageSize = 200

// RebuildIndexResult 重建统计（done+failed=total 时全部收敛）。
type RebuildIndexResult struct {
	Total  int
	Done   int
	Failed int
}

// RebuildCollectionBlockIndex 重建集合全部文档的块级派生索引。onProgress 在每篇
// 文档处理后回调（nil 安全）；集合不存在/端口未接线上抛错误。
func (u *Usecase) RebuildCollectionBlockIndex(ctx context.Context, collectionID string, onProgress func(done, total, failed int)) (RebuildIndexResult, error) {
	if err := u.requireRepo(); err != nil {
		return RebuildIndexResult{}, err
	}
	if u.blockIndex == nil {
		return RebuildIndexResult{}, ErrUnavailable
	}
	col, err := u.collections.GetCollection(ctx, collectionID)
	if err != nil {
		return RebuildIndexResult{}, err
	}
	// 可见集整批提升一次（同集合全部文档共享 workspace 可见边界，不逐文档重查）。
	visible, err := u.visibleCollectionIDs(ctx, collectionID)
	if err != nil {
		return RebuildIndexResult{}, err
	}

	prev := col.SyncState
	if prev == "" || prev == SyncStateRebuilding {
		prev = "active" // 崩溃残留的 rebuilding/空态按 active 恢复
	}
	if err := u.collections.UpdateCollectionSyncState(ctx, collectionID, SyncStateRebuilding, time.Time{}); err != nil {
		return RebuildIndexResult{}, err
	}
	// 恢复 best-effort：重建结果已落库，状态恢复失败不推翻成果（下轮重建/同步自愈）。
	defer func() {
		_ = u.collections.UpdateCollectionSyncState(ctx, collectionID, prev, time.Time{})
	}()

	var res RebuildIndexResult
	for offset := 0; ; offset += rebuildPageSize {
		docs, total, err := u.documents.ListDocuments(ctx, collectionID, rebuildPageSize, offset)
		if err != nil {
			return res, err
		}
		res.Total = total
		for _, doc := range docs {
			// ListDocuments 是摘要投影（SELECT 不含 content_text）：必须逐文档
			// GetDocument 取回正文再重建——2026-08-09 运行时事故：直接传
			// doc.ContentText（恒空）解析出 0 块 0 边，ReplaceDocBlocks 删了
			// 重插成空，全库索引被静默清空。
			full, gerr := u.documents.GetDocument(ctx, doc.ID)
			switch {
			case gerr != nil:
				res.Failed++
			default:
				// 全量重建不改源文本（allowBackfill=false）：重建是索引修复，
				// 惰性锚点回填留给写路径触发，避免批量回填改写用户文档。
				if err := u.rebuildBlockIndex(ctx, collectionID, doc.ID, full.ContentText, visible, false); err != nil {
					res.Failed++
				} else {
					res.Done++
				}
			}
			if onProgress != nil {
				onProgress(res.Done, res.Total, res.Failed)
			}
		}
		if len(docs) < rebuildPageSize {
			break
		}
	}
	return res, nil
}
