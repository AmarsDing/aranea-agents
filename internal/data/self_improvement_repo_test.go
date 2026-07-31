package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func setupSelfImprovementRepo(t *testing.T) (*SelfImprovementRunRepo, context.Context) {
	t.Helper()
	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, loggateway.NewNoop())
	return NewSelfImprovementRunRepo(d, loggateway.NewNoop()), context.Background()
}

func newTestRun(id, suggestionID string, status biz.SelfImprovementRunStatus) *biz.SelfImprovementRun {
	return &biz.SelfImprovementRun{
		ID:            id,
		SuggestionID:  suggestionID,
		Status:        status,
		TriggerSource: biz.TriggerSourceErrorCluster,
		Attempts:      0,
		CreatedAt:     time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt:     time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestSelfImprovementRunRepo_CreateAndGet(t *testing.T) {
	repo, ctx := setupSelfImprovementRepo(t)

	run := newTestRun("run-1", "sug-1", biz.RunStatusDetected)
	run.PatchKind = biz.PatchKindCode
	run.Diff = "--- a/x.go\n+++ b/x.go\n"
	run.DiffStats = biz.DiffStats{Files: 1, Additions: 3, Deletions: 1}
	run.Diagnosis = &biz.Diagnosis{RootCause: "nil deref", AffectedFiles: []string{"internal/biz/x.go"}, ImpactScope: "local", FixStrategy: "guard", Confidence: 0.8}
	run.VerificationReport = []biz.SandboxGateResult{{Gate: biz.SandboxGateBuild, Passed: true, Output: "ok", DurationMS: 120}}
	run.CriticReport = &biz.CriticReport{IsSafe: true, RiskLevel: "low", Concerns: []string{}, Suggestion: ""}
	run.Governance = &biz.GovernanceDecision{RiskLevel: biz.RiskLevelLow, Channel: "auto", RuleHits: []string{"R1"}}

	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID 返回 nil")
	}
	if got.Status != biz.RunStatusDetected || got.TriggerSource != biz.TriggerSourceErrorCluster {
		t.Errorf("状态/触发源不符: %+v", got)
	}
	if got.DiffStats.Files != 1 || got.DiffStats.Additions != 3 {
		t.Errorf("DiffStats 不符: %+v", got.DiffStats)
	}
	if got.Diagnosis == nil || got.Diagnosis.RootCause != "nil deref" || len(got.Diagnosis.AffectedFiles) != 1 {
		t.Errorf("Diagnosis 不符: %+v", got.Diagnosis)
	}
	if len(got.VerificationReport) != 1 || got.VerificationReport[0].Gate != biz.SandboxGateBuild || !got.VerificationReport[0].Passed {
		t.Errorf("VerificationReport 不符: %+v", got.VerificationReport)
	}
	if got.CriticReport == nil || !got.CriticReport.IsSafe {
		t.Errorf("CriticReport 不符: %+v", got.CriticReport)
	}
	if got.Governance == nil || got.Governance.Channel != "auto" {
		t.Errorf("Governance 不符: %+v", got.Governance)
	}

	bySug, err := repo.GetBySuggestionID(ctx, "sug-1")
	if err != nil || bySug == nil || bySug.ID != "run-1" {
		t.Fatalf("GetBySuggestionID: %v, %+v", err, bySug)
	}
}

func TestSelfImprovementRunRepo_GetAbsentReturnsNilNil(t *testing.T) {
	repo, ctx := setupSelfImprovementRepo(t)
	got, err := repo.GetByID(ctx, "absent")
	if err != nil || got != nil {
		t.Fatalf("期望 (nil,nil)，实际 (%v,%v)", got, err)
	}
}

func TestSelfImprovementRunRepo_UpdateCAS(t *testing.T) {
	repo, ctx := setupSelfImprovementRepo(t)
	if err := repo.Create(ctx, newTestRun("run-2", "sug-2", biz.RunStatusDetected)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	run, _ := repo.GetByID(ctx, "run-2")
	run.Status = biz.RunStatusDiagnosing
	run.Diagnosis = &biz.Diagnosis{RootCause: "rc", Confidence: 0.9}
	if err := repo.Update(ctx, run, biz.RunStatusDetected); err != nil {
		t.Fatalf("Update CAS: %v", err)
	}
	got, _ := repo.GetByID(ctx, "run-2")
	if got.Status != biz.RunStatusDiagnosing || got.Diagnosis == nil || got.Diagnosis.Confidence != 0.9 {
		t.Fatalf("Update 未生效: %+v", got)
	}

	// CAS 冲突：from 状态不匹配 → CodeConflict
	run.Status = biz.RunStatusPatching
	err := repo.Update(ctx, run, biz.RunStatusDetected)
	if err == nil {
		t.Fatal("期望 CAS 冲突错误")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("期望 CodeConflict，实际: %v", err)
	}
}

func TestSelfImprovementRunRepo_RecordAttempt(t *testing.T) {
	repo, ctx := setupSelfImprovementRepo(t)
	if err := repo.Create(ctx, newTestRun("run-3", "sug-3", biz.RunStatusPatching)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := repo.RecordAttempt(ctx, "run-3"); err != nil {
			t.Fatalf("RecordAttempt: %v", err)
		}
	}
	got, _ := repo.GetByID(ctx, "run-3")
	if got.Attempts != 2 {
		t.Fatalf("Attempts = %d，期望 2", got.Attempts)
	}
}

func TestSelfImprovementRunRepo_ListAndObservingDue(t *testing.T) {
	repo, ctx := setupSelfImprovementRepo(t)
	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)

	r1 := newTestRun("run-a", "sug-a", biz.RunStatusObserving)
	r1.ObserveUntil = &past
	r2 := newTestRun("run-b", "sug-b", biz.RunStatusObserving)
	r2.ObserveUntil = &future
	r2.TriggerSource = biz.TriggerSourcePerfBottleneck
	r3 := newTestRun("run-c", "sug-c", biz.RunStatusDetected)
	for _, r := range []*biz.SelfImprovementRun{r1, r2, r3} {
		if err := repo.Create(ctx, r); err != nil {
			t.Fatalf("Create %s: %v", r.ID, err)
		}
	}

	observing, err := repo.List(ctx, biz.RunFilter{Status: biz.RunStatusObserving, Limit: 10})
	if err != nil || len(observing) != 2 {
		t.Fatalf("List observing: %v, len=%d", err, len(observing))
	}
	bySource, err := repo.List(ctx, biz.RunFilter{TriggerSource: biz.TriggerSourcePerfBottleneck, Limit: 10})
	if err != nil || len(bySource) != 1 || bySource[0].ID != "run-b" {
		t.Fatalf("List by source: %v, %+v", err, bySource)
	}
	paged, err := repo.List(ctx, biz.RunFilter{Limit: 2, Offset: 0})
	if err != nil || len(paged) != 2 {
		t.Fatalf("List 分页: %v, len=%d", err, len(paged))
	}

	due, err := repo.ListObservingDue(ctx, time.Now().UTC())
	if err != nil || len(due) != 1 || due[0].ID != "run-a" {
		t.Fatalf("ListObservingDue: %v, %+v", err, due)
	}
}

func TestPatchOutcomeRepo_CreateAndList(t *testing.T) {
	repo, ctx := setupSelfImprovementRepo(t)
	if err := repo.Create(ctx, newTestRun("run-o", "sug-o", biz.RunStatusClosed)); err != nil {
		t.Fatalf("Create run: %v", err)
	}
	outcome := &biz.PatchOutcome{
		ID:            "po-1",
		RunID:         "run-o",
		SuggestionID:  "sug-o",
		Verdict:       biz.VerdictEffective,
		MetricsBefore: &biz.MetricsSnapshot{ErrorRate: 0.05, P95MS: 800, AlertCount: 1},
		MetricsAfter:  &biz.MetricsSnapshot{ErrorRate: 0.01, P95MS: 600, AlertCount: 0},
		CreatedAt:     time.Now().UTC(),
	}
	if err := repo.CreateOutcome(ctx, outcome); err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}
	list, err := repo.ListOutcomesByRun(ctx, "run-o")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListOutcomesByRun: %v, len=%d", err, len(list))
	}
	if list[0].Verdict != biz.VerdictEffective || list[0].MetricsAfter == nil || list[0].MetricsAfter.ErrorRate != 0.01 {
		t.Fatalf("Outcome 内容不符: %+v", list[0])
	}
}

