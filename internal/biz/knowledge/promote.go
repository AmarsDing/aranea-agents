package knowledge

import (
	"context"
	"strings"

	"aranea-agents/internal/knowledge/blockparse"
	"aranea-agents/pkg/apierror"
)

// SP1-G 晋升（US-27，设计 S7）：个人库块复制（非移动）到团队库，谱系对
// （promoted_from/promoted_to）+ 级联提示 + 目标文档派生索引即时重放。
//
// 实现路径（对 S7 的两处简化，见 development 文档记录）：
//   - 块克隆经「全文追加 + 目标文档重解析」实现（非直接 INSERT 块行）——
//     knowledge_blocks 是派生索引（整文档删了重插），直插块行会在下次重放丢失；
//     追加原文后重放让新块自然生成，谱系按「尾部 N 块按序对应」回写。
//   - 无单一大事务：逐目标文档原子（文档写 + 块重放各自事务），与 ingest 同
//     哲学（主流程 + 派生索引失败降级）。meta.private_external 列不建——引用
//     私有块的目标经可见性过滤自然落 dangling（raw_target 保留即占位语义）。

// ErrPromoteTargetNotTeam 晋升目标库必须是 team 后端（SP1-G 校验）。
var ErrPromoteTargetNotTeam = apierror.BadRequest("KNOWLEDGE", "target collection must be a team vault")

// ErrPromoteBlockNotFound 晋升块缺失（任一 block_id 查不到即整体拒绝）。
var ErrPromoteBlockNotFound = apierror.BadRequest("KNOWLEDGE", "some block_ids not found")

// ErrPromoteSameCollection 源块已在目标库（同库晋升无意义）。
var ErrPromoteSameCollection = apierror.BadRequest("KNOWLEDGE", "blocks already in target collection")

// PromoteLineage 源→克隆谱系对（US-27 验收 41）。
type PromoteLineage struct {
	SrcBlockID  string // 源块（回写 promoted_to）
	NewBlockID  string // 团队库新块（回写 promoted_from）
	TargetDocID string
}

// PromoteCascadeCandidate 级联提示：晋升块引用了未一并晋升的私有目标。
// 团队侧该引用落 dangling（raw_target 保留），前端据此提示「私有外部引用」。
type PromoteCascadeCandidate struct {
	SrcBlockID      string
	RawTarget       string
	DstDocID        string
	DstCollectionID string
}

// PromoteTouchedDoc 晋升触及的目标文档。Created 标记新建（service 层据此
// 校正 collection counts：新建 docDelta=1，既有仅 chunkDelta）。
type PromoteTouchedDoc struct {
	DocID   string
	Created bool
}

// PromoteResult 晋升产物。TouchedTargetDocs 供 service 层重放 chunk/FTS
// （B-3：晋升完成即可检索）。
type PromoteResult struct {
	CreatedBlocks     []PromoteLineage
	CascadeCandidates []PromoteCascadeCandidate
	TouchedTargetDocs []PromoteTouchedDoc
}

// PromoteBlockReader 晋升读端口（data 层实现于 knowledgeBlockRepo）。
// Stability:evolving
type PromoteBlockReader interface {
	// GetBlocksByIDs 按 ID 批量查块（含 doc/collection 归属与 content_hash）。
	GetBlocksByIDs(ctx context.Context, ids []string) ([]KnowledgeBlock, error)
	// ListOutEdgesByBlocks 块的出向引用边（cascade 候选检测）。
	ListOutEdgesByBlocks(ctx context.Context, blockIDs []string) ([]KnowledgeBlockRefEdge, error)
}

// PromoteLineageWriter 谱系回写端口（data 层实现于 knowledgeBlockRepo）。
// Stability:evolving
type PromoteLineageWriter interface {
	// WritePromoteLineage 单事务回写谱系对：新块 promoted_from=SrcBlockID，
	// 源块 promoted_to=NewBlockID。
	WritePromoteLineage(ctx context.Context, pairs []PromoteLineage) error
}

