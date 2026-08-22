package biz

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── apply-usecase fakes ──────────────────────────────────────────────────────

type siFakeApplier struct {
	mu                 sync.Mutex
	hotCalls           int
	mergeCalls         int
	rollbackCalls      int
	lastRollbackReason string
	hotErr             error
	mergeErr           error
	rollbackErr        error
	ref                string
	sha                string
}

// counts returns (hot, merge) call counts, race-safe.
func (a *siFakeApplier) counts() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hotCalls, a.mergeCalls
}

func (a *siFakeApplier) ApplyHotReload(_ context.Context, _ *SelfImprovementRun) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hotCalls++
	if a.hotErr != nil {
		return "", a.hotErr
	}
	if a.ref == "" {
		a.ref = "snapshot/fake"
	}
	return a.ref, nil
}

func (a *siFakeApplier) ApplyCodeMerge(_ context.Context, _ *SelfImprovementRun) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mergeCalls++
	if a.mergeErr != nil {
		return "", a.mergeErr
	}
	if a.sha == "" {
		a.sha = "sha-fake"
	}
	return a.sha, nil
}

func (a *siFakeApplier) Rollback(_ context.Context, _ *SelfImprovementRun, reason string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rollbackCalls++
	a.lastRollbackReason = reason
	return a.rollbackErr
}

// rollbackStats returns (calls, lastReason), race-safe.
func (a *siFakeApplier) rollbackStats() (int, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rollbackCalls, a.lastRollbackReason
}

// ── helpers ──────────────────────────────────────────────────────────────────

// siApplyDiff builds a minimal unified diff touching path (modify).
func siApplyDiff(path string) string {
	return "--- a/" + path + "\n+++ b/" + path + "\n@@ -1 +1 @@\n-old\n+new\n"
}

func siApplyingRun(kind SelfImprovementPatchKind, diffPath string) *SelfImprovementRun {
	return &SelfImprovementRun{
		ID: "run-1", SuggestionID: "sug-1", Status: RunStatusApplying,
		TriggerSource: TriggerSourceErrorCluster,
		PatchKind:     kind,
		Diff:          siApplyDiff(diffPath),
		Governance:    &GovernanceDecision{RiskLevel: RiskLevelLow, Channel: "auto", RuleHits: []string{"R1"}},
		RiskLevel:     RiskLevelLow,
	}
}

