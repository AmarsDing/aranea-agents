package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── admin fixture ────────────────────────────────────────────────────────────

func siAdminFixture(t *testing.T, runs []SelfImprovementRun, mutate func(*SelfImprovementAdminDeps)) (*SelfImprovementAdminUsecase, *siRunStore, *siWatchApplierFake, *siDriveApplyFake) {
	t.Helper()
	store := &siRunStore{}
	if len(runs) > 0 {
		r0 := runs[0]
		store.run = &r0
		store.others = append(store.others, runs[1:]...)
	}
	applier := &siWatchApplierFake{}
	driver := &siDriveApplyFake{}
	deps := SelfImprovementAdminDeps{
		RunReader: store, RunWriter: store,
		Applier: applier, ApplyDriver: driver,
		Lg: loggateway.NewNoop(),
	}
	if mutate != nil {
		mutate(&deps)
	}
	uc, err := NewSelfImprovementAdminUsecase(deps)
	if err != nil {
		t.Fatalf("NewSelfImprovementAdminUsecase: %v", err)
	}
	return uc, store, applier, driver
}

func siAdminRun(id string, status SelfImprovementRunStatus) SelfImprovementRun {
	return SelfImprovementRun{
		ID: id, SuggestionID: "sug-" + id, Status: status,
		TriggerSource: TriggerSourceErrorCluster,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now(),
	}
}

// ── tests ────────────────────────────────────────────────────────────────────

// Approve：awaiting_governance → applying，记 ApprovedBy 并驱动 apply。
func TestSIAdmin_ApproveTransitionsAndDrives(t *testing.T) {
	uc, store, _, driver := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusAwaitingGovernance)}, nil)
	if err := uc.Approve(context.Background(), "run-1", "alice", "looks safe"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if store.run.Status != RunStatusApplying {
		t.Fatalf("status = %s, want applying", store.run.Status)
	}
	if store.run.ApprovedBy != "alice" {
		t.Fatalf("ApprovedBy = %q, want alice", store.run.ApprovedBy)
	}
	if len(driver.applyCalls) != 1 || driver.applyCalls[0] != "run-1" {
		t.Fatalf("driver 应被驱动一次, 实际 %v", driver.applyCalls)
	}
}

// Approve 非 awaiting_governance → Conflict，不迁移不驱动。
func TestSIAdmin_ApproveWrongStatusConflict(t *testing.T) {
	uc, store, _, driver := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusDetected)}, nil)
	err := uc.Approve(context.Background(), "run-1", "alice", "")
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("err = %v, want Conflict", err)
	}
	if store.run.Status != RunStatusDetected || len(driver.applyCalls) != 0 {
		t.Fatalf("不应迁移/驱动: status=%s drives=%v", store.run.Status, driver.applyCalls)
	}
}

// Reject：reason 必填（BadRequest）；带 reason → rejected 终态。
func TestSIAdmin_RejectRequiresReason(t *testing.T) {
	uc, store, _, _ := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusAwaitingGovernance)}, nil)
	if err := uc.Reject(context.Background(), "run-1", "alice", "  "); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("空 reason err = %v, want BadRequest", err)
	}
	if err := uc.Reject(context.Background(), "run-1", "alice", "too risky"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if store.run.Status != RunStatusRejected {
		t.Fatalf("status = %s, want rejected", store.run.Status)
	}
	if store.run.ClosedReason == "" {
		t.Fatal("ClosedReason 应记录拒绝理由")
	}
}

// Rollback observing：先经 applier 实际回退，再 CAS 迁移 rolled_back。
func TestSIAdmin_RollbackObserving(t *testing.T) {
	uc, store, applier, _ := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusObserving)}, nil)
	if err := uc.Rollback(context.Background(), "run-1", "alice", "p95 regression"); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if applier.rollbackCalls != 1 || applier.lastReason != "p95 regression" {
		t.Fatalf("applier 应回退一次并透传 reason, 实际 calls=%d reason=%q", applier.rollbackCalls, applier.lastReason)
	}
	if store.run.Status != RunStatusRolledBack {
		t.Fatalf("status = %s, want rolled_back", store.run.Status)
	}
}

