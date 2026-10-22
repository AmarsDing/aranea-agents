package codeexecutor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// errExecutor is a fake CodeExecutor whose ExecuteCode always fails with
// sentinelDockerErr, simulating a runtime docker backend failure.
type errExecutor struct{}

func (errExecutor) CodeBlockDelimiter() trpcagentcodeexec.CodeBlockDelimiter {
	return trpcagentcodeexec.CodeBlockDelimiter{}
}

func (errExecutor) ExecuteCode(_ context.Context, _ trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	return trpcagentcodeexec.CodeExecutionResult{}, sentinelDockerErr
}

var sentinelDockerErr = errors.New("docker boom")

// TestDockerRuntimeFallbackFailClosedInProd asserts that in production env
// without CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD, a runtime docker failure must
// NOT silently fall back to the local executor (fail-open). It must return
// the docker error instead (fail-closed), matching the config-time guard in
// Factory.applyAvailabilityFallback.
func TestDockerRuntimeFallbackFailClosedInProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	// CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD intentionally unset → AllowLocalInProd=false

	f := NewFactoryWithLogger(loggateway.NewNoop())
	fb := newDockerRuntimeFallback(errExecutor{}, f, t.TempDir(), loggateway.NewNoop())

	_, err := fb.ExecuteCode(context.Background(), trpcagentcodeexec.CodeExecutionInput{})
	if err == nil {
		t.Fatal("expected docker error to be returned in production (fail-closed), got nil (silent local fallback)")
	}
	if !errors.Is(err, sentinelDockerErr) {
		t.Fatalf("expected sentinel docker error to propagate, got %v", err)
	}
}

// TestDockerRuntimeFallbackStrictRefusesLocal asserts that under
// CODE_EXECUTOR_FALLBACK_POLICY=strict (83-长时运行韧性 FR-3), a runtime
// docker failure is refused even outside production — no silent degrade to
// the less-isolated local executor.
func TestDockerRuntimeFallbackStrictRefusesLocal(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "strict")

	f := NewFactoryWithLogger(loggateway.NewNoop())
	fb := newDockerRuntimeFallback(errExecutor{}, f, t.TempDir(), loggateway.NewNoop())

	_, err := fb.ExecuteCode(context.Background(), trpcagentcodeexec.CodeExecutionInput{})
	if !errors.Is(err, sentinelDockerErr) {
		t.Fatalf("strict: expected sentinel docker error, got %v", err)
	}
}

// TestDockerRuntimeFallbackStillLocalInNonProd asserts the existing dev
// behavior is preserved: outside production, a runtime docker failure still
// falls back to the local executor (no error surfaced to the caller).
func TestDockerRuntimeFallbackStillLocalInNonProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")

	f := NewFactoryWithLogger(loggateway.NewNoop())
	fb := newDockerRuntimeFallback(errExecutor{}, f, t.TempDir(), loggateway.NewNoop())

	res, err := fb.ExecuteCode(context.Background(), trpcagentcodeexec.CodeExecutionInput{})
	if err != nil {
		t.Fatalf("expected local fallback to succeed with empty input in non-prod, got err: %v", err)
	}
	// Empty input → local executor returns an empty result (no code blocks to run).
	_ = res
}

// --- P0-1 ① enforcement 诚实分级（review 2026-10-21 §2.4） ---

// stubExec is a fake CodeExecutor returning a fixed output.
type stubExec struct{ out string }

func (s stubExec) CodeBlockDelimiter() trpcagentcodeexec.CodeBlockDelimiter {
	return trpcagentcodeexec.CodeBlockDelimiter{}
}
func (s stubExec) ExecuteCode(_ context.Context, _ trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	return trpcagentcodeexec.CodeExecutionResult{Output: s.out}, nil
}

// partialMarker 逐次在模型可见输出尾部打 enforcement=partial 尾注；已有
// enforcement 标记的输出不重复追加（§12.1-①：尾注是框架 Result 无元数据位
// 下的诚实信号通道）。
func TestPartialMarkerAppendsTailNote(t *testing.T) {
	pm := partialMarker{inner: stubExec{out: "hello"}}
	res, err := pm.ExecuteCode(context.Background(), trpcagentcodeexec.CodeExecutionInput{})
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if want := "hello" + EnforcementPartialNote; res.Output != want {
		t.Fatalf("output=%q, want %q", res.Output, want)
	}
	// 幂等：已打标的输出不重复追加。
	pm2 := partialMarker{inner: stubExec{out: "x" + EnforcementPartialNote}}
	res2, err := pm2.ExecuteCode(context.Background(), trpcagentcodeexec.CodeExecutionInput{})
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if strings.Count(res2.Output, EnforcementPartialNote) != 1 {
		t.Fatalf("tail note duplicated: %q", res2.Output)
	}
}

