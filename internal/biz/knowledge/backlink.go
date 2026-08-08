package knowledge

import (
	"context"
	"sort"
	"strings"

	"aranea-agents/pkg/apierror"
)

// ── SP1-E：块级反链 / dangling 聚合查询（设计 S5 读路径） ─────────────────────
//
// 读路由：linkIndex 已加载（Loaded）→ 直读内存图（O(度数)）；启动窗口（未加载）
// → BlockLinkReader 落库兜底；双端口均未接线 → 空降级（派生索引缺席不阻断主流程）。
// SrcDocName 为展示增强：DocNameReader 批量解析，失败/缺失留空不阻塞主查询。
// 可见性：当前 API 无用户上下文，visible=nil（读侧片段级权限属 SP5，见设计 S5）。

// BlockBacklink 块级反链视图模型（一条入向引用边 + 源文档显示名）。
type BlockBacklink struct {
	SrcBlockID      string
	SrcDocID        string
	SrcCollectionID string
	SrcDocName      string // 源文档显示名（rel_path，缺失留空）
	RawTarget       string
	EdgeType        string // ref | embed
	Context         string
	Ambiguous       bool
}

// DanglingLink 悬空链聚合（「未创建笔记」视图）：同 raw_target 的引用归组，
// 目标文档创建并索引后自动复活。
type DanglingLink struct {
	RawTarget string
	RefCount  int
	Refs      []BlockBacklink
}

// BlockLinkReader 反链落库兜底端口（linkIndex 未加载时启用）。
// 实现方须保证 SrcDocID（经 src 块 JOIN）与 DstCollectionID（经 dst 文档 JOIN）填充。
// Stability:evolving
type BlockLinkReader interface {
	// ListBacklinksByBlock 块级反链（dst_block_id = blockID）。
	ListBacklinksByBlock(ctx context.Context, blockID string) ([]KnowledgeBlockRefEdge, error)
	// ListBacklinksByDoc 文档反链（dst_doc_id = docID；块级 + 文档级全部入边）。
	ListBacklinksByDoc(ctx context.Context, docID string) ([]KnowledgeBlockRefEdge, error)
	// ListDanglingEdges 集合内 dangling 边（dst_doc_id IS NULL）。
	ListDanglingEdges(ctx context.Context, collectionID string) ([]KnowledgeBlockRefEdge, error)
	// GetBlockOwnerDoc 块所属文档 id（service 层权限断言用）；块不存在返回 NotFound。
	GetBlockOwnerDoc(ctx context.Context, blockID string) (string, error)
}

// DocNameReader 源文档显示名批量解析（反链视图增强）。
// Stability:evolving
type DocNameReader interface {
	// ListDocumentNames 批量返回 docID → 显示名（rel_path 优先，空则 source）。
	ListDocumentNames(ctx context.Context, ids []string) (map[string]string, error)
}

// SetBacklinkRepos 接线反链读端口（SP1-E；可选能力）。reader 为启动窗口 DB 兜底，
// names 为源文档名解析；未接线时对应能力降级（空反链 / 名字留空）。
func (u *Usecase) SetBacklinkRepos(reader BlockLinkReader, names DocNameReader) {
	u.blockLinks = reader
	u.docNames = names
}

// SetBacklinkNames 单独补接源文档名解析（装配方 reader/names 来自不同 repo 时
// 分两步接线；见 ProvideKnowledgeUsecase）。
func (u *Usecase) SetBacklinkNames(names DocNameReader) {
	u.docNames = names
}

// ResolveBlockOwnerDoc 块所属文档 id（service 层权限断言前置）。blockLinks
// 未接线返回 NotFound（块索引未物化 = 块不可寻址）。
func (u *Usecase) ResolveBlockOwnerDoc(ctx context.Context, blockID string) (string, error) {
	blockID = strings.TrimSpace(blockID)
	if blockID == "" {
		return "", ErrIDRequired
	}
	if u.blockLinks == nil {
		return "", apierror.NotFound("knowledge", "block index unavailable: %s", blockID)
	}
	return u.blockLinks.GetBlockOwnerDoc(ctx, blockID)
}

