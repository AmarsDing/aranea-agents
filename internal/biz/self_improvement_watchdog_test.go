package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── watchdog fakes ───────────────────────────────────────────────────────────

type siWatchMetricsFake struct {
	calls int
	snap  *MetricsSnapshot
	err   error
}

func (m *siWatchMetricsFake) Snapshot(_ context.Context, _ time.Duration) (*MetricsSnapshot, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if m.snap != nil {
		cp := *m.snap
		return &cp, nil
	}
	return &MetricsSnapshot{}, nil
}

type siWatchApplierFake struct {
	rollbackCalls int
	lastReason    string
	err           error
}

func (a *siWatchApplierFake) ApplyHotReload(context.Context, *SelfImprovementRun) (string, error) {
	return "", nil
}
func (a *siWatchApplierFake) ApplyCodeMerge(context.Context, *SelfImprovementRun) (string, error) {
	return "", nil
}
func (a *siWatchApplierFake) Rollback(_ context.Context, _ *SelfImprovementRun, reason string) error {
	a.rollbackCalls++
	a.lastReason = reason
	return a.err
}

type siWatchNotifierFake struct {
	calls int
	last  string
}

func (n *siWatchNotifierFake) NotifySelfImprovement(_ context.Context, _ *SelfImprovementRun, msg string) error {
	n.calls++
	n.last = msg
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func siWatchRun(id string, observeUntil *time.Time, baseline *MetricsSnapshot) SelfImprovementRun {
	run := SelfImprovementRun{
		ID: id, SuggestionID: "sug-" + id, Status: RunStatusObserving,
		TriggerSource: TriggerSourceErrorCluster,
		ObserveUntil:  observeUntil,
		UpdatedAt:     time.Now(),
	}
	if baseline != nil {
		raw, _ := json.Marshal(map[string]any{siMetaObserveBaseline: baseline})
		run.Metadata = raw
	}
	return run
}

func siWatchFixture(t *testing.T, runs []SelfImprovementRun, mutate func(*SelfImprovementWatchdogDeps)) (*SelfImprovementWatchdogUsecase, *siRunStore, *siWatchMetricsFake, *siWatchApplierFake, *siWatchNotifierFake) {
	t.Helper()
	store := &siRunStore{others: runs}
	metrics := &siWatchMetricsFake{}
	applier := &siWatchApplierFake{}
	notifier := &siWatchNotifierFake{}
	deps := SelfImprovementWatchdogDeps{
		RunReader: store, RunWriter: store,
		Metrics: metrics, Applier: applier, Notifier: notifier,
		Lg: loggateway.NewNoop(),
	}
	if mutate != nil {
		mutate(&deps)
	}
	uc, err := NewSelfImprovementWatchdogUsecase(deps)
	if err != nil {
		t.Fatalf("NewSelfImprovementWatchdogUsecase: %v", err)
	}
	return uc, store, metrics, applier, notifier
}

func siWatchBaselineOf(t *testing.T, run *SelfImprovementRun) *MetricsSnapshot {
	t.Helper()
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(run.Metadata, &meta); err != nil {
		t.Fatalf("metadata unmarshal: %v", err)
	}
	raw, ok := meta[siMetaObserveBaseline]
	if !ok {
		return nil
	}
	var snap MetricsSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("baseline unmarshal: %v", err)
	}
	return &snap
}

// ── tests ────────────────────────────────────────────────────────────────────

// 首次见到 observing run：采集基线进 metadata，本 tick 不评估。
func TestSIWatchdog_BaselineCapturedOnFirstSight(t *testing.T) {
	past := time.Now().Add(-time.Hour) // 已到期也先采基线
	uc, store, metrics, applier, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, nil)}, nil)
	metrics.snap = &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500}

	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if metrics.calls != 1 {
		t.Fatalf("基线应 Snapshot 一次, 实际 %d", metrics.calls)
	}
	got, _ := store.GetByID(context.Background(), "run-1")
	if got.Status != RunStatusObserving {
		t.Fatalf("采基线后应保持 observing, 实际 %s", got.Status)
	}
	base := siWatchBaselineOf(t, got)
	if base == nil || base.ErrorRate != 0.10 || base.P95MS != 500 {
		t.Fatalf("基线未写入 metadata: %+v", base)
	}
	if applier.rollbackCalls != 0 {
		t.Fatal("采基线 tick 不应回滚")
	}
}

