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
		lg = loggateway.NewNoop()
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
	// 生产环境且未显式允许 local 时 fail-closed：返回错误而非回退到无隔离的
	// local 执行器。与 Factory.applyAvailabilityFallback 的配置期守卫一致，
	// 避免运行期 docker 故障导致不可信代码在宿主机上执行。
	if isProductionEnv() && !d.factory.env.AllowLocalInProd {
		d.lg.Error("生产环境 Docker 运行期失败，拒绝回退 local（fail-closed）",
			loggateway.StepID("agent.codeexec.docker_runtime_fail_closed"),
			loggateway.Err(err))
		ResetDockerProbe()
		return trpcagentcodeexec.CodeExecutionResult{}, err
	}
	d.lg.Warn("Docker 执行失败，回退到 local 执行器",
		loggateway.StepID("agent.codeexec.docker_runtime_fallback"),
		loggateway.Err(err))
	ResetDockerProbe()
	return d.factory.getLocal(d.workDir).ExecuteCode(ctx, input)
}
