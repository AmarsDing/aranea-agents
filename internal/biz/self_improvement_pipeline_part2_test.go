package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 回归（2026-08-08 运行时事故）：failWith/rejectWith 直接写变长字符串未截断，
// 超 ent closed_reason MaxLen(64) 时 Update 校验失败 → fail transition broken，
// run 永久卡死在驱动态（只能 stale 恢复后重试再炸，LLM 每 30min 白烧一次）。
// biz 层必须保证写入的 ClosedReason ≤ siClosedReasonMaxLen。
func TestSIPipeline_LongErrorClosedReasonTruncated(t *testing.T) {
	longErr := "patch llm call failed: " + strings.Repeat("context-window-overflow ", 20)
	uc, store, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Analyst = siAnalystFn(func(context.Context, *SelfImprovementRun, *UnifiedEvolutionSuggestion) (*Diagnosis, error) {
			return nil, errors.New(longErr)
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err == nil {
		t.Fatal("Execute must return the stage error")
	}
	if store.run.Status != RunStatusFailed {
		t.Fatalf("status = %s, want failed", store.run.Status)
	}
	if len(store.run.ClosedReason) > siClosedReasonMaxLen {
		t.Fatalf("ClosedReason len = %d, want <= %d（ent MaxLen 守卫）: %q",
			len(store.run.ClosedReason), siClosedReasonMaxLen, store.run.ClosedReason)
	}
	if !strings.HasPrefix(store.run.ClosedReason, "analyst: patch llm call failed") {
		t.Errorf("ClosedReason 前缀信息丢失: %q", store.run.ClosedReason)
	}
}

func TestSIPipeline_NilCriticDegrades(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Critic = nil
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status = %s", store.run.Status)
	}
	if store.run.CriticReport != nil {
		t.Error("critic report must stay nil when stage degraded")
	}
	if store.run.Governance == nil {
		t.Fatal("governance must still be computed without critic")
	}
	for _, h := range store.run.Governance.RuleHits {
		if h == "R4" {
			t.Error("R4 must not fire without critic report")
		}
	}
}

func TestSIPipeline_EntryGuard(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, nil)
	store.run.Status = RunStatusApplied
	if err := uc.Execute(context.Background(), "run-1"); err == nil {
		t.Fatal("Execute on non-detected run must error")
	}
	if err := uc.Execute(context.Background(), "missing"); err == nil {
		t.Fatal("Execute on missing run must error")
	}
}

func TestSIPipeline_WebOnlySkippedLintFailsClosed(t *testing.T) {
	uc, store, sandbox, _ := siPipelineFixture(1, func(d *SelfImprovementPipelineDeps) {
		d.Patcher = siPatcherFn(func(context.Context, SIPatchRequest) (*PatcherOutput, error) {
			return &PatcherOutput{Diff: siRiskDiff("web/src/App.vue", 5), Kind: PatchKindCode}, nil
		})
	})
	sandbox.gateFn = func(gate SandboxGateKind, _ int) SandboxGateResult {
		if gate == SandboxGateWebLint {
			return NewSkippedSandboxGate(gate, "skipped: pnpm not on PATH")
		}
		return SandboxGateResult{Gate: gate, Passed: true}
	}
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusVerifyFailed {
		t.Fatalf("status = %s, want verify_failed (web-only + no pnpm is fail-closed)", store.run.Status)
	}
	if !strings.Contains(siFailedGateDigest(store.run.VerificationReport), "fail-closed") {
		t.Fatalf("report = %+v, want fail-closed digest", store.run.VerificationReport)
	}
}

func TestSIPipeline_G2ExemptsHEADKnownFailures(t *testing.T) {
	uc, store, sandbox, _ := siPipelineFixture(1, nil)
	sandbox.probeFails = []string{"TestOld"}
	sandbox.gateFn = func(gate SandboxGateKind, _ int) SandboxGateResult {
		if gate == SandboxGateTest {
			return SandboxGateResult{
				Gate: gate, Passed: false,
				Output: "--- FAIL: TestOld (0.01s)\nFAIL\taranea-agents/internal/biz\t0.02s\n",
			}
		}
		return SandboxGateResult{Gate: gate, Passed: true}
	}
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if sandbox.probeCalls == 0 {
		t.Fatal("G2 must probe HEAD failures before ApplyDiff")
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status = %s, want awaiting_governance after exempting HEAD-known G2 fail", store.run.Status)
	}
	var g2 SandboxGateResult
	for _, r := range store.run.VerificationReport {
		if r.Gate == SandboxGateTest {
			g2 = r
		}
	}
	if !g2.Passed || !strings.Contains(g2.Output, "exempted HEAD-known") {
		t.Fatalf("G2 = %+v, want passed with exemption note", g2)
	}
}

func TestSIFailClosedWebLint(t *testing.T) {
	webPlan := PlanSIVerification(PatchKindCode, []PatchFileChange{{Path: "web/src/App.vue", Kind: PatchChangeModified}})
	skipped := NewSkippedSandboxGate(SandboxGateWebLint, "skipped: pnpm not on PATH")
	closed, fail := siFailClosedWebLint(webPlan, skipped)
	if !closed || fail.Passed || fail.Skipped {
		t.Fatalf("web-only skip = closed=%v res=%+v", closed, fail)
	}
	cfgPlan := PlanSIVerification(PatchKindConfig, []PatchFileChange{{Path: "web/src/i18n.ts", Kind: PatchChangeModified}})
	if closed, _ = siFailClosedWebLint(cfgPlan, skipped); closed {
		t.Fatal("config+web skip must not fail-closed")
	}
}
