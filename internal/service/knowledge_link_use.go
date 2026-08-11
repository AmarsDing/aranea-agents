package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/knowledge/v1"
)

// ── B4 #8：wikilink 落链 recency（[[ 补全最近引用排序） ─────────────────────

// RecordLinkUse upserts the (collection, doc) recency row when a wikilink
// completion is applied. Best-effort: biz degrades to no-op when the port is
// not wired; callers may ignore failures.
func (s *KnowledgeService) RecordLinkUse(ctx context.Context, req *v1.RecordLinkUseRequest) (*v1.RecordLinkUseResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	if err := s.uc.RecordLinkUse(ctx, req.GetCollectionId(), req.GetDocId()); err != nil {
		return nil, err
	}
	return &v1.RecordLinkUseResponse{}, nil
}

// ListRecentLinkUses returns recently used wikilink targets of one collection,
// most recent first; drives empty-query [[ completion ordering on the client.
func (s *KnowledgeService) ListRecentLinkUses(ctx context.Context, req *v1.ListRecentLinkUsesRequest) (*v1.ListRecentLinkUsesResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	items, err := s.uc.ListRecentLinkUses(ctx, req.GetCollectionId(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.LinkUseEntry, 0, len(items))
	for _, it := range items {
		lastUsed := ""
		if !it.LastUsedAt.IsZero() {
			lastUsed = it.LastUsedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, &v1.LinkUseEntry{DocId: it.DocID, LastUsedAt: lastUsed})
	}
	return &v1.ListRecentLinkUsesResponse{Items: out}, nil
}
