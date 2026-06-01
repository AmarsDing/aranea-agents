package codeexecutor

import (
	"context"

	"aranea-agents/pkg/loggateway"

	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// dockerRuntimeFallback retries failed docker runs on the local executor.
type dockerRuntimeFallback struct {
	docker  trpcagentcodeexec.CodeExecutor
	factory *Factory
	workDir string
	lg      loggateway.Logger
}

func newDockerRuntimeFallback(docker trpcagentcodeexec.CodeExecutor, factory *Factory, workDir string, lg loggateway.Logger) trpcagentcodeexec.CodeExecutor {
	if docker == nil || factory == nil {
		return docker
	}
	if lg == nil {
		lg = loggateway.Global()
	}
	return &dockerRuntimeFallback{docker: docker, factory: factory, workDir: workDir, lg: lg}
}

func (d *dockerRuntimeFallback) CodeBlockDelimiter() trpcagentcodeexec.CodeBlockDelimiter {
	return d.docker.CodeBlockDelimiter()
}

func (d *dockerRuntimeFallback) ExecuteCode(ctx context.Context, input trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	result, err := d.docker.ExecuteCode(ctx, input)
	if err == nil {
		return result, nil
	}
	d.lg.Warn("Docker 执行失败，回退到 local 执行器",
		loggateway.StepID("agent.codeexec.docker_runtime_fallback"),
		loggateway.Err(err))
	ResetDockerProbe()
	return d.factory.getLocal(d.workDir).ExecuteCode(ctx, input)
}
