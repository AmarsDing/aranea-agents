package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// NewKnowledgeRetriever wires embedder, repo, and optional env-configured reranker (KN-01).
func NewKnowledgeRetriever(emb knowledge.QueryEmbedder, repo biz.KnowledgeRepo, lg loggateway.Logger) *knowledge.Retriever {
	if emb == nil || repo == nil {
		return nil
	}
	rr, err := knowledge.NewRerankerFromEnv()
	if err != nil {
		lg.Warn("重排器配置无效，已禁用",
			loggateway.StepID("knowledge.reranker.config"),
			loggateway.Err(err),
		)
		rr = nil
	}
	return knowledge.NewRetriever(emb, repo, rr, lg)
}
