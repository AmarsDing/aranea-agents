package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
)

// NewKnowledgeRetriever wires embedder, repo, and optional env-configured reranker (KN-01).
func NewKnowledgeRetriever(emb *knowledge.Embedder, repo biz.KnowledgeRepo) *knowledge.Retriever {
	if emb == nil || repo == nil {
		return nil
	}
	rr, err := knowledge.NewRerankerFromEnv()
	if err != nil {
		rr = nil
	}
	return knowledge.NewRetriever(emb, repo, rr)
}
