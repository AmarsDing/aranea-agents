package trpc

import (
	"context"

	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// NewLocalExecutor returns the framework local CodeExecutor (via Factory).
func NewLocalExecutor(factory *localexec.Factory, workDir string) codeexecutor.CodeExecutor {
	if factory == nil {
		factory = localexec.NewFactory()
	}
	return factory.Resolve(context.Background(), localexec.TypeLocal, workDir)
}

// NewExecutorForAgent resolves the CodeExecutor for an agent and wraps artifact persistence.
// agentType is AgentRuntimeSettings.CodeExecutorType; empty uses env CODE_EXECUTOR_BACKEND then local.
func NewExecutorForAgent(ctx context.Context, factory *localexec.Factory, agentType, workDir string, lg loggateway.Logger) codeexecutor.CodeExecutor {
	if factory == nil {
		factory = localexec.NewFactory()
	}
	exec := factory.Resolve(ctx, agentType, workDir)
	return WrapWithArtifactSave(exec, lg)
}
