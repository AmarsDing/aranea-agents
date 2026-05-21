package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
)

// NewKnowledgeRetriever wires embedder, repo, and optional env-configured reranker (KN-01).
func NewKnowledgeRetriever(emb *knowledge.Embedder, repo biz.KnowledgeRepo) *knowledge.Retriever {
	if emb == nil || repo == nil {
		return nil
	}
	rr, err := knowledge.NewRerankerFromEnv()
	if err != nil {
		event.SysLogWarn("knowledge.reranker.config", "重排器配置无效，已禁用",
			event.P("error", err.Error()))
		rr = nil
	}
	return knowledge.NewRetriever(emb, repo, rr)
}
