package agent

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/knowledge"
)

// KnowledgeAdapter converts the project's knowledge configuration into
// framework llmagent.Option functions that enable framework WithKnowledge.
//
// TECH-DEBT: Currently returns nil because the project uses its own
// knowledge_search tool pipeline (see knowledge_inject.go). Will be activated
// when migrating to framework-native knowledge support.
func KnowledgeAdapter(_ context.Context, ag biz.Agent, deps TRPCBuilderDeps, _ loggateway.Logger) []llmagent.Option {
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		return nil
	}
	if deps.KnowledgeUsecase == nil {
		return nil
	}
	// TODO(debt): Activate when project migrates from custom knowledge tools
	// to framework-native llmagent.WithKnowledge.
	return nil
}

// SafetyLimitAdapter converts the project's agent safety settings into
// framework llmagent.Option functions that enable framework safety limits.
//
// TECH-DEBT: Currently returns nil because AgentRuntimeSettings lacks
// MaxLLMCalls / MaxToolIterations fields. Will be extended when those
// fields are added.
func SafetyLimitAdapter(ag biz.Agent) []llmagent.Option {
	if ag.Settings == nil {
		return nil
	}
	// TODO(debt): Add safety options when AgentRuntimeSettings gains
	// MaxLLMCalls / MaxToolIterations fields.
	return nil
}

// FrameworkKnowledge wraps a knowledge.Knowledge implementation to satisfy
// the framework's WithKnowledge option.
//
// TECH-DEBT: Placeholder for future integration when the project migrates
// from custom knowledge tools to the framework's native knowledge support.
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