func siApplyFixture(t *testing.T, run *SelfImprovementRun, mutate func(*SelfImprovementApplyUsecaseDeps)) (*SelfImprovementApplyUsecase, *siRunStore, *siFakeApplier, *siFakeApprovalSink) {
	t.Helper()
	store := &siRunStore{run: run}
	applier := &siFakeApplier{}
	approvals := &siFakeApprovalSink{}
	deps := SelfImprovementApplyUsecaseDeps{
		RunReader: store, RunWriter: store,
		Applier:   applier,
		Approvals: approvals,
		Lg:        loggateway.NewNoop(),
	}
	if mutate != nil {
		mutate(&deps)
	}
	uc, err := NewSelfImprovementApplyUsecase(deps)
	if err != nil {
		t.Fatalf("NewSelfImprovementApplyUsecase: %v", err)
	}
	return uc, store, applier, approvals
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestSIApplyUsecase_CodeKindMergesAndObserves(t *testing.T) {
	uc, store, applier, _ := siApplyFixture(t, siApplyingRun(PatchKindCode, "internal/biz/foo.go"), nil)

	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if hot, merge := applier.counts(); merge != 1 || hot != 0 {
		t.Fatalf("kind 路由错误: merge=%d hot=%d", merge, hot)
	}
	run := store.run
	if run.AppliedCommit != "sha-fake" {
		t.Fatalf("AppliedCommit = %q, 期望 sha-fake", run.AppliedCommit)
	}
	if run.Status != RunStatusObserving {
		t.Fatalf("Status = %s, 期望 observing", run.Status)
	}
	if run.ObserveUntil == nil || time.Until(*run.ObserveUntil) < 23*time.Hour {
		t.Fatalf("ObserveUntil 未按默认 24h 观察窗设置: %v", run.ObserveUntil)
	}
	want := [][2]SelfImprovementRunStatus{
		{RunStatusApplying, RunStatusApplied},
		{RunStatusApplied, RunStatusObserving},
	}
	if fmt.Sprintf("%v", store.transitions) != fmt.Sprintf("%v", want) {
		t.Fatalf("transitions = %v, 期望 %v", store.transitions, want)
	}
}

func TestSIApplyUsecase_SoftKindsHotReload(t *testing.T) {
	for _, kind := range []SelfImprovementPatchKind{PatchKindConfig, PatchKindPrompt, PatchKindDocs} {
		t.Run(string(kind), func(t *testing.T) {
			uc, store, applier, _ := siApplyFixture(t, siApplyingRun(kind, "configs/x.yaml"), nil)
			if err := uc.Apply(context.Background(), "run-1"); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if hot, merge := applier.counts(); hot != 1 || merge != 0 {
				t.Fatalf("kind %s 路由错误: hot=%d merge=%d", kind, hot, merge)
			}
			if store.run.RollbackPointer != "snapshot/fake" {
				t.Fatalf("RollbackPointer = %q, 期望 snapshot/fake", store.run.RollbackPointer)
			}
			if store.run.Status != RunStatusObserving {
				t.Fatalf("Status = %s, 期望 observing", store.run.Status)
			}
			if got := siApplyEffectiveOn(store.run); got != siApplyEffectiveRead {
				t.Fatalf("effective_on = %q, want %s (no reloader wired)", got, siApplyEffectiveRead)
			}
		})
	}
}

type siReloadFn func(ctx context.Context, run *SelfImprovementRun) error

func (f siReloadFn) ReloadAfterWorkingTreeApply(ctx context.Context, run *SelfImprovementRun) error {
	return f(ctx, run)
}

func TestSIApplyUsecase_WorkingTreeReloader(t *testing.T) {
	calls := 0
	uc, store, _, _ := siApplyFixture(t, siApplyingRun(PatchKindConfig, "configs/x.yaml"), func(d *SelfImprovementApplyUsecaseDeps) {
		d.Reloader = siReloadFn(func(context.Context, *SelfImprovementRun) error {
			calls++
			return nil
		})
	})
	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if calls != 1 {
		t.Fatalf("reloader calls = %d, want 1", calls)
	}
	if got := siApplyEffectiveOn(store.run); got != siApplyEffectiveLive {
		t.Fatalf("effective_on = %q, want %s", got, siApplyEffectiveLive)
	}
}

func TestSIApplyUsecase_TestKindMerges(t *testing.T) {
	uc, _, applier, _ := siApplyFixture(t, siApplyingRun(PatchKindTest, "internal/biz/foo_test.go"), nil)
	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, merge := applier.counts(); merge != 1 {
		t.Fatalf("test kind 应走代码合并通道, merge=%d", merge)
	}
}

func TestSIApplyUsecase_UnknownKindFails(t *testing.T) {
	run := siApplyingRun("", "internal/biz/foo.go")
	uc, store, applier, _ := siApplyFixture(t, run, nil)
	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if hot, merge := applier.counts(); hot+merge != 0 {
		t.Fatalf("未知 kind 不应调用 applier")
	}
	if store.run.Status != RunStatusFailed {
		t.Fatalf("Status = %s, 期望 failed", store.run.Status)
	}
	if store.run.ClosedReason == "" {
		t.Fatalf("failed 终态应记录 ClosedReason")
	}
}

func TestSIApplyUsecase_MergeConflictEscalates(t *testing.T) {
	conflictErr := fmt.Errorf("%w: patch does not apply", ErrSIMergeConflict)
	uc, store, _, approvals := siApplyFixture(t, siApplyingRun(PatchKindCode, "internal/biz/foo.go"),
		func(d *SelfImprovementApplyUsecaseDeps) {
			d.Applier = &siFakeApplier{mergeErr: conflictErr}
		})

	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply 冲突升级不应报错: %v", err)
	}
	run := store.run
	if run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("Status = %s, 期望 awaiting_governance（D7 冲突转人工）", run.Status)
	}
	if run.Governance.Channel != "approval" {
		t.Fatalf("Channel = %q, 期望 approval", run.Governance.Channel)
	}
	found := false
	for _, h := range run.Governance.RuleHits {
		if h == "merge_conflict" {
			found = true
		}
	}
	if !found {
		t.Fatalf("RuleHits 缺 merge_conflict: %v", run.Governance.RuleHits)
	}
	if len(approvals.submitted) != 1 || approvals.submitted[0] != "run-1" {
		t.Fatalf("冲突升级应提交人工审批: %v", approvals.submitted)
	}
}

func TestSIApplyUsecase_ApplierErrorFails(t *testing.T) {
	uc, store, _, _ := siApplyFixture(t, siApplyingRun(PatchKindCode, "internal/biz/foo.go"),
		func(d *SelfImprovementApplyUsecaseDeps) {
			d.Applier = &siFakeApplier{mergeErr: errors.New("git exploded")}
		})
	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.run.Status != RunStatusFailed {
		t.Fatalf("Status = %s, 期望 failed", store.run.Status)
	}
	if store.run.ClosedReason == "" {
		t.Fatalf("failed 终态应记录 ClosedReason")
	}
}

