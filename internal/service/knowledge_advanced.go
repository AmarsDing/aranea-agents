package service

import (
	"os"
	"strings"

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

func NewKnowledgeGraphExpander(repo biz.KnowledgeRepo, lg loggateway.Logger) *knowledge.GraphExpander {
	if repo == nil {
		return nil
	}
	links, _ := repo.(knowledge.NeighborLinkReader)
	chunks, _ := repo.(knowledge.NeighborChunkLister)
	if links == nil || chunks == nil {
		lg.Info("图扩展未接线：repo 未同时实现 NeighborLinkReader/NeighborChunkLister",
			loggateway.StepID("knowledge.graph_expander.init_skip"))
		return nil
	}
	return knowledge.NewGraphExpander(links, chunks, lg)
}

func NewKnowledgeAdaptiveRouter(hybrid *knowledge.HybridRetriever, rewriter *knowledge.QueryRewriter, expander *knowledge.GraphExpander, lg loggateway.Logger) *knowledge.AdaptiveRouter {
	if hybrid == nil {
		return nil
	}
	router := knowledge.NewAdaptiveRouter(hybrid, rewriter, lg)
	router.SetGraphExpander(expander)
	return router
}

func NewKnowledgeRetrievalEvaluator(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *knowledge.RetrievalEvaluator {
	if llm == nil {
		return nil
	}
	return knowledge.NewRetrievalEvaluator(llm, sys, catalog, lg)
}

// NewKnowledgeMarkdownOrganizer 构造 Markdown 整理器；LLM 不可用返回 nil（service 跳过整理）。
func NewKnowledgeMarkdownOrganizer(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *knowledge.MarkdownOrganizer {
	if llm == nil {
		lg.Info("LLM 未配置，Markdown 整理已禁用",
			loggateway.StepID("knowledge.markdown_organizer.init"),
		)
		return nil
	}
	return knowledge.NewMarkdownOrganizer(llm, sys, catalog, lg)
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

func ProvideKnowledgeSearchDeps(retriever *knowledge.Retriever, router *knowledge.AdaptiveRouter, evaluator *knowledge.RetrievalEvaluator, federated *knowledge.FederatedRetriever) KnowledgeSearchDeps {
	return KnowledgeSearchDeps{
		Retriever: retriever,
		Router:    router,
		Evaluator: evaluator,
		Federated: federated,
	}
}

// NewKnowledgeExtractorRegistry 装配模态路由注册表（Phase 9）：
// VisionExtractor 优先于 TextExtractor；llm 为 nil 时 Vision 提取返回明确错误（NFR-12），
// 文本路径不受影响。
func NewKnowledgeExtractorRegistry(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *knowledge.ExtractorRegistry {
	return knowledge.NewExtractorRegistry(
		knowledge.NewVisionExtractor(llm, sys, catalog, lg),
		knowledge.NewTextExtractor(),
	)
}

// NewKnowledgeAssetStore 装配原图留存存储（Phase 9 血缘）。
// 根目录优先级：KRATOS_KNOWLEDGE_ASSET_DIR env > ./data/knowledge_assets。
func NewKnowledgeAssetStore(lg loggateway.Logger) *knowledge.AssetStore {
	root := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_ASSET_DIR"))
	if root == "" {
		root = "./data/knowledge_assets"
	}
	lg.Info("知识库原图留存目录",
		loggateway.StepID("knowledge.asset_store.init"),
		loggateway.Str("root", root),
	)
	return knowledge.NewAssetStore(root)
}
