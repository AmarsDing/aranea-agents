package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ── router fakes ─────────────────────────────────────────────────────────────

type siNotifyCall struct {
	runID   string
	message string
}
type siFakeNotifier struct{ calls []siNotifyCall }

func (n *siFakeNotifier) NotifySelfImprovement(_ context.Context, run *SelfImprovementRun, message string) error {
	n.calls = append(n.calls, siNotifyCall{runID: run.ID, message: message})
	return nil
}

type siFakeApprovalSink struct{ submitted []string }

func (s *siFakeApprovalSink) SubmitApproval(_ context.Context, run *SelfImprovementRun) (string, error) {
	s.submitted = append(s.submitted, run.ID)
	return "appr-1", nil
}

func siRouterFixture(channel string, mutate func(*SIGovernanceRouterDeps)) (*SIGovernanceRouter, *siRunStore, *siFakeNotifier, *siFakeApprovalSink) {
	store := &siRunStore{run: &SelfImprovementRun{
		ID: "run-1", SuggestionID: "sug-1", Status: RunStatusAwaitingGovernance,
		TriggerSource: TriggerSourceErrorCluster,
		Governance:    &GovernanceDecision{RiskLevel: RiskLevelLow, Channel: channel, RuleHits: []string{"R1"}},
		RiskLevel:     RiskLevelLow,
	}}
	notifier := &siFakeNotifier{}
	approvals := &siFakeApprovalSink{}
	deps := SIGovernanceRouterDeps{
		RunReader: store, RunWriter: store,
		Notifier: notifier, Approvals: approvals,
		AutoApplyQuotaPerDay: DefaultSIAutoApplyQuotaPerDay,
		Lg:                   loggateway.NewNoop(),
	}
	if mutate != nil {
		mutate(&deps)
	}
	return NewSIGovernanceRouter(deps), store, notifier, approvals
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestSIGovernanceRouter_AutoWithinQuota(t *testing.T) {
	router, store, notifier, approvals := siRouterFixture("auto", nil)
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "auto" {
		t.Errorf("channel = %q, want auto", channel)
	}
	if store.run.Status != RunStatusApplying {
		t.Errorf("status = %s, want applying", store.run.Status)
	}
	if store.run.ApprovedBy != "system:auto" {
		t.Errorf("ApprovedBy = %q, want system:auto", store.run.ApprovedBy)
	}
	if len(notifier.calls) != 0 {
		t.Error("auto channel must not notify")
	}
	if len(approvals.submitted) != 0 {
		t.Error("auto channel must not submit approval")
	}
}

func TestSIGovernanceRouter_NotifyChannelAppliesAndNotifies(t *testing.T) {
	router, store, notifier, approvals := siRouterFixture("notify", nil)
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "notify" {
		t.Errorf("channel = %q, want notify", channel)
	}
	if store.run.Status != RunStatusApplying {
		t.Errorf("status = %s, want applying", store.run.Status)
	}
	if store.run.ApprovedBy != "system:auto+notify" {
		t.Errorf("ApprovedBy = %q, want system:auto+notify", store.run.ApprovedBy)
	}
	if len(notifier.calls) != 1 || notifier.calls[0].runID != "run-1" {
		t.Errorf("notify calls = %+v, want 1 for run-1", notifier.calls)
	}
	if len(approvals.submitted) != 0 {
		t.Error("notify within quota must not submit approval")
	}
}

func TestSIGovernanceRouter_AutoQuotaExhaustedEscalatesToApproval(t *testing.T) {
	router, store, _, approvals := siRouterFixture("auto", func(d *SIGovernanceRouterDeps) {
		d.AutoApplyQuotaPerDay = 1
	})
	// 第 1 个 run 消耗唯一直配额
	if _, err := router.Route(context.Background(), "run-1"); err != nil {
		t.Fatalf("Route #1: %v", err)
	}
	if store.run.Status != RunStatusApplying {
		t.Fatalf("run-1 status = %s, want applying", store.run.Status)
	}
	// 第 2 个 run：配额耗尽 → 转 approval（D10）
	store.run = &SelfImprovementRun{
		ID: "run-2", Status: RunStatusAwaitingGovernance,
		Governance: &GovernanceDecision{RiskLevel: RiskLevelLow, Channel: "auto"},
	}
	channel, err := router.Route(context.Background(), "run-2")
	if err != nil {
		t.Fatalf("Route #2: %v", err)
	}
	if channel != "approval" {
		t.Errorf("channel = %q, want approval（超限转人工）", channel)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Errorf("status = %s, want awaiting_governance（等待审批）", store.run.Status)
	}
	if len(approvals.submitted) != 1 || approvals.submitted[0] != "run-2" {
		t.Errorf("approvals = %+v, want run-2", approvals.submitted)
	}
}

func TestSIGovernanceRouter_ZeroQuotaDisablesAutoApply(t *testing.T) {
	router, store, _, approvals := siRouterFixture("auto", func(d *SIGovernanceRouterDeps) {
		d.AutoApplyQuotaPerDay = 0
	})
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "approval" {
		t.Errorf("channel = %q, want approval（配额 0 = 关闭 auto-apply）", channel)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Errorf("status = %s, want awaiting_governance", store.run.Status)
	}
	if len(approvals.submitted) != 1 || approvals.submitted[0] != "run-1" {
		t.Errorf("approvals = %+v, want run-1", approvals.submitted)
	}
}

func TestSIGovernanceRouter_ApprovalChannelStaysWaiting(t *testing.T) {
	router, store, _, approvals := siRouterFixture("approval", nil)
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "approval" {
		t.Errorf("channel = %q, want approval", channel)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Errorf("status = %s, want awaiting_governance", store.run.Status)
	}
	if len(approvals.submitted) != 1 {
		t.Errorf("approvals = %+v, want 1", approvals.submitted)
	}
}

func TestSIGovernanceRouter_RejectChannel(t *testing.T) {
	router, store, _, _ := siRouterFixture("reject", nil)
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "reject" || store.run.Status != RunStatusRejected {
		t.Errorf("channel=%q status=%s, want reject/rejected", channel, store.run.Status)
	}
}

// 回归（2026-08-08）：reject 直接拼接 RuleHits 未截断，命中规则多/文案长时
// 超 ent closed_reason MaxLen(64) → Update 校验失败 → reject 无法落库。
func TestSIGovernanceRouter_RejectTruncatesClosedReason(t *testing.T) {
	router, store, _, _ := siRouterFixture("reject", nil)
	store.run.Governance.RuleHits = []string{"R1", strings.Repeat("core-path-glob-hit/", 8)}
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "reject" || store.run.Status != RunStatusRejected {
		t.Fatalf("channel=%q status=%s, want reject/rejected", channel, store.run.Status)
	}
	if len(store.run.ClosedReason) > siClosedReasonMaxLen {
		t.Fatalf("ClosedReason len = %d, want <= %d: %q",
			len(store.run.ClosedReason), siClosedReasonMaxLen, store.run.ClosedReason)
	}
	if !strings.HasPrefix(store.run.ClosedReason, "governance reject: R1") {
		t.Errorf("ClosedReason 前缀信息丢失: %q", store.run.ClosedReason)
	}
}

func TestSIGovernanceRouter_EntryGuards(t *testing.T) {
	router, store, _, _ := siRouterFixture("auto", nil)
	store.run.Status = RunStatusApplying // 非 awaiting_governance
	if _, err := router.Route(context.Background(), "run-1"); err == nil {
		t.Fatal("non awaiting_governance must error")
	}
	store.run = &SelfImprovementRun{ID: "run-9", Status: RunStatusAwaitingGovernance, Governance: nil}
	if _, err := router.Route(context.Background(), "run-9"); err == nil {
		t.Fatal("nil governance must error")
	}
}

// TestSIPipeline_EndToEnd_AutoChannel exercises T3.5 DoD: 信号(建议)→诊断→补丁
// →验证→治理路由(auto→applying)，全 mock（LLM stages + sandbox）。
func TestSIPipeline_EndToEnd_AutoChannel(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.Patcher = siPatcherFn(func(context.Context, SIPatchRequest) (*PatcherOutput, error) {
			// 低风险：单文件 config 补丁 ≤100 行 → R1 → low → auto
			return &PatcherOutput{Diff: siRiskDiff("configs/config.yaml", 10), Kind: PatchKindConfig}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("pipeline Execute: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("post-pipeline status = %s, want awaiting_governance", store.run.Status)
	}
	if store.run.Governance == nil || store.run.Governance.Channel != "auto" {
		t.Fatalf("governance = %+v, want auto channel", store.run.Governance)
	}
	notifier := &siFakeNotifier{}
	approvals := &siFakeApprovalSink{}
	router := NewSIGovernanceRouter(SIGovernanceRouterDeps{
		RunReader: store, RunWriter: store, Notifier: notifier, Approvals: approvals,
		AutoApplyQuotaPerDay: DefaultSIAutoApplyQuotaPerDay, Lg: loggateway.NewNoop(),
	})
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "auto" || store.run.Status != RunStatusApplying {
		t.Errorf("end-to-end: channel=%q status=%s, want auto/applying", channel, store.run.Status)
	}
	if !strings.Contains(store.run.ApprovedBy, "auto") {
		t.Errorf("ApprovedBy = %q", store.run.ApprovedBy)
	}
}

// ── apply driver hook (T4.5) ────────────────────────────────────────────────

type siFakeApplyDriver struct {
	calls []string
	err   error
}

func (d *siFakeApplyDriver) Apply(_ context.Context, runID string) error {
	d.calls = append(d.calls, runID)
	return d.err
}

func TestSIGovernanceRouter_AutoApplyDrivesApply(t *testing.T) {
	driver := &siFakeApplyDriver{}
	router, store, _, _ := siRouterFixture("auto", func(d *SIGovernanceRouterDeps) {
		d.ApplyDriver = driver
	})
	channel, err := router.Route(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if channel != "auto" {
		t.Fatalf("channel = %q, want auto", channel)
	}
	if len(driver.calls) != 1 || driver.calls[0] != "run-1" {
		t.Fatalf("driver calls = %v, want [run-1]", driver.calls)
	}
	// fake driver 不做状态迁移：run 停留 applying 等待真实驱动结果。
	if store.run.Status != RunStatusApplying {
		t.Fatalf("status = %s, want applying", store.run.Status)
	}
}

func TestSIGovernanceRouter_DriverErrorPropagates(t *testing.T) {
	driver := &siFakeApplyDriver{err: errors.New("drive boom")}
	router, store, _, _ := siRouterFixture("auto", func(d *SIGovernanceRouterDeps) {
		d.ApplyDriver = driver
	})
	if _, err := router.Route(context.Background(), "run-1"); err == nil {
		t.Fatalf("driver 错误应透传")
	}
	// 驱动失败不回滚路由迁移：run 停留 applying，由 resume worker 重驱动。
	if store.run.Status != RunStatusApplying {
		t.Fatalf("status = %s, want applying", store.run.Status)
	}
}

func TestSIGovernanceRouter_ApprovalChannelDoesNotDrive(t *testing.T) {
	driver := &siFakeApplyDriver{}
	router, _, _, _ := siRouterFixture("approval", func(d *SIGovernanceRouterDeps) {
		d.ApplyDriver = driver
	})
	if _, err := router.Route(context.Background(), "run-1"); err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(driver.calls) != 0 {
		t.Fatalf("approval 通道不应触发 apply driver: %v", driver.calls)
	}
}
