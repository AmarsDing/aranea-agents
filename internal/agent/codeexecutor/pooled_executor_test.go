package codeexecutor

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/sandbox"
)

// newPooledManager returns a sandbox Manager over the session-test fake engine.
func newPooledManager(t *testing.T, eng sandbox.Engine) *sandbox.Manager {
	t.Helper()
	cfg := sandbox.DefaultConfig()
	cfg.Pool.MinReady = 0
	m := sandbox.NewManager(cfg, eng, nil)
	t.Cleanup(m.Close)
	return m
}

// 池化后端的内部 Result 必须透传引擎的 enforcement 分级（恒 full——池后端
// 是隔离执行域；§2.4 诚实分级：按实际运行面打标，不在适配层虚构）。
func TestPooledRuntimePropagatesEnforcementFull(t *testing.T) {
	eng := &fakeSandboxEngine{}
	eng.execFunc = func(h sandbox.Handle, spec sandbox.ExecSpec) (sandbox.ExecResult, error) {
		return sandbox.ExecResult{
			Stdout:      "done",
			ExitCode:    0,
			Enforcement: sandbox.EnforcementFull,
		}, nil
	}
	mgr := newPooledManager(t, eng)
	lease, err := mgr.Acquire(context.Background(), sandbox.AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer func() { _ = lease.Release(context.Background()) }()

	rt := newPooledRuntime(mgr)
	out, err := rt.Run(context.Background(), lease, "python", "print('x')", 10*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Enforcement != EnforcementFull {
		t.Fatalf("Enforcement=%q, want %q", out.Enforcement, EnforcementFull)
	}
}

// stderr 截断尾注从引擎层原样保留在输出链（尾注是模型可见的截断信号，
// 经过适配层不得被吞掉）。
func TestPooledRuntimePreservesStderrTruncationNote(t *testing.T) {
	eng := &fakeSandboxEngine{}
	eng.execFunc = func(h sandbox.Handle, spec sandbox.ExecSpec) (sandbox.ExecResult, error) {
		return sandbox.ExecResult{
			Stderr:          "boom...\n[stderr truncated at 262144 bytes]",
			StderrTruncated: true,
			Enforcement:     sandbox.EnforcementFull,
		}, nil
	}
	mgr := newPooledManager(t, eng)
	lease, err := mgr.Acquire(context.Background(), sandbox.AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer func() { _ = lease.Release(context.Background()) }()

	rt := newPooledRuntime(mgr)
	out, err := rt.Run(context.Background(), lease, "python", "print('x')", 10*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !contains(out.Stderr, "[stderr truncated at 262144 bytes]") {
		t.Fatalf("stderr truncation note lost: %q", out.Stderr)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
