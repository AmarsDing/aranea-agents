package biz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// TestSIFullChain_ConfigPatch_ObserveToObserving is the P0 verification
// contract for the 2026-08-09 smoke hole: a config-only patch must not pay
// G2 (HEAD 红测), must classify as auto, and must land in observing with
// honest working-tree apply semantics.
func TestSIFullChain_ConfigPatch_ObserveToObserving(t *testing.T) {
	uc, store, sandbox, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Patcher = siPatcherFn(func(context.Context, SIPatchRequest) (*PatcherOutput, error) {
			return &PatcherOutput{Diff: siRiskDiff("configs/config.yaml", 7), Kind: PatchKindConfig, Summary: "tune"}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("after pipeline status=%s, want awaiting_governance", store.run.Status)
	}
	if len(sandbox.gateCalls) != 0 {
		t.Fatalf("config-only must not execute Go/web gates, calls=%v", sandbox.gateCalls)
	}
	for _, g := range store.run.VerificationReport {
		if !g.Skipped || g.Passed {
			t.Fatalf("gate %s = %+v, want skipped", g.Gate, g)
		}
	}
	if store.run.Governance == nil || store.run.Governance.Channel != "auto" {
		t.Fatalf("governance=%+v, want auto", store.run.Governance)
	}

	applyUC, err := NewSelfImprovementApplyUsecase(SelfImprovementApplyUsecaseDeps{
		RunReader: store, RunWriter: store,
		Applier: &siFakeApplier{ref: "snapshot/fullchain"},
		Lg:      loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("apply usecase: %v", err)
	}
	router := NewSIGovernanceRouter(SIGovernanceRouterDeps{
		RunReader: store, RunWriter: store,
		Approvals:            &siFakeApprovalSink{},
		ApplyDriver:          applyUC,
		AutoApplyQuotaPerDay: DefaultSIAutoApplyQuotaPerDay,
		Lg:                   loggateway.NewNoop(),
	})
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "auto" {
		t.Fatalf("channel=%q, want auto", channel)
	}
	if store.run.Status != RunStatusObserving {
		t.Fatalf("after apply status=%s, want observing", store.run.Status)
	}
	if store.run.RollbackPointer != "snapshot/fullchain" {
		t.Fatalf("RollbackPointer=%q", store.run.RollbackPointer)
	}
	if got := siApplyEffectiveOn(store.run); got != siApplyEffectiveRead {
		t.Fatalf("effective_on=%q, want %s", got, siApplyEffectiveRead)
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(store.run.Metadata, &meta); err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if _, ok := meta[siMetaApplySemantics]; !ok {
		t.Fatal("apply_semantics missing from metadata")
	}
}

func TestSIFullChain_CodePatch_StillRunsG2(t *testing.T) {
	uc, store, sandbox, _ := siPipelineFixture(3, nil)
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status=%s", store.run.Status)
	}
	var sawG2 bool
	for _, g := range sandbox.gateCalls {
		if g == SandboxGateTest {
			sawG2 = true
		}
	}
	if !sawG2 {
		t.Fatalf("code+go patch must still execute G2, calls=%v", sandbox.gateCalls)
	}
	if last := store.run.VerificationReport[len(store.run.VerificationReport)-1]; last.Gate != SandboxGateEvalBase || !last.Skipped {
		t.Fatalf("G5 must remain skipped, last=%+v", last)
	}
}

func TestSIFullChain_QuotaZeroForcesApproval(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Patcher = siPatcherFn(func(context.Context, SIPatchRequest) (*PatcherOutput, error) {
			return &PatcherOutput{Diff: siRiskDiff("configs/config.yaml", 7), Kind: PatchKindConfig}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	router := NewSIGovernanceRouter(SIGovernanceRouterDeps{
		RunReader: store, RunWriter: store,
		Approvals:            &siFakeApprovalSink{},
		AutoApplyQuotaPerDay: 0,
		Lg:                   loggateway.NewNoop(),
	})
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "approval" {
		t.Fatalf("channel=%q, want approval when quota=0", channel)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status=%s, want awaiting_governance", store.run.Status)
	}
	if !strings.Contains(strings.Join(store.run.Governance.RuleHits, ","), "quota") &&
		store.run.Governance.Channel != "approval" {
		// Router rewrites channel to approval; stay awaiting_governance.
		t.Logf("governance after quota-0 route: %+v", store.run.Governance)
	}
}
