package service

import (
	"context"
	"encoding/json"
	"time"

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

// ── M4 自治理层：治理提案人工二审 RPC ─────────────────────────────────────
// ListGovernanceProposals 是提案只读出口（此前 pending 提案只写不读，死信堆积）；
// ResolveGovernanceProposal 是二审闭环（pending → applied/rejected）。
// 权限模型：list 指定 collection_id 时校验读权限，空 = 平台管理视图（与记忆管家
// governance_* 工具同规）；resolve 为平台级治理动作（提案落点即团队知识库，
// 与 dream_cycle curate 的全局语义一致）。

// ListGovernanceProposals lists curation governance proposals (M4 人工二审出口).
// Read-only, no flow log. status/limit 收口在 biz 层（非法 status → CodeBadRequest）。
func (s *KnowledgeService) ListGovernanceProposals(ctx context.Context, req *v1.ListGovernanceProposalsRequest) (*v1.ListGovernanceProposalsResponse, error) {
	if req.GetCollectionId() != "" {
		col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
		if err != nil {
			return nil, err
		}
		if err := s.assertCollectionAccess(ctx, col); err != nil {
			return nil, err
		}
	}
	views, err := s.uc.ListGovernanceProposals(ctx, req.GetCollectionId(), req.GetStatus(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := &v1.ListGovernanceProposalsResponse{Items: make([]*v1.GovernanceProposal, 0, len(views))}
	for _, v := range views {
		payloadJSON, _ := json.Marshal(v.Payload)
		item := &v1.GovernanceProposal{
			Id:           v.ID,
			CollectionId: v.CollectionID,
			Kind:         v.Kind,
			Risk:         v.Risk,
			Status:       v.Status,
			PayloadJson:  string(payloadJSON),
			CreatedAt:    v.CreatedAt.UTC().Format(time.RFC3339),
		}
		if !v.ResolvedAt.IsZero() {
			item.ResolvedAt = v.ResolvedAt.UTC().Format(time.RFC3339)
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// ResolveGovernanceProposal closes one pending proposal (M4 人工二审):
// decision=applied 批准治理动作，rejected 驳回。流程日志 step
// knowledge.governance.resolve（K1 入口/出口 + K2 错误路径）。
func (s *KnowledgeService) ResolveGovernanceProposal(ctx context.Context, req *v1.ResolveGovernanceProposalRequest) (*v1.ResolveGovernanceProposalResponse, error) {
	if req.GetId() <= 0 {
		return nil, apierror.BadRequest("KNOWLEDGE", "id is required")
	}
	decision := req.GetDecision()
	if decision != "applied" && decision != "rejected" {
		return nil, apierror.BadRequest("KNOWLEDGE", "decision must be applied or rejected")
	}

	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.governance.resolve", "治理提案人工二审",
		event.P("proposal_id", req.GetId()),
		event.P("decision", decision))

	if err := s.uc.ResolveGovernanceProposal(ctx, req.GetId(), decision); err != nil {
		flow.LogError("knowledge.governance.resolve", "治理提案二审失败",
			event.P("proposal_id", req.GetId()),
			event.P("error", err.Error()))
		return nil, err
	}
	flow.LogDone("knowledge.governance.resolve", "治理提案二审完成",
		event.P("proposal_id", req.GetId()),
		event.P("decision", decision))
	return &v1.ResolveGovernanceProposalResponse{Id: req.GetId(), Status: decision}, nil
}