// SetPromoteRepos 接线晋升端口（SP1-G；可选能力，未接线时 PromoteBlocks
// 返回 ErrUnavailable）。装配同 SetBlockIndexRepos 模式（knowledgeBlockRepo
// 类型断言双端口）。
func (u *Usecase) SetPromoteRepos(reader PromoteBlockReader, writer PromoteLineageWriter) {
	u.promoteReader = reader
	u.promoteWriter = writer
}

// PromoteBlocks 把 blockIDs 克隆到目标 team 库：目标文档按源 rel_path 同名
// 查找或新建，块全文追加到正文尾部后重放块级派生索引，谱系按尾部 N 块按序
// 对应回写。返回谱系对与 cascade 候选（引用私有目标未一并晋升）。
func (u *Usecase) PromoteBlocks(ctx context.Context, blockIDs []string, targetCollectionID string) (PromoteResult, error) {
	if err := u.requireRepo(); err != nil {
		return PromoteResult{}, err
	}
	if u.promoteReader == nil || u.promoteWriter == nil {
		return PromoteResult{}, ErrUnavailable
	}
	if len(blockIDs) == 0 {
		return PromoteResult{}, apierror.BadRequest("KNOWLEDGE", "block_ids is required")
	}
	target, err := u.collections.GetCollection(ctx, targetCollectionID)
	if err != nil {
		return PromoteResult{}, err
	}
	if target.VaultBackend != VaultBackendTeam {
		return PromoteResult{}, ErrPromoteTargetNotTeam
	}
	blocks, err := u.promoteReader.GetBlocksByIDs(ctx, blockIDs)
	if err != nil {
		return PromoteResult{}, err
	}
	if len(blocks) != len(dedupeStrings(blockIDs)) {
		return PromoteResult{}, ErrPromoteBlockNotFound
	}
	// 按输入序重排（GetBlocksByIDs 不保证顺序）并按源文档分组。
	byID := make(map[string]KnowledgeBlock, len(blocks))
	for _, b := range blocks {
		if b.CollectionID == targetCollectionID {
			return PromoteResult{}, ErrPromoteSameCollection
		}
		byID[b.ID] = b
	}
	ordered := make([]KnowledgeBlock, 0, len(blockIDs))
	seen := map[string]bool{}
	var docOrder []string
	byDoc := map[string][]KnowledgeBlock{}
	for _, id := range blockIDs {
		b, ok := byID[id]
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, b)
		if _, ok := byDoc[b.DocID]; !ok {
			docOrder = append(docOrder, b.DocID)
		}
		byDoc[b.DocID] = append(byDoc[b.DocID], b)
	}

	var res PromoteResult
	for _, srcDocID := range docOrder {
		docBlocks := byDoc[srcDocID]
		touched, err := u.promoteDocBlocks(ctx, target, srcDocID, docBlocks, &res)
		if err != nil {
			return res, err
		}
		res.TouchedTargetDocs = append(res.TouchedTargetDocs, touched)
	}

	// cascade 候选：出边指向目标库以外的已解析目标（私有库文档/块）。
	edges, err := u.promoteReader.ListOutEdgesByBlocks(ctx, blockIDs)
	if err != nil {
		return res, err
	}
	for _, e := range edges {
		if e.DstDocID == "" || e.DstCollectionID == targetCollectionID {
			continue
		}
		res.CascadeCandidates = append(res.CascadeCandidates, PromoteCascadeCandidate{
			SrcBlockID:      e.SrcBlockID,
			RawTarget:       e.RawTarget,
			DstDocID:        e.DstDocID,
			DstCollectionID: e.DstCollectionID,
		})
	}
	return res, nil
}

