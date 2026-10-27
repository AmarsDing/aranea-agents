// sourcesync_unbind.go —— G-38 结转（W2 挂账收口，G-40 A1）：带外删档反扫 port。
//
// 背景：connector run 外（UI 删除/vault 外部删除镜像回收/治理处置）走
// Usecase.DeleteDocument(Mirror) 删除已建档文档时，sourcesync items 指纹行的
// document_id 绑定仍指向已删文档——悬空锚。W3 Applier 已兜底（下一次 Upsert
// 遇 GetDocument not-found → 落 adopt/create 重建档），但本兜底只在「源漂移
// 触发 needsWork」后发生；源不变时 items 行 ContentHash 已回填、needsWork=false，
// 悬空绑定常态化存在且语义撒谎（账本声称绑定一个不存在的事实）。
//
// 决策：删除成功后 best-effort 回调 SourceItemUnbinder 清绑定（document_id 置
// 空串 + own_lifecycle=false）。钩点=DeleteDocumentMirror（DeleteDocument 与
// 治理 orphan 处置（C 批修）均经此；行已不存在时 NotFound 幂等跳过）。清空后
// runner needsWork 判定（DocumentID==""）自然在下轮重摄取并经 adopt/create
// 重绑——零新增调度路径。content_hash 保留（曾成功摄取的源侧 checkpoint 事实；
// 重摄取提交后被 SetItemContentHash 覆写，无陈旧风险）。
// DeleteCollection 集合级联不逐文档经此钩点——删除前预扫文档 ID，集合删除
// 成功后逐文档 best-effort 回调本口（同语义，见 knowledge.go DeleteCollection）。
//
// 已知边界（G-40 C 批评审 #10/#111 双镜确认，拍板登记不修机制）：runner ⑥-7
// 级联删档路径同经此钩点——级联次序（先 applier.Delete 删档、后 MarkItemDeleted
// 墓碑化）决定反扫谓词 status<>'deleted' 会把『级联在途 missing 行』的绑定一并
// 清掉；随后落成的 deleted 墓碑行 document_id 恒为空。即：墓碑谓词豁免只保护
// 『删档前已存在的』墓碑行，级联新生墓碑不带历史工程锚（PG 钉
// TestSourceSyncItem_UnbindCascadeTombstoneTerminalState 钉死该终态）。adopt 行
// （own_lifecycle=false 共享他源文档）的悬空交给 W3 dangling 兜底。
package knowledge

import (
	"context"

	"aranea-agents/pkg/loggateway"
)

// SourceItemUnbinder 带外删档反扫窄口（Stability: Internal —— 仅装配侧消费）。
// 实现=internal/data sourceSyncRepo（与 bizsync.ItemRepo 同源同实现）。
type SourceItemUnbinder interface {
	// UnbindItemsByDocumentID 清所有绑定到 documentID 的非 deleted 行
	//（status<>'deleted'——谓词豁免仅保护删档前已存在的 deleted 墓碑行的历史
	// 工程锚；级联在途行（status='missing'）同被清锚，见包级头注已知边界）。
	// 返回受影响行数；失败由调用方 best-effort 告警（删除事实已成立，不阻断）。
	UnbindItemsByDocumentID(ctx context.Context, documentID string) (int, error)
}

// SetSourceItemUnbinder 接线反扫窄口（可选；nil=未接线删除路径零旁效应）。
func (u *Usecase) SetSourceItemUnbinder(v SourceItemUnbinder) {
	u.sourceItems = v
}

// unbindSourceItemsBestEffort DeleteDocumentMirror 内的 best-effort 回调。
// 删档成功后才调用；失败仅 Warn——W3 dangling 兜底仅在「源指纹漂移触发
// needsWork」的轮次到达（runner 判据含 DocumentID==""，绑定保留时不满足），
// 静态源场景下永不自动收敛（与包级头注一致）。
func (u *Usecase) unbindSourceItemsBestEffort(ctx context.Context, docID string) {
	if u == nil || u.sourceItems == nil {
		return
	}
	n, err := u.sourceItems.UnbindItemsByDocumentID(ctx, docID)
	if err != nil {
		u.lg.Warn("sourcesync 带外删档反扫失败（悬空绑定将留待该源下次指纹漂移重摄取经 W3 dangling 兜底；静态源不自愈，需人工重扫或重删档触发）",
			loggateway.StepID("knowledge.sourcesync.unbind"),
			loggateway.Str("doc_id", docID),
			loggateway.Str("heal", "on_next_source_drift_only"),
			loggateway.Err(err),
		)
		return
	}
	if n > 0 {
		u.lg.Info("sourcesync 带外删档反扫完成",
			loggateway.StepID("knowledge.sourcesync.unbind"),
			loggateway.Str("doc_id", docID),
			loggateway.Int("rows", n),
		)
	}
}
