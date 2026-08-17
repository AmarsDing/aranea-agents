package service

import (
	"context"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// triggerKnowledgeGraph closes graph extraction for database-backed documents.
// The injected hook is asynchronous and content-hash idempotent; failures are
// observable but never roll back an already searchable document.
func (s *KnowledgeService) triggerKnowledgeGraph(
	ctx context.Context,
	col biz.KnowledgeCollection,
	docs []bizknowledge.PromoteTouchedDoc,
) {
	if s == nil || s.writeBackGraph == nil || len(docs) == 0 {
		return
	}
	if err := s.writeBackGraph(ctx, col, docs); err != nil {
		s.lg.Warn("知识文档图谱抽取触发失败（文档已可检索，后续幂等重试）",
			loggateway.StepID("knowledge.graph.extract"),
			loggateway.Str("collection_id", col.ID),
			loggateway.Err(err))
	}
}
