package biz

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── outcome fakes ────────────────────────────────────────────────────────────

type siOutcomeRunStore struct {
	siRunStore
	pending []SelfImprovementRun
}

func (s *siOutcomeRunStore) ListTerminalPendingOutcome(_ context.Context, _ int) ([]SelfImprovementRun, error) {
	out := make([]SelfImprovementRun, len(s.pending))
	copy(out, s.pending)
	return out, nil
}

type siOutcomeWriterFake struct {
	mu       sync.Mutex
	created  []PatchOutcome
	byTrig   []PatchOutcome
	createEr error
}

func (f *siOutcomeWriterFake) CreateOutcome(_ context.Context, o *PatchOutcome) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createEr != nil {
		return f.createEr
	}
	f.created = append(f.created, *o)
	return nil
}
func (f *siOutcomeWriterFake) ListOutcomesByRun(context.Context, string) ([]PatchOutcome, error) {
	return nil, nil
}
func (f *siOutcomeWriterFake) ListRecentOutcomesByTrigger(_ context.Context, _ string, _ int) ([]PatchOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PatchOutcome, len(f.byTrig))
	copy(out, f.byTrig)
	return out, nil
}

type siPatternSinkFake struct {
	mu   sync.Mutex
	recs []SINegativePatternRecord
	err  error
}

func (s *siPatternSinkFake) RecordNegativePattern(_ context.Context, rec SINegativePatternRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.recs = append(s.recs, rec)
	return nil
}

type siFeedbackFake struct {
	mu     sync.Mutex
	calls  []string
	factor float64
}

func (f *siFeedbackFake) EscalateTriggerCooldown(_ context.Context, triggerSource string, factor float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, triggerSource)
	f.factor = factor
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func siOutcomeRun(id string, status SelfImprovementRunStatus, trigger string) SelfImprovementRun {
	return SelfImprovementRun{
		ID: id, SuggestionID: "sug-" + id, Status: status,
		TriggerSource: trigger,
		Diff:          "--- a/internal/biz/foo.go\n+++ b/internal/biz/foo.go\n@@ -1 +1 @@\n-x\n+y\n",
		ClosedReason:  "auto rollback: error_rate regression",
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now(),
	}
}

func siOutcomeFixture(t *testing.T, pending []SelfImprovementRun, mutate func(*SelfImprovementOutcomeDeps)) (*SelfImprovementOutcomeUsecase, *siOutcomeWriterFake, *siPatternSinkFake, *siFeedbackFake) {
	t.Helper()
	store := &siOutcomeRunStore{pending: pending}
	outcomes := &siOutcomeWriterFake{}
	patterns := &siPatternSinkFake{}
	feedback := &siFeedbackFake{}
	deps := SelfImprovementOutcomeDeps{
		RunReader: store, Outcomes: outcomes,
		Patterns: patterns, Feedback: feedback,
		Lg: loggateway.NewNoop(),
	}
	if mutate != nil {
		mutate(&deps)
	}
	uc, err := NewSelfImprovementOutcomeUsecase(deps)
	if err != nil {
		t.Fatalf("NewSelfImprovementOutcomeUsecase: %v", err)
	}
	return uc, outcomes, patterns, feedback
}

// ── tests ────────────────────────────────────────────────────────────────────