// ListBlockBacklinks 块级反链：blockID 单块入边；docID 非空时优先聚合该文档
// 全部入边（块级 + 文档级）。输出按 (SrcDocID, SrcBlockID) 确定性排序。
func (u *Usecase) ListBlockBacklinks(ctx context.Context, blockID, docID string) ([]BlockBacklink, error) {
	blockID, docID = strings.TrimSpace(blockID), strings.TrimSpace(docID)
	if blockID == "" && docID == "" {
		return nil, ErrIDRequired
	}
	var edges []KnowledgeBlockRefEdge
	if u.linkIndex != nil && u.linkIndex.Loaded() {
		if docID != "" {
			edges = u.linkIndex.BacklinksByDoc(docID, nil)
		} else {
			edges = u.linkIndex.BacklinksByBlock(blockID, nil)
		}
	} else if u.blockLinks != nil {
		var err error
		if docID != "" {
			edges, err = u.blockLinks.ListBacklinksByDoc(ctx, docID)
		} else {
			edges, err = u.blockLinks.ListBacklinksByBlock(ctx, blockID)
		}
		if err != nil {
			return nil, err
		}
	}
	items := make([]BlockBacklink, 0, len(edges))
	for _, e := range edges {
		items = append(items, backlinkFromEdge(e))
	}
	sortBacklinks(items)
	u.fillBacklinkDocNames(ctx, items)
	return items, nil
}

// ListDanglingLinks 集合 dangling 聚合：raw_target 归组 + 引用计数。
// 组间 ref_count 降序、raw_target 字典序；组内 refs 按 (SrcDocID, SrcBlockID) 排序。
func (u *Usecase) ListDanglingLinks(ctx context.Context, collectionID string) ([]DanglingLink, error) {
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return nil, ErrCollectionIDRequired
	}
	var edges []KnowledgeBlockRefEdge
	if u.linkIndex != nil && u.linkIndex.Loaded() {
		edges = u.linkIndex.DanglingByCollection(collectionID, nil)
	} else if u.blockLinks != nil {
		var err error
		edges, err = u.blockLinks.ListDanglingEdges(ctx, collectionID)
		if err != nil {
			return nil, err
		}
	}
	groups := make(map[string][]BlockBacklink)
	for _, e := range edges {
		groups[e.RawTarget] = append(groups[e.RawTarget], backlinkFromEdge(e))
	}
	out := make([]DanglingLink, 0, len(groups))
	for raw, refs := range groups {
		sortBacklinks(refs)
		u.fillBacklinkDocNames(ctx, refs)
		out = append(out, DanglingLink{RawTarget: raw, RefCount: len(refs), Refs: refs})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RefCount != out[j].RefCount {
			return out[i].RefCount > out[j].RefCount
		}
		return out[i].RawTarget < out[j].RawTarget
	})
	return out, nil
}

// backlinkFromEdge 边 → 反链视图（名字后填）。
func backlinkFromEdge(e KnowledgeBlockRefEdge) BlockBacklink {
	return BlockBacklink{
		SrcBlockID:      e.SrcBlockID,
		SrcDocID:        e.SrcDocID,
		SrcCollectionID: e.CollectionID,
		RawTarget:       e.RawTarget,
		EdgeType:        e.EdgeType,
		Context:         e.Context,
		Ambiguous:       e.Ambiguous,
	}
}

// sortBacklinks (SrcDocID, SrcBlockID) 字典序（确定性输出契约）。
func sortBacklinks(items []BlockBacklink) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].SrcDocID != items[j].SrcDocID {
			return items[i].SrcDocID < items[j].SrcDocID
		}
		return items[i].SrcBlockID < items[j].SrcBlockID
	})
}

// fillBacklinkDocNames 批量填充源文档显示名（展示增强）。names 未接线或查询
// 失败时留空降级——反链主数据是边，名字缺席不阻断查询。
func (u *Usecase) fillBacklinkDocNames(ctx context.Context, items []BlockBacklink) {
	if u.docNames == nil || len(items) == 0 {
		return
	}
	seen := make(map[string]bool, len(items))
	ids := make([]string, 0, len(items))
	for _, it := range items {
		if it.SrcDocID != "" && !seen[it.SrcDocID] {
			seen[it.SrcDocID] = true
			ids = append(ids, it.SrcDocID)
		}
	}
	names, err := u.docNames.ListDocumentNames(ctx, ids)
	if err != nil {
		return
	}
	for i := range items {
		items[i].SrcDocName = names[items[i].SrcDocID]
	}
}