// 观察窗未到期：跳过评估。
func TestSIWatchdog_NotDueSkipped(t *testing.T) {
	future := time.Now().Add(time.Hour)
	uc, _, metrics, applier, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &future, &MetricsSnapshot{ErrorRate: 0.1, P95MS: 100})}, nil)
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if metrics.calls != 0 || applier.rollbackCalls != 0 {
		t.Fatalf("未到期不应评估: metrics=%d rollback=%d", metrics.calls, applier.rollbackCalls)
	}
}

// 到期且指标未退化 → closed（提前确认有效）。
func TestSIWatchdog_DueNoRegressionClosed(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	uc, store, metrics, applier, notifier := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500})}, nil)
	metrics.snap = &MetricsSnapshot{ErrorRate: 0.12, P95MS: 600}

	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	got, _ := store.GetByID(context.Background(), "run-1")
	if got.Status != RunStatusClosed {
		t.Fatalf("未退化应 closed, 实际 %s", got.Status)
	}
	if applier.rollbackCalls != 0 || notifier.calls != 0 {
		t.Fatalf("未退化不应回滚/通知: rollback=%d notify=%d", applier.rollbackCalls, notifier.calls)
	}
}

// 到期且错误率退化 → 自动回滚 + rolled_back + 管理员通知。
func TestSIWatchdog_ErrorRateRegressionRollback(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	uc, store, metrics, applier, notifier := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500})}, nil)
	metrics.snap = &MetricsSnapshot{ErrorRate: 0.20, P95MS: 500} // 0.20 > 0.10×1.5

	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if applier.rollbackCalls != 1 {
		t.Fatalf("错误率退化应回滚一次, 实际 %d", applier.rollbackCalls)
	}
	got, _ := store.GetByID(context.Background(), "run-1")
	if got.Status != RunStatusRolledBack {
		t.Fatalf("回滚后应 rolled_back, 实际 %s", got.Status)
	}
	if notifier.calls != 1 {
		t.Fatalf("回滚应通知管理员一次, 实际 %d", notifier.calls)
	}
}

// 到期且 P95 退化 → 自动回滚。
func TestSIWatchdog_P95RegressionRollback(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	uc, store, metrics, applier, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500})}, nil)
	metrics.snap = &MetricsSnapshot{ErrorRate: 0.10, P95MS: 700} // 700 > 500×1.3=650

	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if applier.rollbackCalls != 1 {
		t.Fatalf("P95 退化应回滚, 实际 %d", applier.rollbackCalls)
	}
	if got, _ := store.GetByID(context.Background(), "run-1"); got.Status != RunStatusRolledBack {
		t.Fatalf("应 rolled_back, 实际 %s", got.Status)
	}
}

// 零基线地板：baseline=0 时退化判定走低量程绝对地板，避免无流量基线误判。
func TestSIWatchdog_ZeroBaselineFloor(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	// after=0.05 低于 10% 地板 → 不回滚
	uc, store, metrics, applier, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, &MetricsSnapshot{ErrorRate: 0, P95MS: 0})}, nil)
	metrics.snap = &MetricsSnapshot{ErrorRate: 0.05, P95MS: 800}
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if applier.rollbackCalls != 0 {
		t.Fatal("低于地板不应回滚")
	}
	if got, _ := store.GetByID(context.Background(), "run-1"); got.Status != RunStatusClosed {
		t.Fatalf("应 closed, 实际 %s", got.Status)
	}

	// after=0.15 高于 10% 地板 → 回滚
	uc2, store2, metrics2, applier2, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-2", &past, &MetricsSnapshot{ErrorRate: 0, P95MS: 0})}, nil)
	metrics2.snap = &MetricsSnapshot{ErrorRate: 0.15, P95MS: 800}
	if err := uc2.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if applier2.rollbackCalls != 1 {
		t.Fatal("高于地板应回滚")
	}
	if got, _ := store2.GetByID(context.Background(), "run-2"); got.Status != RunStatusRolledBack {
		t.Fatalf("应 rolled_back, 实际 %s", got.Status)
	}
}

