package agent

import (
	"context"

	"aranea-agents/internal/biz"
	knowledgeadapter "aranea-agents/internal/knowledge"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/knowledge"
)

// KnowledgeAdapter converts the project's knowledge configuration into
// framework llmagent.Option functions that enable framework WithKnowledge.
// It bridges the project's self-built retriever to the framework's
// knowledge.Knowledge interface, enabling framework-native knowledge_search
// alongside the project's custom knowledge tools.
func KnowledgeAdapter(_ context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) []llmagent.Option {
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		return nil
	}
	if deps.KnowledgeUsecase == nil || deps.KnowledgeUsecase.IsUnavailable() {
		return nil
	}
	if deps.KnowledgeRetriever == nil {
		return nil
	}
	// Bridge the project's retriever to framework knowledge.Knowledge.
	// P2-a 收编：框架原生 knowledge_search 与自定义 knowledge 工具同规——
	// 裸 Retriever.Search 无 workspace 概念（collection_id 空 → 查集合 '' 恒空；
	// 指定 id → 跨租户直查），改为 workspace 路由检索。
	adapter := knowledgeadapter.NewKnowledgeAdapter(workspaceRoutedKnowledgeSearch(deps), lg)
	fk := NewFrameworkKnowledge(adapter)
	return fk.Options()
}

// workspaceRoutedKnowledgeSearch 框架原生 knowledge_search 的收编检索函数（P2-a）：
//   - 未指定 collection_id：走联邦检索 SearchAll，workspace.ReadableFilterID 租户过滤
//     （system 见全部，租户见自有+共享）；联邦检索器不在 ctx（非会话路径）降级空结果，
//     不阻塞会话。
//   - 指定 collection_id：先经 GetCollection 做租户可见性校验（C-25：跨租户 →
//     NotFound），可见后优先 AdaptiveRouter（access_log/Hebbian 记账），退化 Retriever。
func workspaceRoutedKnowledgeSearch(deps TRPCBuilderDeps) knowledgeadapter.SearchFunc {
	return func(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
		if q.CollectionID == "" {
			fr := knowledgetool.FederatedRetrieverFromContext(ctx)
			if fr == nil {
				return nil, nil
			}
			return fr.SearchAll(ctx, q, nil, "", workspace.ReadableFilterID(ctx))
		}
		if deps.KnowledgeUsecase != nil {
			// C-25：GetCollection 按 ctx workspace 过滤，跨租户 → NotFound 上抛。
			if _, err := deps.KnowledgeUsecase.GetCollection(ctx, q.CollectionID); err != nil {
				return nil, err
			}
		}
		if router := knowledgetool.AdaptiveRouterFromContext(ctx); router != nil {
			return router.Search(ctx, q, nil, "")
		}
		return deps.KnowledgeRetriever.Search(ctx, q)
	}
}

// SafetyLimitAdapter converts the project's agent safety settings into
// framework llmagent.Option functions that enable framework safety limits.
// When MaxLLMCalls or MaxToolIterations are configured (> 0), the framework
// enforces per-turn limits to prevent runaway agent behavior.
// Legacy rows that violate the coupling invariant (see biz.ValidateSafetyLimitCoupling)
// are defensively elevated so the turn can still end with a graceful summary.
func SafetyLimitAdapter(ag biz.Agent, lg loggateway.Logger) []llmagent.Option {
	if ag.Settings == nil {
		return nil
	}
	maxLLMCalls, maxToolIterations, elevated := biz.CoupledSafetyLimits(ag.Settings)
	if elevated {
		lg.Warn("max_llm_calls 低于优雅收尾所需余量，已防御性抬升",
			loggateway.StepID("agent.safety_limit_elevated"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Int("max_llm_calls", maxLLMCalls),
			loggateway.Int("max_tool_iterations", maxToolIterations))
	}
	var opts []llmagent.Option
	if maxLLMCalls > 0 {
		opts = append(opts, llmagent.WithMaxLLMCalls(maxLLMCalls))
	}
	if maxToolIterations > 0 {
		opts = append(opts, llmagent.WithMaxToolIterations(maxToolIterations))
	}
	return opts
}

// FrameworkKnowledge wraps a knowledge.Knowledge implementation to satisfy
// the framework's WithKnowledge option. It is actively used by
// KnowledgeAdapter to bridge the project's retriever to the framework's
// native knowledge_search tool.
type FrameworkKnowledge struct {
	inner knowledge.Knowledge
}

// NewFrameworkKnowledge creates a FrameworkKnowledge wrapper.
func NewFrameworkKnowledge(kb knowledge.Knowledge) *FrameworkKnowledge {
	return &FrameworkKnowledge{inner: kb}
}

// Options returns the llmagent.Option slice that enables framework knowledge.
func (fk *FrameworkKnowledge) Options() []llmagent.Option {
	if fk.inner == nil {
		return nil
	}
	return []llmagent.Option{llmagent.WithKnowledge(fk.inner)}
}
