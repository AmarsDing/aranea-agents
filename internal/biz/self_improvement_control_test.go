package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ── control plane 单元 ───────────────────────────────────────────────────────

func TestSIControlPlane_IssuePollClear(t *testing.T) {
	cp := NewSIControlPlane()
	if err := cp.Issue("run-1", SIControlCommand("bogus")); err == nil {
		t.Fatal("invalid command must error")
	}
	if err := cp.Issue("", SIControlPause); err == nil {
		t.Fatal("empty runID must error")
	}
	if err := cp.Issue("run-1", SIControlPause); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	cmd, ok := cp.Poll("run-1")
	if !ok || cmd != SIControlPause {
		t.Fatalf("Poll = %q,%v, want pause,true", cmd, ok)
	}
	if _, ok := cp.Poll("run-1"); ok {
		t.Fatal("Poll must consume once")
	}
	if err := cp.Issue("run-1", SIControlRollback); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	cp.Clear("run-1")
	if _, ok := cp.Poll("run-1"); ok {
		t.Fatal("Clear must drop pending command")
	}
}

func TestParseSIControlCommand(t *testing.T) {
	for _, s := range []string{"pause", "skip_retry", "rollback"} {
		if _, err := ParseSIControlCommand(s); err != nil {
			t.Errorf("Parse(%q): %v", s, err)
		}
	}
	if _, err := ParseSIControlCommand("bogus"); err == nil {
		t.Error("bogus must error")
	}
}

// ── 暂停：流水线在 patching 入口退出，run 停留非终态（可恢复） ──────────────

func TestSIPipeline_ControlPause(t *testing.T) {
	cp := NewSIControlPlane()
	uc, store, _, patchReqs := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Control = cp
		d.Analyst = siAnalystFn(func(ctx context.Context, run *SelfImprovementRun, sug *UnifiedEvolutionSuggestion) (*Diagnosis, error) {
			if err := cp.Issue(run.ID, SIControlPause); err != nil {
				t.Errorf("Issue pause: %v", err)
			}
			return &Diagnosis{RootCause: "x", Confidence: 0.9}, nil
		})
	})
	err := uc.Execute(context.Background(), "run-1")
	if !errors.Is(err, ErrSIRunPaused) {
		t.Fatalf("err = %v, want ErrSIRunPaused", err)
	}
	if store.run.Status != RunStatusPatching {
		t.Errorf("status = %s, want patching（非终态，待恢复）", store.run.Status)
	}
	if len(*patchReqs) != 0 {
		t.Errorf("patcher calls = %d, want 0", len(*patchReqs))
	}
}

// ── 强制回滚（pre-apply 中止）：patching 入口消费 → rejected ────────────────

func TestSIPipeline_ControlRollbackPreApply(t *testing.T) {
	cp := NewSIControlPlane()
	uc, store, _, patchReqs := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Control = cp
		d.Analyst = siAnalystFn(func(ctx context.Context, run *SelfImprovementRun, sug *UnifiedEvolutionSuggestion) (*Diagnosis, error) {
			if err := cp.Issue(run.ID, SIControlRollback); err != nil {
				t.Errorf("Issue rollback: %v", err)
			}
			return &Diagnosis{RootCause: "x", Confidence: 0.9}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v（用户中止是正常终态）", err)
	}
	if store.run.Status != RunStatusRejected {
		t.Errorf("status = %s, want rejected", store.run.Status)
	}
	if !strings.Contains(store.run.ClosedReason, "user_rollback") {
		t.Errorf("ClosedReason = %q, want user_rollback", store.run.ClosedReason)
	}
	if len(*patchReqs) != 0 {
		t.Errorf("patcher calls = %d, want 0", len(*patchReqs))
	}
}

// ── 跳过重试：verify 失败后直接终态 verify_failed，不再回 Patcher ───────────

func TestSIPipeline_ControlSkipRetry(t *testing.T) {
	cp := NewSIControlPlane()
	patcherCalls := 0
	uc, store, sandbox, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Control = cp
		d.Patcher = siPatcherFn(func(_ context.Context, req SIPatchRequest) (*PatcherOutput, error) {
			patcherCalls++
			if err := cp.Issue(req.Run.ID, SIControlSkipRetry); err != nil {
				t.Errorf("Issue skip_retry: %v", err)
			}
			return &PatcherOutput{Diff: siRiskDiff("internal/biz/x.go", 5), Kind: PatchKindCode}, nil
		})
	})
	sandbox.gateFn = func(gate SandboxGateKind, _ int) SandboxGateResult {
		return SandboxGateResult{Gate: gate, Passed: false, Output: "boom"}
	}
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusVerifyFailed {
		t.Errorf("status = %s, want verify_failed", store.run.Status)
	}
	if patcherCalls != 1 {
		t.Errorf("patcher calls = %d, want 1（重试被跳过）", patcherCalls)
	}
}

// ── 无控制面时流水线行为不变（nil-safe） ────────────────────────────────────

func TestSIPipeline_ControlNilSafe(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, nil)
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status = %s", store.run.Status)
	}
}