// Rollback applied（排队中）同样允许。
func TestSIAdmin_RollbackApplied(t *testing.T) {
	uc, store, applier, _ := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusApplied)}, nil)
	if err := uc.Rollback(context.Background(), "run-1", "alice", ""); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if applier.rollbackCalls != 1 {
		t.Fatalf("applier 应回退一次, 实际 %d", applier.rollbackCalls)
	}
	if store.run.Status != RunStatusRolledBack {
		t.Fatalf("status = %s, want rolled_back", store.run.Status)
	}
}

// Rollback 终态 run → Conflict，applier 不被调用。
func TestSIAdmin_RollbackTerminalConflict(t *testing.T) {
	uc, _, applier, _ := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusClosed)}, nil)
	err := uc.Rollback(context.Background(), "run-1", "alice", "")
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("err = %v, want Conflict", err)
	}
	if applier.rollbackCalls != 0 {
		t.Fatalf("终态不应触发 applier, 实际 %d", applier.rollbackCalls)
	}
}

// applier 回退失败：错误透传，run 状态保持，供操作员重试。
func TestSIAdmin_RollbackApplierFailurePropagates(t *testing.T) {
	uc, store, applier, _ := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusObserving)}, nil)
	applier.err = errors.New("git revert failed")
	if err := uc.Rollback(context.Background(), "run-1", "alice", ""); err == nil {
		t.Fatal("applier 失败应透传")
	}
	if store.run.Status != RunStatusObserving {
		t.Fatalf("失败不应迁移状态, status = %s", store.run.Status)
	}
}

// Close observing → closed 终态。
func TestSIAdmin_CloseObserving(t *testing.T) {
	uc, store, _, _ := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusObserving)}, nil)
	if err := uc.Close(context.Background(), "run-1", "alice"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.run.Status != RunStatusClosed {
		t.Fatalf("status = %s, want closed", store.run.Status)
	}
}

// Close applied（未入观察窗）→ Conflict。
func TestSIAdmin_CloseAppliedConflict(t *testing.T) {
	uc, store, _, _ := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusApplied)}, nil)
	err := uc.Close(context.Background(), "run-1", "alice")
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("err = %v, want Conflict", err)
	}
	if store.run.Status != RunStatusApplied {
		t.Fatalf("不应迁移, status = %s", store.run.Status)
	}
}

func TestSIAdmin_ConstructorGuards(t *testing.T) {
	if _, err := NewSelfImprovementAdminUsecase(SelfImprovementAdminDeps{}); err == nil {
		t.Fatal("缺依赖应报错")
	}
	if _, err := NewSelfImprovementAdminUsecase(SelfImprovementAdminDeps{
		RunReader: &siRunStore{}, RunWriter: &siRunStore{}, Lg: loggateway.NewNoop(),
	}); err == nil {
		t.Fatal("缺 Applier 应报错")
	}
}

// ── console query surface (P5, design §7 ListRuns/GetRun/GetOutcomeStats) ────

type siStatsReaderFake struct {
	rows []SITriggerVerdictCount
	err  error
}

func (f *siStatsReaderFake) AggregateOutcomeStats(context.Context) ([]SITriggerVerdictCount, error) {
	return f.rows, f.err
}

