package knowledge

import (
	"context"

	"aranea-agents/internal/knowledge/blockparse"
)

// SP1-C 写路径编排（设计 S3/S4）：blockparse 纯函数解析 → LinkResolver 跨库解析 →
// BlockIndexRepo 物化 → explicit 文档轨投影（N-3 权重聚合）。
// blockparse 为纯函数叶包（无 IO、无项目内依赖），biz 直接引用不分层违规。

// SetBlockIndexRepos 接线块级派生索引端口（可选能力；未接线时 RebuildBlockIndex
// 降级 no-op，explicit 轨也不再重建——生产装配必须同时接线，见 ProvideKnowledgeUsecase）。
func (u *Usecase) SetBlockIndexRepos(blocks BlockIndexRepo, idx ResolveIndex) {
	u.blockIndex = blocks
	u.resolveIndex = idx
}

// RebuildBlockIndex 重建文档的块级派生索引（块 + 引用边 + explicit 文档轨投影 +
// 解析键 title/aliases 物化）。Vault 同步索引成功后 / 移动入链修复 / team 摄取
// 成功后调用。失败返回 error 供调用方降级记日志（不回滚主流程，最终一致）。
// 写路径语义：Resolver 检出 heading-path 引用命中的未锚块后执行惰性锚点回填
// （SP1-H/F-SP1-10，best-effort 副作用，不影响本重建的返回）。
func (u *Usecase) RebuildBlockIndex(ctx context.Context, collectionID, docID, body string) error {
	return u.rebuildBlockIndex(ctx, collectionID, docID, body, nil, true)
}

// ListDocsMissingBlockIndex 列出「已索引但块索引缺失」的文档 ID（SP2 #4 下游收敛
// 校验）；未接线块端口时返回空（收敛扫描 no-op）。
func (u *Usecase) ListDocsMissingBlockIndex(ctx context.Context, collectionID string, limit int) ([]string, error) {
	if u == nil || u.blockIndex == nil {
		return nil, nil
	}
	return u.blockIndex.ListDocsMissingBlockIndex(ctx, collectionID, limit)
}

// rebuildBlockIndex 内部实现：visible 为预解析可见集合集（SP1-H 全量重建整批
// 提升一次）；nil 时按文档现查（单文档写路径语义不变）。
// allowBackfill 区分写路径（true，执行惰性锚点回填）与全量重建/回填自触发
// 重索引（false——重建是索引修复不改源文本；回填一跳即止不级联）。
func (u *Usecase) rebuildBlockIndex(ctx context.Context, collectionID, docID, body string, visible []string, allowBackfill bool) error {
	if u == nil || u.blockIndex == nil {
		return nil
	}
	meta := blockparse.ParseDocMeta([]byte(body))
	rows, refRows, _ := blockparse.Parse(docID, []byte(body)) // err 恒 nil（容错解析契约）
	blocks := toBizBlocks(rows)
	refs := toBizRefInputs(refRows)
	var backfills []AnchorBackfillRequest
	if len(refs) > 0 && u.resolveIndex != nil {
		if visible == nil {
			var err error
			visible, err = u.visibleCollectionIDs(ctx, collectionID)
			if err != nil {
				return err
			}
		}
		var err error
		refs, backfills, err = NewLinkResolver(u.resolveIndex).ResolveRefs(ctx, collectionID, docID, visible, refs, blocks)
		if err != nil {
			return err
		}
	}
	edges, err := u.blockIndex.ReplaceDocBlocks(ctx, collectionID, docID, blocks, refs)
	if err != nil {
		return err
	}
	if err := u.blockIndex.UpdateDocLinkKeys(ctx, docID, meta.Title, meta.Aliases); err != nil {
		return err
	}
	if err := u.projectExplicitLinks(ctx, collectionID, docID, refs); err != nil {
		return err
	}
	u.applyLinkIndex(ctx, docID, edges)
	// 主流程物化完成后执行回填副作用（SP1-H）：失败不回滚、不重试，
	// 目标文档下次写路径自愈（幂等）。
	if allowBackfill {
		u.backfillAnchors(ctx, backfills)
	}
	return nil
}

// visibleCollectionIDs 解析可见集合集（B-1 ②）：后台索引无「当前用户」，
// 取文档所在 workspace 的全部集合（跨库解析的工作区边界；读侧片段级权限属 SP5）。
func (u *Usecase) visibleCollectionIDs(ctx context.Context, collectionID string) ([]string, error) {
	col, err := u.collections.GetCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	cols, _, err := u.collections.ListCollections(ctx, col.Workspace, maxLinkCandidates, 0)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(cols))
	for _, c := range cols {
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		ids = []string{collectionID}
	}
	return ids, nil
}

// projectExplicitLinks 把块级 refs 投影为文档级 explicit 边（设计 S4 并存策略 +
// N-3 权重规则）：同 (src_doc, dst_doc) 多条块边聚合为一条，Weight = 块边数；
// dangling（DstDocID 空）与文档级自环不投影（块内自引用保留在 refs 层）。
// Context 取首个 raw_target（关联区展示沿用既有语义）。links 未接线时 no-op。
func (u *Usecase) projectExplicitLinks(ctx context.Context, collectionID, docID string, refs []KnowledgeBlockRefInput) error {
	if u == nil || u.links == nil {
		return nil
	}
	type agg struct {
		weight   int
		firstRaw string
	}
	byDst := make(map[string]*agg)
	var order []string
	for _, rf := range refs {
		if rf.DstDocID == "" || rf.DstDocID == docID {
			continue
		}
		a, ok := byDst[rf.DstDocID]
		if !ok {
			a = &agg{firstRaw: rf.RawTarget}
			byDst[rf.DstDocID] = a
			order = append(order, rf.DstDocID)
		}
		a.weight++
	}
	links := make([]Link, 0, len(order))
	for _, dst := range order {
		links = append(links, Link{
			CollectionID: collectionID,
			DocID:        docID,
			TargetDocID:  dst,
			LinkType:     LinkTypeExplicit,
			Context:      byDst[dst].firstRaw,
			Weight:       byDst[dst].weight,
		})
	}
	return u.links.ReplaceLinks(ctx, collectionID, docID, LinkTypeExplicit, links)
}

func toBizBlocks(rows []blockparse.BlockRow) []KnowledgeBlock {
	out := make([]KnowledgeBlock, len(rows))
	for i, r := range rows {
		out[i] = KnowledgeBlock{
			Ordinal:     r.Ordinal,
			Kind:        string(r.Kind),
			Anchor:      r.Anchor,
			HeadingPath: r.HeadingPath,
			ContentHash: r.ContentHash,
			TextExcerpt: r.TextExcerpt,
		}
	}
	return out
}

func toBizRefInputs(rows []blockparse.RefRow) []KnowledgeBlockRefInput {
	out := make([]KnowledgeBlockRefInput, len(rows))
	for i, r := range rows {
		out[i] = KnowledgeBlockRefInput{
			SrcOrdinal: r.SrcOrdinal,
			RawTarget:  r.RawTarget,
			Alias:      r.Alias,
			EdgeType:   string(r.EdgeType),
			Context:    r.Context,
		}
	}
	return out
}
