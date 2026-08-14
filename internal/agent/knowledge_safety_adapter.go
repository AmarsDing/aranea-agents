package agent

import (
	"context"

	"aranea-agents/internal/biz"
	knowledgeadapter "aranea-agents/internal/knowledge"
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
	// Bridge the project's Retriever.Search to framework knowledge.Knowledge.
	adapter := knowledgeadapter.NewKnowledgeAdapter(deps.KnowledgeRetriever.Search, lg)
	fk := NewFrameworkKnowledge(adapter)
	return fk.Options()
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
