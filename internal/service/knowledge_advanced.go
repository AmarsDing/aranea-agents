package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
)

func NewKnowledgeHybridRetriever(retriever *knowledge.Retriever, sparse biz.KnowledgeSparseSearcher) *knowledge.HybridRetriever {
	if retriever == nil {
		return nil
	}
	if sparse == nil {
		event.SysLogInfo("knowledge.hybrid.init", "稀疏检索未配置，仅使用密集检索")
	}
	return knowledge.NewHybridRetriever(retriever, sparse)
}

func NewKnowledgeQueryRewriter(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase) *knowledge.QueryRewriter {
	if llm == nil {
		event.SysLogInfo("knowledge.query_rewriter.init", "LLM 未配置，查询重写已禁用")
		return nil
	}
	return knowledge.NewQueryRewriter(llm, sys, catalog)
}

func NewKnowledgeAdaptiveRouter(hybrid *knowledge.HybridRetriever, rewriter *knowledge.QueryRewriter) *knowledge.AdaptiveRouter {
	if hybrid == nil {
		return nil
	}
	return knowledge.NewAdaptiveRouter(hybrid, rewriter)
}

func NewKnowledgeRetrievalEvaluator(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase) *knowledge.RetrievalEvaluator {
	if llm == nil {
		return nil
	}
	return knowledge.NewRetrievalEvaluator(llm, sys, catalog)
}
