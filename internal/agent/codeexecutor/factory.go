package codeexecutor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"aranea-agents/pkg/loggateway"

	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	e2bexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/e2b"
	trpclocal "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
)

// Factory builds and resolves CodeExecutor instances for Skill runtime.
// Wire injects a process-level singleton; heavy backends (E2B, container) register lazily.
type Factory struct {
	mu            sync.RWMutex
	env           EnvConfig
	docker        trpcagentcodeexec.CodeExecutor
	e2b           trpcagentcodeexec.CodeExecutor
	e2bOnce       sync.Once
	container     trpcagentcodeexec.CodeExecutor
	containerOnce sync.Once
	localWD       string
	localExec     trpcagentcodeexec.CodeExecutor
	lg            loggateway.Logger
}

func NewFactoryWithLogger(lg loggateway.Logger) *Factory {
	return &Factory{env: LoadEnvConfig(), lg: lg}
}

func (f *Factory) newLocal(workDir string) trpcagentcodeexec.CodeExecutor {
	opts := []trpclocal.CodeExecutorOption{trpclocal.WithCleanTempFiles(true)}
	if workDir != "" {
		opts = append(opts, trpclocal.WithWorkDir(workDir))
	}
	return trpclocal.New(opts...)
}

func (f *Factory) newDocker() trpcagentcodeexec.CodeExecutor {
	cfg := DefaultDockerConfig()
	if f.env.DockerImage != "" {
		cfg.Image = f.env.DockerImage
	}
	return newDockerAdapter(cfg, f.env.Timeout)
}

func (f *Factory) tryE2B() (trpcagentcodeexec.CodeExecutor, error) {
	if f.env.E2BAPIKey == "" {
		return nil, fmt.Errorf("e2b: E2B_API_KEY not set")
	}
	return e2bexec.New(e2bexec.WithAPIKey(f.env.E2BAPIKey))
}

func (f *Factory) tryContainer() (trpcagentcodeexec.CodeExecutor, error) {
	return tryNewContainerExecutor()
}

func (f *Factory) getLocal(workDir string) trpcagentcodeexec.CodeExecutor {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.localExec != nil && f.localWD == workDir {
		return f.localExec
	}
	f.localWD = workDir
	f.localExec = wrapMetrics(f.newLocal(workDir), TypeLocal)
	return f.localExec
}

func (f *Factory) getDocker() trpcagentcodeexec.CodeExecutor {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.docker == nil {
		f.docker = f.newDocker()
	}
	return f.docker
}

func (f *Factory) getE2B() trpcagentcodeexec.CodeExecutor {
	f.e2bOnce.Do(func() {
		exec, err := f.tryE2B()
		if err != nil {
			return
		}
		f.e2b = wrapMetrics(exec, TypeE2B)
	})
	return f.e2b
}

func (f *Factory) getContainer() trpcagentcodeexec.CodeExecutor {
	f.containerOnce.Do(func() {
		exec, err := f.tryContainer()
		if err != nil {
			return
		}
		f.container = wrapMetrics(exec, TypeContainer)
	})
	return f.container
}