func TestSIApplyUsecase_ObservingSlotFullStaysApplied(t *testing.T) {
	uc, store, _, _ := siApplyFixture(t, siApplyingRun(PatchKindCode, "internal/biz/foo.go"), nil)
	for i := 0; i < 3; i++ {
		store.others = append(store.others, SelfImprovementRun{
			ID: fmt.Sprintf("obs-%d", i), Status: RunStatusObserving, Diff: siApplyDiff("internal/biz/other.go"),
		})
	}
	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.run.Status != RunStatusApplied {
		t.Fatalf("观察窗满（D10 ≤3）应停留 applied, 实际 %s", store.run.Status)
	}
	if store.run.ObserveUntil != nil {
		t.Fatalf("未入观察窗不应设 ObserveUntil")
	}
}

func TestSIApplyUsecase_CorePathConflictStaysApplied(t *testing.T) {
	uc, store, _, _ := siApplyFixture(t, siApplyingRun(PatchKindCode, "internal/agent/runtime/x.go"), nil)
	store.others = []SelfImprovementRun{
		{ID: "obs-1", Status: RunStatusObserving, Diff: siApplyDiff("internal/agent/other/y.go")},
	}
	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.run.Status != RunStatusApplied {
		t.Fatalf("同核心路径（internal/agent/**）补丁应串行，停留 applied, 实际 %s", store.run.Status)
	}
}

func TestSIApplyUsecase_CorePathDisjointObserves(t *testing.T) {
	uc, store, _, _ := siApplyFixture(t, siApplyingRun(PatchKindCode, "internal/agent/runtime/x.go"), nil)
	store.others = []SelfImprovementRun{
		{ID: "obs-1", Status: RunStatusObserving, Diff: siApplyDiff("internal/service/chat_foo.go")},
	}
	if err := uc.Apply(context.Background(), "run-1"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.run.Status != RunStatusObserving {
		t.Fatalf("核心路径不相交应入观察窗, 实际 %s", store.run.Status)
	}
}

func TestSIApplyUsecase_PromoteEligibleOldestFirst(t *testing.T) {
	uc, store, _, _ := siApplyFixture(t, nil, func(d *SelfImprovementApplyUsecaseDeps) {
		d.MaxConcurrentObserving = 1
	})
	older := SelfImprovementRun{
		ID: "run-old", Status: RunStatusApplied, Diff: siApplyDiff("internal/biz/a.go"),
		CreatedAt: time.Now().Add(-time.Hour),
	}
	newer := SelfImprovementRun{
		ID: "run-new", Status: RunStatusApplied, Diff: siApplyDiff("internal/biz/b.go"),
		CreatedAt: time.Now(),
	}
	// data 层 List 按 created_at DESC 返回（新的在前），编排必须翻转为最老优先。
	store.others = []SelfImprovementRun{newer, older}

	if err := uc.PromoteEligible(context.Background()); err != nil {
		t.Fatalf("PromoteEligible: %v", err)
	}
	promoted := map[string]bool{}
	for _, r := range store.others {
		if r.Status == RunStatusObserving {
			promoted[r.ID] = true
		}
	}
	if !promoted["run-old"] || promoted["run-new"] {
		t.Fatalf("max=1 时仅最老的 run-old 应晋升: %v", promoted)
	}
}

func TestSIApplyUsecase_ConcurrentApplies(t *testing.T) {
	const n = 8
	// 共享一个 usecase + store：8 个 run 并发 Apply，观察窗槽位充足，
	// 全部应进入 observing（验证 usecase 内部无共享态数据竞争，-race）。
	store := &siRunStore{}
	for i := 0; i < n; i++ {
		run := siApplyingRun(PatchKindCode, fmt.Sprintf("internal/biz/f%d.go", i))
		run.ID = fmt.Sprintf("run-%d", i)
		store.others = append(store.others, *run)
	}
	uc, err := NewSelfImprovementApplyUsecase(SelfImprovementApplyUsecaseDeps{
		RunReader: store, RunWriter: store,
		Applier: &siFakeApplier{}, MaxConcurrentObserving: n,
		Lg: loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewSelfImprovementApplyUsecase: %v", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := uc.Apply(context.Background(), id); err != nil {
				errs <- err
			}
		}(fmt.Sprintf("run-%d", i))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发 Apply 出错: %v", err)
	}
	observing := 0
	for _, r := range store.others {
		if r.Status != RunStatusObserving {
			t.Fatalf("run %s 应为 observing, 实际 %s", r.ID, r.Status)
		}
		observing++
	}
	if observing != n {
		t.Fatalf("observing = %d, 期望 %d", observing, n)
	}
}

func TestSIApplyUsecase_EntryGuards(t *testing.T) {
	uc, _, _, _ := siApplyFixture(t, nil, nil)
	if err := uc.Apply(context.Background(), "run-9"); err == nil {
		t.Fatalf("run 不存在应报错")
	}
	// 非法前态：run 处于 detected（仅 applying 可驱动）。
	uc2, _, _, _ := siApplyFixture(t, &SelfImprovementRun{ID: "run-1", Status: RunStatusDetected}, nil)
	if err := uc2.Apply(context.Background(), "run-1"); err == nil {
		t.Fatalf("非 applying 状态应报错")
	}
}
