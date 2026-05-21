package codeexecutor

import (
	"context"

	"aranea-agents/internal/event"

	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// dockerRuntimeFallback retries failed docker runs on the local executor.
type dockerRuntimeFallback struct {
	docker  trpcagentcodeexec.CodeExecutor
	factory *Factory
	workDir string
}

func newDockerRuntimeFallback(docker trpcagentcodeexec.CodeExecutor, factory *Factory, workDir string) trpcagentcodeexec.CodeExecutor {
	if docker == nil || factory == nil {
		return docker
	}
	return &dockerRuntimeFallback{docker: docker, factory: factory, workDir: workDir}
}

func (d *dockerRuntimeFallback) CodeBlockDelimiter() trpcagentcodeexec.CodeBlockDelimiter {
	return d.docker.CodeBlockDelimiter()
}

func (d *dockerRuntimeFallback) ExecuteCode(ctx context.Context, input trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	result, err := d.docker.ExecuteCode(ctx, input)
	if err == nil {
		return result, nil
	}
	event.CtxFlowLogWarn(ctx, "system.codeexec.docker_runtime_fallback",
		"Docker 执行失败，回退到 local 执行器",
		event.P("error", err.Error()))
	ResetDockerProbe()
	return d.factory.getLocal(d.workDir).ExecuteCode(ctx, input)
}
