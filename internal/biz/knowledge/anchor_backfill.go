package knowledge

import (
	"context"

	"aranea-agents/internal/knowledge/blockparse"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// SP1-H H-2 惰性锚点回填执行侧（设计 S2/F-SP1-10）：
// 写路径 Resolver 检出 heading-path 引用命中未锚块后，向目标文档源文本行尾
// 追加 " ^<uuid7>"（块 ID 即锚文本，库内唯一由部分唯一索引保证）。
//
// 存储后端分流（设计 S6）：
//   - team 库 / 非 vault 文档：PG content_text 即真相源 → UpdateDocumentContent；
//   - local 库：文件系统即真相源 → VaultFiler CAS 写文件 + 镜像正文/hash 同步。
//     hash 同步后下轮同步轮询按幂等短路跳过——回填不触发 chunks/embedding 重建
//     （blockparse 块 content_hash 锚稳定契约的文档级对应）。
//
// 语义边界：
//   - 幂等：AppendHeadingAnchor 复查兜底（并发/滞后窗口内已锚则跳过）；
//   - 一跳即止：回填自触发重索引 allowBackfill=false，不级联改写第三方文档；
//   - best-effort：失败仅记 Warn（K3），不回滚主流程、不重试——目标文档下次
//     写路径自愈；源文档指向旧未锚块 ID 的边经 FK SET NULL 转块级 dangling，
//     同样在源文档下次写路径重解析时愈合（最终一致，SP1-ADR-3）。

// newAnchorID 生成回填锚点（^<uuid7>；锚字符集 [A-Za-z0-9_-] 兼容解析正则）。
func newAnchorID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// backfillAnchors 逐请求执行回填；单请求失败不阻塞其余（best-effort）。
func (u *Usecase) backfillAnchors(ctx context.Context, reqs []AnchorBackfillRequest) {
	for _, req := range reqs {
		if err := u.backfillOne(ctx, req); err != nil {
			u.lg.Warn("knowledge anchor backfill failed",
				loggateway.Str("collection_id", req.CollectionID),
				loggateway.Str("doc_id", req.DocID),
				loggateway.Err(err),
			)
		}
	}
}

// backfillOne 回填单个目标文档的指定标题路径。
func (u *Usecase) backfillOne(ctx context.Context, req AnchorBackfillRequest) error {
	if req.DocID == "" || len(req.HeadingPath) == 0 {
		return nil
	}
	doc, err := u.documents.GetDocument(ctx, req.DocID)
	if err != nil {
		return err
	}
	col, err := u.collections.GetCollection(ctx, req.CollectionID)
	if err != nil {
		return err
	}
	if col.VaultBackend == VaultBackendTeam || doc.RelPath == "" || col.RootPath == "" {
		return u.backfillTeamDoc(ctx, col.ID, doc, req.HeadingPath)
	}
	return u.backfillLocalDoc(ctx, col, doc, req.HeadingPath)
}

// backfillTeamDoc PG 真相源落锚：正文更新 + 目标重索引（一跳即止）。
func (u *Usecase) backfillTeamDoc(ctx context.Context, collectionID string, doc Document, path []string) error {
	newBody, changed := blockparse.AppendHeadingAnchor([]byte(doc.ContentText), path, newAnchorID())
	if !changed {
		return nil
	}
	if err := u.documents.UpdateDocumentContent(ctx, doc.ID, string(newBody), doc.Organized); err != nil {
		return err
	}
	return u.rebuildBlockIndex(ctx, collectionID, doc.ID, string(newBody), nil, false)
}

// backfillLocalDoc 文件系统真相源落锚：CAS 写文件 + 镜像正文/hash 同步 + 目标重索引。
// CAS 冲突（并发窗口内外部已改）时保守放弃本轮镜像同步与重索引——文件仍已写入，
// 下轮轮询按 hash 差异全量重放收敛（含 chunks 重建）。
func (u *Usecase) backfillLocalDoc(ctx context.Context, col Collection, doc Document, path []string) error {
	if u.filer == nil {
		return nil // filer 未接线：降级跳过（下轮同步/写路径自愈）
	}
	vd, hash, err := u.filer.ReadDocWithHash(col.RootPath, doc.RelPath)
	if err != nil {
		return err
	}
	newBody, changed := blockparse.AppendHeadingAnchor([]byte(vd.Body), path, newAnchorID())
	if !changed {
		return nil
	}
	vd.Body = string(newBody)
	conflict, err := u.filer.WriteDocCAS(col.RootPath, doc.RelPath, vd, hash)
	if err != nil {
		return err
	}
	if conflict {
		return nil
	}
	if err := u.documents.UpdateDocumentContent(ctx, doc.ID, string(newBody), doc.Organized); err != nil {
		return err
	}
	if err := u.documents.UpdateDocumentSyncMeta(ctx, doc.ID, DocumentSyncMeta{
		ContentHash: HashContent(marshalVaultDoc(vd)),
		Summary:     vd.Frontmatter.Summary,
		SummaryHash: vd.Frontmatter.SummaryHash,
		Tags:        vd.Frontmatter.Tags,
		DocType:     vd.Frontmatter.Type,
	}); err != nil {
		return err
	}
	return u.rebuildBlockIndex(ctx, col.ID, doc.ID, string(newBody), nil, false)
}