// docker 运行期降级必须：① 落 local 且输出带 [enforcement:partial] 尾注；
// ② 决策点发 sandbox.enforcement_partial（AS-EVT-01 Important）流程日志。
func TestDockerRuntimeFallbackMarksPartialAndEmits(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "degrade")

	f := NewFactoryWithLogger(loggateway.NewNoop())
	bus := &captureFlowBus{}
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: bus}, event.TraceContext{
		TraceID: "tr-p01-partial",
		Domain:  event.TraceDomainSkill,
	}, nil)
	ctx := event.WithTraceEmitter(context.Background(), em)

	fb := newDockerRuntimeFallback(errExecutor{}, f, t.TempDir(), loggateway.NewNoop())
	res, err := fb.ExecuteCode(ctx, trpcagentcodeexec.CodeExecutionInput{})
	if err != nil {
		t.Fatalf("degraded local run: %v", err)
	}
	if !strings.Contains(res.Output, EnforcementPartialNote) {
		t.Fatalf("output missing %q note: %q", EnforcementPartialNote, res.Output)
	}
	if !bus.hasFlowStep("sandbox.enforcement_partial") {
		t.Fatal("expected flow-log step sandbox.enforcement_partial")
	}
	if got := testutil.ToFloat64(codeExecEnforcementPartialTotal.WithLabelValues(TypeDocker, TypeLocal)); got < 1 {
		t.Fatalf("partial counter=%v, want ≥1", got)
	}
}

// --- P0-1 ③ SurfacePolicy per-surface 显式链解析 ---

// degrade（默认/空值）= 与旧硬编码 Resolve(TypeLocal) 同语义：返回非 nil local。
func TestResolveForSurfaceDegradeMatchesLegacyLocal(t *testing.T) {
	t.Setenv("ARANEA_ENV", "")
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "")
	f := NewFactoryWithLogger(loggateway.NewNoop())
	if got := f.ResolveForSurface(context.Background(), SurfaceSkillGate, t.TempDir()); got == nil {
		t.Fatal("degrade surface must resolve a local executor (legacy parity)")
	}
}

// refuse：恒 nil（调用点输出结构化 skipped）。
func TestResolveForSurfaceRefuseReturnsNil(t *testing.T) {
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "refuse")
	f := NewFactoryWithLogger(loggateway.NewNoop())
	if got := f.ResolveForSurface(context.Background(), SurfaceSkillGate, t.TempDir()); got != nil {
		t.Fatalf("refuse surface must return nil, got %T", got)
	}
}

// enforce：sandbox/docker 均不可用时 fail-closed nil（不静默落 local）。
func TestResolveForSurfaceEnforceUnavailableReturnsNil(t *testing.T) {
	stubDockerProbeHooks(t, false)
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "enforce")
	f := NewFactoryWithLogger(loggateway.NewNoop())
	if got := f.ResolveForSurface(context.Background(), SurfaceSkillGate, t.TempDir()); got != nil {
		t.Fatalf("enforce with no isolated backend must return nil, got %T", got)
	}
}

// enforce + docker 可用：直出 docker（不挂 local 运行期回退——面级严格）。
func TestResolveForSurfaceEnforceDockerDirect(t *testing.T) {
	stubDockerProbeHooks(t, true)
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "enforce")
	f := NewFactoryWithLogger(loggateway.NewNoop())
	got := f.ResolveForSurface(context.Background(), SurfaceSkillGate, t.TempDir())
	if got == nil {
		t.Fatal("enforce with docker up must resolve an executor")
	}
	if _, isRuntimeFallback := got.(*dockerRuntimeFallback); isRuntimeFallback {
		t.Fatal("enforce path must not wrap docker with a local runtime fallback")
	}
}

// SurfacePolicyFor/StrictFallback 只读访问器。
func TestFactoryPolicyAccessors(t *testing.T) {
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "enforce")
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "strict")
	f := NewFactoryWithLogger(loggateway.NewNoop())
	if got := f.SurfacePolicyFor(SurfaceSkillGate); got != PolicyEnforce {
		t.Fatalf("SurfacePolicyFor=%q, want enforce", got)
	}
	if got := f.SurfacePolicyFor(SurfaceMonitor); got != PolicyDegrade {
		t.Fatalf("unset surface=%q, want degrade", got)
	}
	if !f.StrictFallback() {
		t.Fatal("StrictFallback must mirror env strict")
	}
}
