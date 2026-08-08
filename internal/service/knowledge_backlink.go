package service

import (
	"context"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── SP1-E：块级反链 / dangling 聚合 API（设计 S5 读路径） ─────────────────────

// ListBlockBacklinks returns block-level inbound references (context + source
// doc/block per edge). doc_id binding takes precedence (aggregate all blocks of
// one document); block_id path resolves the owner doc for access assertion.
func (s *KnowledgeService) ListBlockBacklinks(ctx context.Context, req *v1.ListBlockBacklinksRequest) (*v1.ListBlockBacklinksResponse, error) {
	docID := req.GetDocId()
	if docID == "" {
		// 块级路径：先解析块归属文档做权限断言（块不存在 → NotFound 透传）。
		owner, err := s.uc.ResolveBlockOwnerDoc(ctx, req.GetBlockId())
		if err != nil {
			return nil, err
		}
		docID = owner
	}
	doc, err := s.uc.GetDocument(ctx, docID)
	if err != nil {
		return nil, err
	}
	col, err := s.uc.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	items, err := s.uc.ListBlockBacklinks(ctx, req.GetBlockId(), req.GetDocId())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.BlockBacklink, 0, len(items))
	for _, b := range items {
		out = append(out, toProtoBlockBacklink(b))
	}
	return &v1.ListBlockBacklinksResponse{Items: out}, nil
}

// ListDanglingLinks aggregates dangling references of one collection by
// raw_target with ref counts ("uncreated notes" view).
func (s *KnowledgeService) ListDanglingLinks(ctx context.Context, req *v1.ListDanglingLinksRequest) (*v1.ListDanglingLinksResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	items, err := s.uc.ListDanglingLinks(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.DanglingLink, 0, len(items))
	for _, d := range items {
		refs := make([]*v1.BlockBacklink, 0, len(d.Refs))
		for _, b := range d.Refs {
			refs = append(refs, toProtoBlockBacklink(b))
		}
		out = append(out, &v1.DanglingLink{
			RawTarget: d.RawTarget,
			RefCount:  int32(d.RefCount),
			Refs:      refs,
		})
	}
	return &v1.ListDanglingLinksResponse{Items: out}, nil
}

func toProtoBlockBacklink(b bizknowledge.BlockBacklink) *v1.BlockBacklink {
	return &v1.BlockBacklink{
		SrcBlockId:      b.SrcBlockID,
		SrcDocId:        b.SrcDocID,
		SrcCollectionId: b.SrcCollectionID,
		SrcDocName:      b.SrcDocName,
		RawTarget:       b.RawTarget,
		EdgeType:        b.EdgeType,
		Context:         b.Context,
		Ambiguous:       b.Ambiguous,
	}
}