func TestSelfImprovementRunRepo_ListTerminalPendingOutcome(t *testing.T) {
	repo, ctx := setupSelfImprovementRepo(t)
	// terminal 且无 outcome → 命中
	if err := repo.Create(ctx, newTestRun("run-t1", "sug-t1", biz.RunStatusClosed)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// terminal 且已有 outcome → 排除
	if err := repo.Create(ctx, newTestRun("run-t2", "sug-t2", biz.RunStatusRolledBack)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 非终态 → 排除
	if err := repo.Create(ctx, newTestRun("run-t3", "sug-t3", biz.RunStatusObserving)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.CreateOutcome(ctx, &biz.PatchOutcome{ID: "po-t2", RunID: "run-t2", SuggestionID: "sug-t2", Verdict: biz.VerdictRegressed, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}

	pending, err := repo.ListTerminalPendingOutcome(ctx, 10)
	if err != nil {
		t.Fatalf("ListTerminalPendingOutcome: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "run-t1" {
		t.Fatalf("pending = %+v, want only run-t1", pending)
	}

	// 全部归因后 → 空
	if err := repo.CreateOutcome(ctx, &biz.PatchOutcome{ID: "po-t1", RunID: "run-t1", SuggestionID: "sug-t1", Verdict: biz.VerdictEffective, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}
	pending, err = repo.ListTerminalPendingOutcome(ctx, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("归因后 pending = %v, len=%d, want 0", err, len(pending))
	}
}

func TestSelfImprovementRunRepo_ListRecentOutcomesByTrigger(t *testing.T) {
	repo, ctx := setupSelfImprovementRepo(t)
	mk := func(id, sug string, src string) {
		r := newTestRun(id, sug, biz.RunStatusClosed)
		r.TriggerSource = src
		if err := repo.Create(ctx, r); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	mk("run-s1", "sug-s1", biz.TriggerSourceErrorCluster)
	mk("run-s2", "sug-s2", biz.TriggerSourceErrorCluster)
	mk("run-s3", "sug-s3", biz.TriggerSourcePerfBottleneck)
	// 先写 s1 的 outcome，再写 s2 的（时间递增，后者更新）。
	if err := repo.CreateOutcome(ctx, &biz.PatchOutcome{ID: "po-s1", RunID: "run-s1", SuggestionID: "sug-s1", Verdict: biz.VerdictNeutral, CreatedAt: time.Now().UTC().Add(-time.Minute)}); err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}
	if err := repo.CreateOutcome(ctx, &biz.PatchOutcome{ID: "po-s2", RunID: "run-s2", SuggestionID: "sug-s2", Verdict: biz.VerdictRegressed, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}
	if err := repo.CreateOutcome(ctx, &biz.PatchOutcome{ID: "po-s3", RunID: "run-s3", SuggestionID: "sug-s3", Verdict: biz.VerdictEffective, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}

	recent, err := repo.ListRecentOutcomesByTrigger(ctx, biz.TriggerSourceErrorCluster, 10)
	if err != nil {
		t.Fatalf("ListRecentOutcomesByTrigger: %v", err)
	}
	if len(recent) != 2 || recent[0].ID != "po-s2" || recent[1].ID != "po-s1" {
		t.Fatalf("recent = %+v, want [po-s2 po-s1]（newest first）", recent)
	}
	// 无该触发源的 outcome → nil
	none, err := repo.ListRecentOutcomesByTrigger(ctx, biz.TriggerSourceTestFailure, 10)
	if err != nil || len(none) != 0 {
		t.Fatalf("未知触发源 = %v, %+v, want empty", err, none)
	}
}