// promoteDocBlocks 单源文档的晋升单元：提取块全文 → 目标文档 find-or-create
// + 尾部追加 → 块级索引重放 → 尾部 N 块按序对应回写谱系。
func (u *Usecase) promoteDocBlocks(ctx context.Context, target Collection, srcDocID string, docBlocks []KnowledgeBlock, res *PromoteResult) (PromoteTouchedDoc, error) {
	srcDoc, err := u.documents.GetDocument(ctx, srcDocID)
	if err != nil {
		return PromoteTouchedDoc{}, err
	}
	rows, _, _ := blockparse.Parse(srcDocID, []byte(srcDoc.ContentText))
	textByOrdinal := make(map[int]string, len(rows))
	for _, r := range rows {
		textByOrdinal[r.Ordinal] = r.Text
	}
	texts := make([]string, 0, len(docBlocks))
	for _, b := range docBlocks {
		text, ok := textByOrdinal[b.Ordinal]
		if !ok || strings.TrimSpace(text) == "" {
			return PromoteTouchedDoc{}, apierror.BadRequest("KNOWLEDGE", "block %s ordinal %d not found in doc %s", b.ID, b.Ordinal, srcDocID)
		}
		texts = append(texts, text)
	}
	appendix := strings.Join(texts, "\n\n") + "\n"

	// 目标文档：按 rel_path 同名查找或新建（源 rel_path 空 = 非 vault 文档，直接新建）。
	var targetDoc Document
	if srcDoc.RelPath != "" {
		targetDoc, err = u.documents.GetDocumentByRelPath(ctx, target.ID, srcDoc.RelPath)
		if err != nil && !apierror.IsCode(err, apierror.CodeNotFound) {
			return PromoteTouchedDoc{}, err
		}
	}
	created := targetDoc.ID == ""
	var newContent string
	if created {
		source := strings.TrimSpace(srcDoc.Source)
		if source == "" {
			source = srcDoc.RelPath
		}
		if source == "" {
			source = "promoted.md"
		}
		targetDoc, err = u.CreateDocument(ctx, Document{
			CollectionID: target.ID,
			RelPath:      srcDoc.RelPath,
			Source:       source,
			MimeType:     srcDoc.MimeType,
			ContentText:  appendix,
			Organized:    true,
			Status:       "pending", // chunk/FTS 由 service 层收尾（B-3）
		})
		if err != nil {
			return PromoteTouchedDoc{}, err
		}
		newContent = appendix
	} else {
		newContent = strings.TrimRight(targetDoc.ContentText, "\n") + "\n\n" + appendix
		if err := u.documents.UpdateDocumentContent(ctx, targetDoc.ID, newContent, targetDoc.Organized); err != nil {
			return PromoteTouchedDoc{}, err
		}
	}

	// 目标文档派生索引同事务级重放（块/refs/explicit 投影 + LinkIndex 增量）。
	if err := u.RebuildBlockIndex(ctx, target.ID, targetDoc.ID, newContent); err != nil {
		return PromoteTouchedDoc{}, err
	}
	// 谱系：追加块文本按源块顺序拼接，重放后尾部 N 块与之按序一一对应。
	newBlocks, err := u.blockIndex.ListDocBlocks(ctx, targetDoc.ID)
	if err != nil {
		return PromoteTouchedDoc{}, err
	}
	if len(newBlocks) < len(docBlocks) {
		return PromoteTouchedDoc{}, apierror.Internal("KNOWLEDGE", "promote replay produced %d blocks, want at least %d", len(newBlocks), len(docBlocks))
	}
	tail := newBlocks[len(newBlocks)-len(docBlocks):]
	pairs := make([]PromoteLineage, 0, len(docBlocks))
	for i, sb := range docBlocks {
		pairs = append(pairs, PromoteLineage{
			SrcBlockID:  sb.ID,
			NewBlockID:  tail[i].ID,
			TargetDocID: targetDoc.ID,
		})
	}
	if err := u.promoteWriter.WritePromoteLineage(ctx, pairs); err != nil {
		return PromoteTouchedDoc{}, err
	}
	res.CreatedBlocks = append(res.CreatedBlocks, pairs...)
	return PromoteTouchedDoc{DocID: targetDoc.ID, Created: created}, nil
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