// List：按筛选返回 items + total（console 分页数据源）。
func TestSIAdmin_ListReturnsRunsAndTotal(t *testing.T) {
	uc, _, _, _ := siAdminFixture(t, []SelfImprovementRun{
		siAdminRun("run-1", RunStatusDetected),
		siAdminRun("run-2", RunStatusObserving),
	}, nil)

	runs, total, err := uc.List(context.Background(), RunFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(runs) != 2 {
		t.Fatalf("unfiltered total=%d len=%d, want 2/2", total, len(runs))
	}

	runs, total, err = uc.List(context.Background(), RunFilter{Status: RunStatusObserving})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if total != 1 || len(runs) != 1 || runs[0].ID != "run-2" {
		t.Fatalf("filtered total=%d runs=%v, want run-2 only", total, runs)
	}
}

// Get：存在返回详情；缺失 → NotFound（service 映射 404）。
func TestSIAdmin_GetRun(t *testing.T) {
	uc, _, _, _ := siAdminFixture(t,
		[]SelfImprovementRun{siAdminRun("run-1", RunStatusDetected)}, nil)
	run, err := uc.Get(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if run.ID != "run-1" {
		t.Fatalf("run.ID = %q", run.ID)
	}
	if _, err := uc.Get(context.Background(), "nope"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("missing run err = %v, want NotFound", err)
	}
}

// OutcomeStats：聚合行 → 总量 + 分触发源 + 比率（design §7 GetOutcomeStats）。
func TestSIAdmin_OutcomeStatsAggregates(t *testing.T) {
	uc, _, _, _ := siAdminFixture(t, nil, func(d *SelfImprovementAdminDeps) {
		d.StatsReader = &siStatsReaderFake{rows: []SITriggerVerdictCount{
			{TriggerSource: TriggerSourceTestFailure, Verdict: VerdictNeutral, Count: 2},
			{TriggerSource: TriggerSourceErrorCluster, Verdict: VerdictEffective, Count: 3},
			{TriggerSource: TriggerSourceErrorCluster, Verdict: VerdictRegressed, Count: 1},
		}}
	})
	stats, err := uc.OutcomeStats(context.Background())
	if err != nil {
		t.Fatalf("OutcomeStats: %v", err)
	}
	if stats.Total != 6 || stats.Effective != 3 || stats.Neutral != 2 || stats.Regressed != 1 {
		t.Fatalf("totals = %+v, want 6/3/2/1", stats)
	}
	if stats.EffectiveRate != 0.5 {
		t.Fatalf("EffectiveRate = %v, want 0.5", stats.EffectiveRate)
	}
	if want := 1.0 / 6.0; stats.RollbackRate != want {
		t.Fatalf("RollbackRate = %v, want %v", stats.RollbackRate, want)
	}
	if len(stats.ByTrigger) != 2 {
		t.Fatalf("ByTrigger len = %d, want 2", len(stats.ByTrigger))
	}
	// 确定性排序：按 trigger_source 升序。
	ec := stats.ByTrigger[0]
	if ec.TriggerSource != TriggerSourceErrorCluster || ec.Total != 4 || ec.Effective != 3 || ec.Regressed != 1 {
		t.Fatalf("ByTrigger[0] = %+v, want error_cluster 4/3/0/1", ec)
	}
	tf := stats.ByTrigger[1]
	if tf.TriggerSource != TriggerSourceTestFailure || tf.Total != 2 || tf.Neutral != 2 {
		t.Fatalf("ByTrigger[1] = %+v, want test_failure 2/0/2/0", tf)
	}
}

// OutcomeStats 空集：全零不 panic（total=0 时比率为 0）。
func TestSIAdmin_OutcomeStatsEmpty(t *testing.T) {
	uc, _, _, _ := siAdminFixture(t, nil, func(d *SelfImprovementAdminDeps) {
		d.StatsReader = &siStatsReaderFake{}
	})
	stats, err := uc.OutcomeStats(context.Background())
	if err != nil {
		t.Fatalf("OutcomeStats: %v", err)
	}
	if stats.Total != 0 || stats.EffectiveRate != 0 || stats.RollbackRate != 0 || len(stats.ByTrigger) != 0 {
		t.Fatalf("empty stats = %+v, want zeros", stats)
	}
}

// StatsReader 未接线（SI 部分降级）→ 明确错误而非 panic。
func TestSIAdmin_OutcomeStatsNilReader(t *testing.T) {
	uc, _, _, _ := siAdminFixture(t, nil, nil)
	if _, err := uc.OutcomeStats(context.Background()); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("err = %v, want Internal", err)
	}
}

// ── risk rules (P5, design §7 UpdateRiskRules) ───────────────────────────────

type siRiskRuleRepoFake struct {
	rules SIRiskRules
	err   error
}

func (f *siRiskRuleRepoFake) GetSIRiskRules(context.Context) (SIRiskRules, error) {
	return f.rules, f.err
}

func (f *siRiskRuleRepoFake) UpdateSIRiskRules(_ context.Context, rules SIRiskRules) (SIRiskRules, error) {
	if f.err != nil {
		return SIRiskRules{}, f.err
	}
	f.rules = rules
	return rules, nil
}

// GetRiskRules：repo 原始值透传；未接线 → 代码默认。
func TestSIAdmin_GetRiskRules(t *testing.T) {
	fake := &siRiskRuleRepoFake{rules: SIRiskRules{LowMaxLines: 50}}
	uc, _, _, _ := siAdminFixture(t, nil, func(d *SelfImprovementAdminDeps) { d.RiskRules = fake })
	got, err := uc.GetRiskRules(context.Background())
	if err != nil {
		t.Fatalf("GetRiskRules: %v", err)
	}
	if got.LowMaxLines != 50 {
		t.Fatalf("LowMaxLines = %d, want 50", got.LowMaxLines)
	}

	ucNil, _, _, _ := siAdminFixture(t, nil, nil)
	got, err = ucNil.GetRiskRules(context.Background())
	if err != nil {
		t.Fatalf("GetRiskRules nil repo: %v", err)
	}
	if got.LowMaxLines != DefaultSIRiskRules().LowMaxLines {
		t.Fatalf("nil repo 应返回代码默认, got %+v", got)
	}
}

// UpdateRiskRules：合法规则持久化并透传。
func TestSIAdmin_UpdateRiskRulesPersists(t *testing.T) {
	fake := &siRiskRuleRepoFake{}
	uc, _, _, _ := siAdminFixture(t, nil, func(d *SelfImprovementAdminDeps) { d.RiskRules = fake })
	want := SIRiskRules{
		LowMaxLines:    50,
		MediumMaxLines: 200,
		CorePathGlobs:  []string{"internal/service/**"},
		DailyAutoQuota: 2,
	}
	got, err := uc.UpdateRiskRules(context.Background(), "alice", want)
	if err != nil {
		t.Fatalf("UpdateRiskRules: %v", err)
	}
	if got.LowMaxLines != 50 || fake.rules.DailyAutoQuota != 2 {
		t.Fatalf("persisted rules = %+v, want %+v", fake.rules, want)
	}
}

// UpdateRiskRules：非法输入 → BadRequest，不落库。
func TestSIAdmin_UpdateRiskRulesValidation(t *testing.T) {
	fake := &siRiskRuleRepoFake{}
	uc, _, _, _ := siAdminFixture(t, nil, func(d *SelfImprovementAdminDeps) { d.RiskRules = fake })
	ctx := context.Background()

	cases := []struct {
		name  string
		rules SIRiskRules
	}{
		{"negative threshold", SIRiskRules{LowMaxLines: -1}},
		{"low > medium", SIRiskRules{LowMaxLines: 300, MediumMaxLines: 100}},
		{"blank glob", SIRiskRules{CorePathGlobs: []string{"  "}}},
		{"invalid glob", SIRiskRules{CorePathGlobs: []string{"[unclosed"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := uc.UpdateRiskRules(ctx, "alice", c.rules); !apierror.IsCode(err, apierror.CodeBadRequest) {
				t.Fatalf("err = %v, want BadRequest", err)
			}
		})
	}
}

// UpdateRiskRules：repo 未接线 → Internal。
func TestSIAdmin_UpdateRiskRulesNilRepo(t *testing.T) {
	uc, _, _, _ := siAdminFixture(t, nil, nil)
	if _, err := uc.UpdateRiskRules(context.Background(), "alice", SIRiskRules{}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("err = %v, want Internal", err)
	}
}
