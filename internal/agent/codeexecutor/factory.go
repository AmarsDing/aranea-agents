package codeexecutor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"aranea-agents/internal/event"
	"aranea-agents/internal/sandbox"
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
	sandboxMgr    *sandbox.Manager // M82: bound post-construction by the wire provider
	sandboxStore  *sandbox.SessionLeases
	lg            loggateway.Logger
}

// SetSandboxManager binds the M82 sandbox manager (warm-pool backend) plus
// the process-wide shared SessionLeases store (P1-1/P1-2: the SAME store is
// injected into the sandbox_fs toolset so code_exec and sandbox_fs share one
// sandbox per session). The manager is constructed after the factory in the
// wire graph, so binding is a post-construction setter rather than a
// constructor param.
func (f *Factory) SetSandboxManager(mgr *sandbox.Manager, store *sandbox.SessionLeases) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sandboxMgr = mgr
	if mgr != nil {
		f.sandboxStore = store
	} else {
		f.sandboxStore = nil
	}
}

// sandboxAvailable reports whether the pooled sandbox backend can serve runs.
func (f *Factory) sandboxAvailable() bool {
	f.mu.RLock()
	mgr := f.sandboxMgr
	f.mu.RUnlock()
	return mgr != nil && mgr.Available()
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
	if typ == TypeAuto {
		if DockerAvailable() {
			typ = TypeDocker
		} else {
			typ = TypeLocal
		}
	} else if PreferDockerWhenUnset(agentType, f.env.Backend, DockerAvailable()) {
		typ = TypeDocker
	}
	typ = f.applyAvailabilityFallback(ctx, typ)
	typ = f.refuseLocalInProd(typ)

	switch typ {
	case TypeDisabled:
		return nil
	case TypeSandbox:
		f.mu.RLock()
		mgr := f.sandboxMgr
		store := f.sandboxStore
		f.mu.RUnlock()
		if mgr != nil && mgr.Available() {
			return wrapMetrics(newPooledAdapter(mgr, f.env.Timeout, store), TypeSandbox)
		}
		// 防御性二次检查：applyAvailabilityFallback 通常已把 sandbox→docker；
		// 运行期 Manager 不可用时走与 docker 相同的运行时回退链。
		if f.env.StrictFallback() {
			f.strictRefused(TypeSandbox, "sandbox_unavailable_runtime")
			return nil
		}
		f.emitFallbackFlow(ctx, TypeSandbox, TypeDocker)
		lg := f.lg
		if lg == nil {
			lg = loggateway.NewNoop()
		}
		return wrapMetrics(newDockerRuntimeFallback(f.getDocker(), f, workDir, lg), TypeDocker)
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
		if f.env.StrictFallback() {
			f.strictRefused(TypeE2B, "e2b_init_failed")
			return nil
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
		if f.env.StrictFallback() {
			f.strictRefused(TypeContainer, "container_init_failed")
			return nil
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
	if typ == TypeSandbox && !f.sandboxAvailable() {
		// 83 FR-3 strict：不可用即拒，不沿降级链回退。
		if f.env.StrictFallback() {
			return f.strictRefused(TypeSandbox, "sandbox_unavailable")
		}
		// M82 降级链：sandbox 池不可用（禁用/无 daemon）→ docker；docker 再由
		// 下方检查继续降级 local（NFR-04）。
		f.logger().Warn("sandbox 池不可用，回退到 docker 执行器",
			loggateway.StepID("codeexec.sandbox_fallback"),
			loggateway.Str("requested", TypeSandbox))
		f.emitFallbackFlow(ctx, TypeSandbox, TypeDocker)
		typ = TypeDocker
	}
	if typ == TypeDocker && !DockerAvailable() {
		if f.env.StrictFallback() {
			return f.strictRefused(TypeDocker, "docker_unavailable")
		}
		if isProductionEnv() && !f.env.AllowLocalInProd {
			f.logger().Error("生产环境 Docker 不可用且未允许 local 执行器，拒绝代码执行",
				loggateway.StepID("codeexec.docker_unavailable_prod"),
				loggateway.Str("requested", TypeDocker))
			return TypeDisabled
		}
		f.logger().Warn("Docker 不可用，回退到 local 执行器",
			loggateway.StepID("codeexec.docker_fallback"),
			loggateway.Str("requested", TypeDocker))
		f.emitFallbackFlow(ctx, TypeDocker, TypeLocal)
		return TypeLocal
	}
	if typ == TypeE2B && !f.IsBackendAvailable(TypeE2B) {
		if f.env.StrictFallback() {
			return f.strictRefused(TypeE2B, "e2b_unavailable")
		}
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
		if f.env.StrictFallback() {
			return f.strictRefused(TypeContainer, "container_unavailable")
		}
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

// strictRefused 严格降级策略（83-长时运行韧性 FR-3）的统一判拒出口：
// Error 进程日志（复用 refuseLocalInProd 的日志风格）+ TypeDisabled。
func (f *Factory) strictRefused(requested, reason string) string {
	f.logger().Error("严格降级策略（CODE_EXECUTOR_FALLBACK_POLICY=strict）：请求的执行器不可用，拒绝降级并拒绝代码执行",
		loggateway.StepID("codeexec.strict_fallback_refused"),
		loggateway.Str("requested", requested),
		loggateway.Str("reason", reason))
	return TypeDisabled
}

func (f *Factory) warnResolveFallback(ctx context.Context, requested string) {
	f.logger().Warn("请求的执行器不可用，回退到 local",
		loggateway.StepID("codeexec.resolve_fallback"),
		loggateway.Str("requested", requested))
	f.emitFallbackFlow(ctx, requested, TypeLocal)
}

// emitFallbackFlow 降级点流程日志（83-长时运行韧性 FR-3 K3）：ctx 携带
// TraceEmitter 时发射 codeexec.backend_fallback（step 已登记 flow_log.go）；
// 未携带（启动期/非会话调用）时仅进程日志（现状语义），nil-safe。
func (f *Factory) emitFallbackFlow(ctx context.Context, requested, fallback string) {
	em := event.TraceEmitterFromContext(ctx)
	if em == nil {
		return
	}
	em.LogWarn("codeexec.backend_fallback", "",
		fmt.Sprintf("代码执行后端降级：%s → %s", requested, fallback),
		event.P("requested", requested),
		event.P("fallback", fallback))
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
