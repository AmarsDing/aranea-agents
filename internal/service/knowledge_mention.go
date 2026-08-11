package service

import (
	"context"

	v1 "aranea-agents/api/kratos/knowledge/v1"
)

// ── P2-7：unlinked mentions（未链接提及） ────────────────────────────────────

// ListUnlinkedMentions returns documents that mention the target doc's display
// name in plain text (outside [[wikilinks]]) without linking to it.
func (s *KnowledgeService) ListUnlinkedMentions(ctx context.Context, req *v1.ListUnlinkedMentionsRequest) (*v1.ListUnlinkedMentionsResponse, error) {
	doc, err := s.uc.GetDocument(ctx, req.GetDocId())
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
	items, err := s.uc.ListUnlinkedMentions(ctx, req.GetDocId())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.UnlinkedMention, 0, len(items))
	for _, m := range items {
		out = append(out, &v1.UnlinkedMention{
			SrcDocId:   m.SrcDocID,
			SrcDocName: m.SrcDocName,
			Count:      int32(m.Count),
			Snippet:    m.Snippet,
		})
	}
	return &v1.ListUnlinkedMentionsResponse{Items: out}, nil
}
