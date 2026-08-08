package service

import (
	"context"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
)

// G5-F 实体治理 RPC（B10 合并 / B11 建议）。
// 数据流：proto → biz Usecase（MergeEntities / 建议计算）→ proto；流程日志
// step knowledge.entity.merge（K1 入口/出口 + K2 错误路径）。

// MergeKnowledgeEntities merges mergee entities into the keeper atomically
// (G5-F B10); returns rewrite counts for inline UI feedback.
func (s *KnowledgeService) MergeKnowledgeEntities(ctx context.Context, req *v1.MergeKnowledgeEntitiesRequest) (*v1.MergeKnowledgeEntitiesResponse, error) {
	if req.GetKeeperId() <= 0 {
		return nil, apierror.BadRequest("KNOWLEDGE", "keeper_id is required")
	}
	if len(req.GetMergeeIds()) == 0 {
		return nil, apierror.BadRequest("KNOWLEDGE", "mergee_ids is required")
	}
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}

	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.entity.merge", "知识实体合并",
		event.P("collection_id", col.ID),
		event.P("keeper_id", req.GetKeeperId()),
		event.P("mergee_ids", len(req.GetMergeeIds())))

	res, err := s.uc.MergeEntities(ctx, col.ID, req.GetKeeperId(), req.GetMergeeIds())
	if err != nil {
		flow.LogError("knowledge.entity.merge", "知识实体合并失败",
			event.P("collection_id", col.ID),
			event.P("keeper_id", req.GetKeeperId()),
			event.P("error", err.Error()))
		return nil, err
	}
	flow.LogDone("knowledge.entity.merge", "知识实体合并完成",
		event.P("collection_id", col.ID),
		event.P("merged_entities", res.MergedEntities),
		event.P("rewritten_mentions", res.RewrittenMentions),
		event.P("rewritten_links", res.RewrittenLinks))
	return &v1.MergeKnowledgeEntitiesResponse{
		RewrittenMentions: int32(res.RewrittenMentions),
		RewrittenLinks:    int32(res.RewrittenLinks),
		MergedEntities:    int32(res.MergedEntities),
	}, nil
}

// ListEntityMergeSuggestions lists entity merge candidates (G5-F B11): norm
// conflict groups first, then embedding-similar pairs by similarity desc.
// Embedder nil/failure degrades to norm-only (NFR-15); read-only, no flow log.
func (s *KnowledgeService) ListEntityMergeSuggestions(ctx context.Context, req *v1.ListEntityMergeSuggestionsRequest) (*v1.ListEntityMergeSuggestionsResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	suggestions, err := s.uc.ListEntityMergeSuggestions(ctx, col.ID, s.embedder)
	if err != nil {
		return nil, err
	}
	out := &v1.ListEntityMergeSuggestionsResponse{Items: make([]*v1.EntityMergeSuggestion, 0, len(suggestions))}
	for _, sg := range suggestions {
		out.Items = append(out.Items, &v1.EntityMergeSuggestion{
			KeeperId:   sg.KeeperID,
			KeeperName: sg.KeeperName,
			MergeeId:   sg.MergeeID,
			MergeeName: sg.MergeeName,
			Source:     sg.Source,
			Similarity: sg.Similarity,
			Tier:       sg.Tier,
		})
	}
	return out, nil
}