// 回滚失败：run 停留 observing 待下 tick 重试，ScanOnce 不报错。
func TestSIWatchdog_RollbackFailureRetried(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	uc, store, metrics, applier, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500})}, nil)
	metrics.snap = &MetricsSnapshot{ErrorRate: 0.30, P95MS: 500}
	applier.err = errors.New("git revert conflict")

	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("回滚失败应被吸收: %v", err)
	}
	got, _ := store.GetByID(context.Background(), "run-1")
	if got.Status != RunStatusObserving {
		t.Fatalf("回滚失败应停留 observing 待重试, 实际 %s", got.Status)
	}
}

// 指标采集失败：run 跳过不评估，整 tick 不报错。
func TestSIWatchdog_MetricsErrorAbsorbed(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	uc, store, metrics, applier, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500})}, nil)
	metrics.err = errors.New("db down")

	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("指标错误应被吸收: %v", err)
	}
	got, _ := store.GetByID(context.Background(), "run-1")
	if got.Status != RunStatusObserving {
		t.Fatalf("指标失败应停留 observing, 实际 %s", got.Status)
	}
	if applier.rollbackCalls != 0 {
		t.Fatal("指标失败不应回滚")
	}
}

// 可配置因子生效。
func TestSIWatchdog_ConfigurableFactors(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	uc, store, metrics, applier, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500})},
		func(d *SelfImprovementWatchdogDeps) { d.ErrorRateFactor = 3.0 })
	metrics.snap = &MetricsSnapshot{ErrorRate: 0.20, P95MS: 500} // 0.20 < 0.10×3.0
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if applier.rollbackCalls != 0 {
		t.Fatal("放宽因子后不应回滚")
	}
	if got, _ := store.GetByID(context.Background(), "run-1"); got.Status != RunStatusClosed {
		t.Fatalf("应 closed, 实际 %s", got.Status)
	}
}

func TestSIWatchdog_ConstructorGuards(t *testing.T) {
	if _, err := NewSelfImprovementWatchdogUsecase(SelfImprovementWatchdogDeps{}); err == nil {
		t.Fatal("缺依赖应报错")
	}
	store := &siRunStore{}
	if _, err := NewSelfImprovementWatchdogUsecase(SelfImprovementWatchdogDeps{
		RunReader: store, RunWriter: store, Metrics: &siWatchMetricsFake{}, Lg: loggateway.NewNoop(),
	}); err == nil {
		t.Fatal("缺 Applier 应报错")
	}
	// Notifier 可 nil（降级仅日志）。
	if _, err := NewSelfImprovementWatchdogUsecase(SelfImprovementWatchdogDeps{
		RunReader: store, RunWriter: store, Metrics: &siWatchMetricsFake{}, Applier: &siWatchApplierFake{},
		Lg: loggateway.NewNoop(),
	}); err != nil {
		t.Fatalf("Notifier 可 nil 降级: %v", err)
	}
}

// CAS 冲突（他入口已推进）→ 静默跳过。
func TestSIWatchdog_CASConflictSilent(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	uc, store, metrics, _, _ := siWatchFixture(t,
		[]SelfImprovementRun{siWatchRun("run-1", &past, &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500})}, nil)
	metrics.snap = &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500}
	// 外部把 run 推到 closed：Watchdog 的 close CAS 应冲突并静默。
	got, _ := store.GetByID(context.Background(), "run-1")
	got.Status = RunStatusClosed
	if err := store.Update(context.Background(), got, RunStatusObserving); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("CAS 冲突应静默: %v", err)
	}
}
