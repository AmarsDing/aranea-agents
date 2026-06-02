package team

import (
	"context"

	"aranea-agents/internal/biz"
	graphadapter "aranea-agents/internal/graph/adapter"
	"aranea-agents/internal/knowledge"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	tooltrpc "aranea-agents/internal/tools/trpc"
)

type KnowledgeFacade struct {
	Retriever          *knowledge.Retriever
	Router             *knowledge.AdaptiveRouter
	FederatedRetriever *knowledge.FederatedRetriever
	Evaluator          *knowledge.RetrievalEvaluator
}

type RunnerConfig struct {
	GraphLoader       GraphBuildConfigLoader
	TeamGraphTasks    TeamGraphTaskCreator
	AwaitHookProvider func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc
	Knowledge         *KnowledgeFacade
	StreamOptsFactory StreamOptsFactory
	AgentHelper       biz.TeamAgentHelper
	Runs              *rt.RunRegistry
	GraphRoot         graphadapter.TeamGraphRootBuilder
	PluginRT          *plugintrpc.Runtime
	PluginManager     *plugintrpc.Manager
}