// verdict 映射（D8）：closed→effective / rolled_back→regressed / 其余终态→neutral。
// 2026-08-19：closed 但 record_only（低置信度仅记录、补丁未应用）→ neutral，
// 否则有效率指标被从未生效的补丁虚增。
func TestSIOutcome_VerdictMapping(t *testing.T) {
	recordOnlyRun := siOutcomeRun("run-recordonly", RunStatusClosed, TriggerSourcePerfBottleneck)
	recordOnlyRun.ClosedReason = "record_only: confidence 0.35 < 0.50"
	pending := []SelfImprovementRun{
		siOutcomeRun("run-closed", RunStatusClosed, TriggerSourceErrorCluster),
		siOutcomeRun("run-rolled", RunStatusRolledBack, TriggerSourcePerfBottleneck),
		siOutcomeRun("run-vfailed", RunStatusVerifyFailed, TriggerSourceEvalRegression),
		siOutcomeRun("run-rejected", RunStatusRejected, TriggerSourceTestFailure),
		siOutcomeRun("run-failed", RunStatusFailed, TriggerSourceErrorCluster),
		recordOnlyRun,
	}
	uc, outcomes, patterns, _ := siOutcomeFixture(t, pending, nil)
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if len(outcomes.created) != 6 {
		t.Fatalf("应为 6 个终态 run 各建一条 outcome, 实际 %d", len(outcomes.created))
	}
	want := map[string]SelfImprovementVerdict{
		"run-closed": VerdictEffective, "run-rolled": VerdictRegressed,
		"run-vfailed": VerdictNeutral, "run-rejected": VerdictNeutral, "run-failed": VerdictNeutral,
		"run-recordonly": VerdictNeutral,
	}
	for _, o := range outcomes.created {
		if o.Verdict != want[o.RunID] {
			t.Errorf("run %s verdict = %s, want %s", o.RunID, o.Verdict, want[o.RunID])
		}
	}
	// 仅 regressed 写负面样本。
	if len(patterns.recs) != 1 || patterns.recs[0].RunID != "run-rolled" {
		t.Fatalf("仅 rolled_back 应写 KB 负面样本, 实际 %+v", patterns.recs)
	}
	// regressed 的 outcome 带 pattern_hash 与 rollback_reason。
	for _, o := range outcomes.created {
		if o.RunID == "run-rolled" {
			if o.PatternHash == "" || o.RollbackReason == "" {
				t.Errorf("regressed outcome 缺 pattern_hash/rollback_reason: %+v", o)
			}
		}
	}
}

// Watchdog 写入的 metadata 快照进入 outcome 的 metrics_before/after。
func TestSIOutcome_MetricsFromMetadata(t *testing.T) {
	run := siOutcomeRun("run-1", RunStatusRolledBack, TriggerSourceErrorCluster)
	meta, _ := json.Marshal(map[string]any{
		siMetaObserveBaseline: &MetricsSnapshot{ErrorRate: 0.10, P95MS: 500},
		siMetaObserveAfter:    &MetricsSnapshot{ErrorRate: 0.30, P95MS: 900},
	})
	run.Metadata = meta
	uc, outcomes, _, _ := siOutcomeFixture(t, []SelfImprovementRun{run}, nil)
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	o := outcomes.created[0]
	if o.MetricsBefore == nil || o.MetricsBefore.ErrorRate != 0.10 {
		t.Fatalf("metrics_before 缺失: %+v", o.MetricsBefore)
	}
	if o.MetricsAfter == nil || o.MetricsAfter.ErrorRate != 0.30 {
		t.Fatalf("metrics_after 缺失: %+v", o.MetricsAfter)
	}
}

// 触发器自适应（D8）：同一 trigger_source 连续 3 次 neutral/regressed → 冷却 ×2。
func TestSIOutcome_TriggerCooldownEscalation(t *testing.T) {
	pending := []SelfImprovementRun{
		siOutcomeRun("run-1", RunStatusRolledBack, TriggerSourceErrorCluster),
	}
	uc, outcomes, _, feedback := siOutcomeFixture(t, pending, nil)
	// 前两次已为 regressed/neutral（newest first），本次 run-1 为第三次。
	outcomes.byTrig = []PatchOutcome{
		{RunID: "run-0b", Verdict: VerdictNeutral},
		{RunID: "run-0a", Verdict: VerdictRegressed},
	}
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if len(feedback.calls) != 1 || feedback.calls[0] != TriggerSourceErrorCluster {
		t.Fatalf("连续 3 次非 effective 应降频一次, 实际 %+v", feedback.calls)
	}
	if feedback.factor != 2.0 {
		t.Fatalf("降频因子应为 2, 实际 %v", feedback.factor)
	}
}

