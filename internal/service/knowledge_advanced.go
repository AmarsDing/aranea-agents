package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

func NewKnowledgeHybridRetriever(retriever *knowledge.Retriever, sparse biz.KnowledgeSparseSearcher, lg loggateway.Logger) *knowledge.HybridRetriever {
	if retriever == nil {
		return nil
	}
	if sparse == nil {
		lg.Info("稀疏检索未配置，仅使用密集检索",
			loggateway.StepID("knowledge.hybrid.init"),
		)
	}
	return knowledge.NewHybridRetriever(retriever, sparse, lg)
}

func NewKnowledgeQueryRewriter(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *knowledge.QueryRewriter {
	if llm == nil {
		lg.Info("LLM 未配置，查询重写已禁用",
			loggateway.StepID("knowledge.query_rewriter.init"),
		)
		return nil
	}
	return knowledge.NewQueryRewriter(llm, sys, catalog, lg)
}

func NewKnowledgeAdaptiveRouter(hybrid *knowledge.HybridRetriever, rewriter *knowledge.QueryRewriter, lg loggateway.Logger) *knowledge.AdaptiveRouter {
	if hybrid == nil {
		return nil
	}
	return knowledge.NewAdaptiveRouter(hybrid, rewriter, lg)
}

func NewKnowledgeRetrievalEvaluator(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *knowledge.RetrievalEvaluator {
	if llm == nil {
		return nil
	}
	return knowledge.NewRetrievalEvaluator(llm, sys, catalog, lg)
}

func NewKnowledgeFederatedRetriever(router *knowledge.AdaptiveRouter, retriever *knowledge.Retriever, uc *biz.KnowledgeUsecase, lg loggateway.Logger) *knowledge.FederatedRetriever {
	if router == nil && retriever == nil {
		return nil
	}
	if uc != nil {
		return knowledge.NewFederatedRetrieverWithMeta(router, retriever, uc, lg)
	}
	return knowledge.NewFederatedRetriever(router, retriever, lg)
}

func ProvideKnowledgeSearchDeps(retriever *knowledge.Retriever, router *knowledge.AdaptiveRouter, evaluator *knowledge.RetrievalEvaluator) KnowledgeSearchDeps {
	return KnowledgeSearchDeps{
		Retriever: retriever,
		Router:    router,
		Evaluator: evaluator,
	}
}