// Resolve returns the executor for agentType with env fallback and availability checks.
func (f *Factory) Resolve(ctx context.Context, agentType, workDir string) trpcagentcodeexec.CodeExecutor {
	typ := ResolveType(agentType, f.env.Backend)
	typ = f.applyAvailabilityFallback(ctx, typ)
	typ = f.refuseLocalInProd(typ)

	switch typ {
	case TypeDisabled:
		return nil
	case TypeDocker:
		lg := f.lg
		if lg == nil {
			lg = loggateway.NewNoop()
		}
		return wrapMetrics(newDockerRuntimeFallback(f.getDocker(), f, workDir, lg), TypeDocker)
	case TypeE2B:
		if exec := f.getE2B(); exec != nil {
			return exec
		}
		if isProductionEnv() && !f.env.AllowLocalInProd {
			f.logger().Error("生产环境 E2B 初始化失败且未允许 local 执行器，拒绝代码执行",
				loggateway.StepID("codeexec.e2b_init_fail_prod"),
				loggateway.Str("requested", TypeE2B))
			return nil
		}
		f.warnResolveFallback(ctx, typ)
		return f.getLocal(workDir)
	case TypeContainer:
		if exec := f.getContainer(); exec != nil {
			return exec
		}
		if isProductionEnv() && !f.env.AllowLocalInProd {
			f.logger().Error("生产环境 Container 初始化失败且未允许 local 执行器，拒绝代码执行",
				loggateway.StepID("codeexec.container_init_fail_prod"),
				loggateway.Str("requested", TypeContainer))
			return nil
		}
		f.warnResolveFallback(ctx, typ)
		return f.getLocal(workDir)
	default:
		return f.getLocal(workDir)
	}
}

func (f *Factory) logger() loggateway.Logger {
	if f.lg != nil {
		return f.lg
	}
	return loggateway.NewNoop()
}

func (f *Factory) applyAvailabilityFallback(ctx context.Context, typ string) string {
	if typ == TypeDocker && !DockerAvailable() {
		if isProductionEnv() && !f.env.AllowLocalInProd {
			f.logger().Error("生产环境 Docker 不可用且未允许 local 执行器，拒绝代码执行",
				loggateway.StepID("codeexec.docker_unavailable_prod"),
				loggateway.Str("requested", TypeDocker))
			return TypeDisabled
		}
		f.logger().Warn("Docker 不可用，回退到 local 执行器",
			loggateway.StepID("codeexec.docker_fallback"),
			loggateway.Str("requested", TypeDocker))
		return TypeLocal
	}
	if typ == TypeE2B && !f.IsBackendAvailable(TypeE2B) {
		if isProductionEnv() && !f.env.AllowLocalInProd {
			f.logger().Error("生产环境 E2B 不可用且未允许 local 执行器，拒绝代码执行",
				loggateway.StepID("codeexec.e2b_unavailable_prod"),
				loggateway.Str("requested", TypeE2B))
			return TypeDisabled
		}
		f.warnResolveFallback(ctx, typ)
		return TypeLocal
	}
	if typ == TypeContainer && !f.IsBackendAvailable(TypeContainer) {
		if isProductionEnv() && !f.env.AllowLocalInProd {
			f.logger().Error("生产环境 Container 不可用且未允许 local 执行器，拒绝代码执行",
				loggateway.StepID("codeexec.container_unavailable_prod"),
				loggateway.Str("requested", TypeContainer))
			return TypeDisabled
		}
		f.warnResolveFallback(ctx, typ)
		return TypeLocal
	}
	return typ
}

func (f *Factory) warnResolveFallback(ctx context.Context, requested string) {
	f.logger().Warn("请求的执行器不可用，回退到 local",
		loggateway.StepID("codeexec.resolve_fallback"),
		loggateway.Str("requested", requested))
}

// refuseLocalInProd converts TypeLocal → TypeDisabled in production unless
// CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD is explicitly set (B-03 fail-closed).
func (f *Factory) refuseLocalInProd(typ string) string {
	if typ != TypeLocal || f.env.AllowLocalInProd || !isProductionEnv() {
		return typ
	}
	f.logger().Error("生产环境拒绝 local 执行器（无隔离）；请配置 docker/e2b/container 或设置 CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD=1",
		loggateway.StepID("codeexec.local_in_prod_refused"),
		loggateway.Str("backend", TypeLocal))
	return TypeDisabled
}

func isProductionEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ARANEA_ENV"))) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

// RegisteredTypes returns backend types currently available (no eager E2B init).
func (f *Factory) RegisteredTypes() []string {
	caps := f.Capabilities()
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		if c.Available {
			out = append(out, c.Type)
		}
	}
	return out
}
