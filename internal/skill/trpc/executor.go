package trpc

import (
	"context"

	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// NewLocalExecutor returns the framework local CodeExecutor (via Factory).
func NewLocalExecutor(factory *localexec.Factory, workDir string) codeexecutor.CodeExecutor {
	if factory == nil {
		factory = localexec.NewFactoryWithLogger(loggateway.NewNoop())
	}
	return factory.Resolve(context.Background(), localexec.TypeLocal, workDir)
}

// NewExecutorForAgent resolves the CodeExecutor for an agent and wraps artifact persistence.
// agentType is AgentRuntimeSettings.CodeExecutorType; empty uses env CODE_EXECUTOR_BACKEND then local.
func NewExecutorForAgent(ctx context.Context, factory *localexec.Factory, agentType, workDir string, lg loggateway.Logger) codeexecutor.CodeExecutor {
	return NewExecutorForAgentWithFlowLog(ctx, factory, agentType, workDir, lg, nil)
}

// NewExecutorForAgentWithFlowLog additionally wires skill.execute flow-log
// emission through the given monitor bus (nil disables bus publication).
// The wrapper is surface-gate neutral: it only forwards Engine() when the
// resolved chain already implements codeexecutor.EngineProvider, so callers
// can adopt it without changing the llmagent tool surface. See
// WrapWithFlowLog for the activation requirements.
func NewExecutorForAgentWithFlowLog(ctx context.Context, factory *localexec.Factory, agentType, workDir string, lg loggateway.Logger, bus contract.MonitorBus) codeexecutor.CodeExecutor {
	if factory == nil {
		factory = localexec.NewFactoryWithLogger(lg)
	}
	exec := factory.Resolve(ctx, agentType, workDir)
	exec = WrapWithArtifactSave(exec, lg)
	return WrapWithFlowLog(exec, bus, lg)
}
