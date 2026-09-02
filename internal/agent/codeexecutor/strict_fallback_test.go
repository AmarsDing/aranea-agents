package codeexecutor

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// newStrictFactory 构造 strict 降级策略的 Factory（83-长时运行韧性 FR-3）。
// env 在构造期加载，故 Setenv 必须先于 NewFactoryWithLogger。
func newStrictFactory(t *testing.T) *Factory {
	t.Helper()
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "strict")
	t.Setenv("ARANEA_ENV", "") // 排除生产 fail-closed 分支干扰断言
	return NewFactoryWithLogger(loggateway.NewNoop())
}

func TestStrictFallbackRefusesDockerDegrade(t *testing.T) {
	stubDockerProbeHooks(t, false)
	f := newStrictFactory(t)
	if exec := f.Resolve(context.Background(), TypeDocker, t.TempDir()); exec != nil {
		t.Fatal("strict + docker unavailable must refuse (nil executor)")
	}
}

func TestStrictFallbackRefusesSandboxDegrade(t *testing.T) {
	// 无 sandbox manager → sandboxAvailable() false → 降级 sandbox→docker 被拒。
	f := newStrictFactory(t)
	if exec := f.Resolve(context.Background(), TypeSandbox, t.TempDir()); exec != nil {
		t.Fatal("strict + sandbox unavailable must refuse (nil executor)")
	}
}

func TestStrictFallbackRefusesE2BDegrade(t *testing.T) {
	t.Setenv("E2B_API_KEY", "")
	f := newStrictFactory(t)
	if exec := f.Resolve(context.Background(), TypeE2B, t.TempDir()); exec != nil {
		t.Fatal("strict + e2b unavailable must refuse (nil executor)")
	}
}

func TestStrictFallbackKeepsAvailableBackend(t *testing.T) {
	stubDockerProbeHooks(t, true)
	f := newStrictFactory(t)
	if exec := f.Resolve(context.Background(), TypeDocker, t.TempDir()); exec == nil {
		t.Fatal("strict must not refuse an available backend")
	}
}

// 回归：degrade（默认）策略下 docker→local 降级行为不变。
func TestDegradePolicyKeepsDockerToLocalFallback(t *testing.T) {
	stubDockerProbeHooks(t, false)
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "degrade")
	t.Setenv("ARANEA_ENV", "")
	f := NewFactoryWithLogger(loggateway.NewNoop())
	if exec := f.Resolve(context.Background(), TypeDocker, t.TempDir()); exec == nil {
		t.Fatal("degrade policy must keep docker→local fallback")
	}
}