// 最近 3 次含 effective → 不降频。
func TestSIOutcome_NoEscalationWhenEffective(t *testing.T) {
	pending := []SelfImprovementRun{siOutcomeRun("run-1", RunStatusRolledBack, TriggerSourceErrorCluster)}
	uc, outcomes, _, feedback := siOutcomeFixture(t, pending, nil)
	outcomes.byTrig = []PatchOutcome{
		{RunID: "run-0b", Verdict: VerdictEffective},
		{RunID: "run-0a", Verdict: VerdictRegressed},
	}
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if len(feedback.calls) != 0 {
		t.Fatalf("含 effective 不应降频, 实际 %+v", feedback.calls)
	}
}

// 历史不足 3 次 → 不降频。
func TestSIOutcome_NoEscalationBelowThree(t *testing.T) {
	pending := []SelfImprovementRun{siOutcomeRun("run-1", RunStatusRolledBack, TriggerSourceErrorCluster)}
	uc, outcomes, _, feedback := siOutcomeFixture(t, pending, nil)
	outcomes.byTrig = []PatchOutcome{{RunID: "run-0a", Verdict: VerdictRegressed}}
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if len(feedback.calls) != 0 {
		t.Fatalf("样本不足 3 不应降频, 实际 %+v", feedback.calls)
	}
}

// CreateOutcome 失败：吸收并继续后续 run；不写 KB、不降频（未归因成功）。
func TestSIOutcome_CreateErrorAbsorbed(t *testing.T) {
	pending := []SelfImprovementRun{
		siOutcomeRun("run-1", RunStatusRolledBack, TriggerSourceErrorCluster),
		siOutcomeRun("run-2", RunStatusClosed, TriggerSourcePerfBottleneck),
	}
	uc, outcomes, patterns, feedback := siOutcomeFixture(t, pending, nil)
	outcomes.createEr = errors.New("db down")
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("单 run 失败应被吸收: %v", err)
	}
	if len(patterns.recs) != 0 || len(feedback.calls) != 0 {
		t.Fatalf("归因失败不应写 KB/降频: patterns=%d feedback=%d", len(patterns.recs), len(feedback.calls))
	}
}

// KB / 反馈端口 nil → 降级（仅 outcome 归因）。
func TestSIOutcome_OptionalDepsDegraded(t *testing.T) {
	pending := []SelfImprovementRun{siOutcomeRun("run-1", RunStatusRolledBack, TriggerSourceErrorCluster)}
	uc, outcomes, _, _ := siOutcomeFixture(t, pending, func(d *SelfImprovementOutcomeDeps) {
		d.Patterns = nil
		d.Feedback = nil
	})
	outcomes.byTrig = []PatchOutcome{
		{RunID: "run-0b", Verdict: VerdictNeutral},
		{RunID: "run-0a", Verdict: VerdictRegressed},
	}
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("nil 端口应降级: %v", err)
	}
	if len(outcomes.created) != 1 {
		t.Fatalf("outcome 仍应归因, 实际 %d", len(outcomes.created))
	}
}

func TestSIOutcome_ConstructorGuards(t *testing.T) {
	if _, err := NewSelfImprovementOutcomeUsecase(SelfImprovementOutcomeDeps{}); err == nil {
		t.Fatal("缺依赖应报错")
	}
	if _, err := NewSelfImprovementOutcomeUsecase(SelfImprovementOutcomeDeps{
		RunReader: &siOutcomeRunStore{}, Lg: loggateway.NewNoop(),
	}); err == nil {
		t.Fatal("缺 Outcomes 应报错")
	}
}

// 非终态 run 不应进入归因扫描（repo 过滤兜底外的双保险）。
func TestSIOutcome_NonTerminalSkipped(t *testing.T) {
	pending := []SelfImprovementRun{siOutcomeRun("run-1", RunStatusObserving, TriggerSourceErrorCluster)}
	uc, outcomes, _, _ := siOutcomeFixture(t, pending, nil)
	if err := uc.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if len(outcomes.created) != 0 {
		t.Fatalf("非终态不应归因, 实际 %d", len(outcomes.created))
	}
}
