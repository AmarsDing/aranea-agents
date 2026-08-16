package service

import (
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
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
	ret := knowledge.NewRetriever(emb, repo, rr, lg)
	// P2-c：access_log 记账下沉 Retriever——repo 实现 AccessLogRepo 时接线，
	// 覆盖 Router 缺席/退化全路径（只记不加成；Router 在役路径由 Router 自记，互斥）。
	if access, ok := repo.(bizknowledge.AccessLogRepo); ok {
		ret.SetAccessLog(access)
	}
	return ret
}
