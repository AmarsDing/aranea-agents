package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// siTestMeta decodes the suggestion metadata JSON for assertions.
func siTestMeta(t *testing.T, s UnifiedEvolutionSuggestion) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(s.Metadata, &m); err != nil {
		t.Fatalf("unmarshal suggestion metadata: %v", err)
	}
	return m
}

// ── AnnotateOrchestrationTrace rule chain ────────────────────────────────────

func TestAnnotateOrchestrationTrace_DoomLoop(t *testing.T) {
	tr := OrchestrationTrace{
		OrchestrationID: "o1",
		Status:          string(OrchestrationStatusCancelled),
		CancelReason:    string(CancelReasonDoomLoop),
	}
	a := AnnotateOrchestrationTrace(tr)
	if a == nil {
		t.Fatal("expected annotation for doom_loop cancel")
	}
	if a.Mode != MASTStepRepetition {
		t.Errorf("doom_loop should map to FM-1.3 step_repetition, got %q", a.Mode)
	}
	if a.Category != MASTCategorySpecification {
		t.Errorf("FM-1.3 category should be specification, got %q", a.Category)
	}
	if a.Confidence < 0.9 {
		t.Errorf("doom_loop annotation confidence should be >= 0.9, got %.2f", a.Confidence)
	}
}

func TestAnnotateOrchestrationTrace_RepeatedStepErrors(t *testing.T) {
	tr := OrchestrationTrace{
		OrchestrationID: "o2",
		Status:          string(OrchestrationStatusFailed),
		ErrorSteps:      map[string]int{"spirit.team.execute": 4, "spirit.planner.assess": 1},
	}
	a := AnnotateOrchestrationTrace(tr)
	if a == nil {
		t.Fatal("expected annotation for repeated step errors")
	}
	if a.Mode != MASTStepRepetition {
		t.Errorf("repeated step errors (>=3) should map to FM-1.3, got %q", a.Mode)
	}
}

func TestAnnotateOrchestrationTrace_Timeout(t *testing.T) {
	tr := OrchestrationTrace{
		OrchestrationID: "o3",
		Status:          string(OrchestrationStatusCancelled),
		CancelReason:    string(CancelReasonTimeout),
	}
	a := AnnotateOrchestrationTrace(tr)
	if a == nil {
		t.Fatal("expected annotation for timeout cancel")
	}
	if a.Mode != MASTUnawareTermination {
		t.Errorf("timeout should map to FM-1.5 unaware_of_termination, got %q", a.Mode)
	}
}

func TestAnnotateOrchestrationTrace_SetupFailure(t *testing.T) {
	tr := OrchestrationTrace{
		OrchestrationID: "o4",
		Status:          string(OrchestrationStatusFailed),
		TeamCount:       0,
	}
	a := AnnotateOrchestrationTrace(tr)
	if a == nil {
		t.Fatal("expected annotation for setup failure")
	}
	if a.Mode != MASTDisobeyTaskSpec {
		t.Errorf("setup failure (0 teams) should map to FM-1.1, got %q", a.Mode)
	}
}

func TestAnnotateOrchestrationTrace_PrematureTermination(t *testing.T) {
	tr := OrchestrationTrace{
		OrchestrationID: "o5",
		Status:          string(OrchestrationStatusFailed),
		Strategy:        string(StrategyCoordinator),
		TeamCount:       2,
		DurationMS:      5_000, // 5s — team strategy failing this fast = premature
	}
	a := AnnotateOrchestrationTrace(tr)
	if a == nil {
		t.Fatal("expected annotation for premature termination")
	}
	if a.Mode != MASTPrematureTermination {
		t.Errorf("fast team failure should map to FM-3.1 premature_termination, got %q", a.Mode)
	}
	if a.Category != MASTCategoryVerification {
		t.Errorf("FM-3.1 category should be verification, got %q", a.Category)
	}
}

func TestAnnotateOrchestrationTrace_UserCancelSkipped(t *testing.T) {
	for _, reason := range []string{string(CancelReasonUser), string(CancelReasonParent), ""} {
		tr := OrchestrationTrace{
			OrchestrationID: "o6",
			Status:          string(OrchestrationStatusCancelled),
			CancelReason:    reason,
		}
		if a := AnnotateOrchestrationTrace(tr); a != nil {
			t.Errorf("user/parent cancel (reason=%q) should not be annotated, got %q", reason, a.Mode)
		}
	}
}

func TestAnnotateOrchestrationTrace_GenericFailureFallback(t *testing.T) {
	tr := OrchestrationTrace{
		OrchestrationID: "o7",
		Status:          string(OrchestrationStatusFailed),
		Strategy:        string(StrategyCoordinator),
		TeamCount:       2,
		DurationMS:      120_000, // 2min — not premature
	}
	a := AnnotateOrchestrationTrace(tr)
	if a == nil {
		t.Fatal("expected fallback annotation for generic failure")
	}
	if a.Mode != MASTNoVerification {
		t.Errorf("generic failure fallback should map to FM-3.2, got %q", a.Mode)
	}
	if a.Confidence > 0.5 {
		t.Errorf("fallback annotation should be low confidence (<=0.5), got %.2f", a.Confidence)
	}
}

func TestAnnotateOrchestrationTrace_CompletedSkipped(t *testing.T) {
	tr := OrchestrationTrace{
		OrchestrationID: "o8",
		Status:          string(OrchestrationStatusCompleted),
	}
	if a := AnnotateOrchestrationTrace(tr); a != nil {
		t.Errorf("completed orchestration should not be annotated, got %q", a.Mode)
	}
}

// ── OrchestrationTraceTrigger ────────────────────────────────────────────────

type mockOrchestrationTraceReader struct {
	traces []OrchestrationTrace
	err    error
}

func (m *mockOrchestrationTraceReader) ListTerminalOrchestrationTraces(_ context.Context, _ time.Time, _ int) ([]OrchestrationTrace, error) {
	return m.traces, m.err
}

func TestOrchestrationTraceTrigger_Interfaces(t *testing.T) {
	tr := NewOrchestrationTraceTrigger(nil, 0, 0, loggateway.NewNoop())
	var _ EvolutionTrigger = tr
	if tr.TargetType() != EvolutionTargetPlatform {
		t.Errorf("target type = %q, want platform", tr.TargetType())
	}
	if tr.TriggerSource() != TriggerSourceOrchestrationTrace {
		t.Errorf("trigger source = %q, want %q", tr.TriggerSource(), TriggerSourceOrchestrationTrace)
	}
}

func TestOrchestrationTraceTrigger_NilReader(t *testing.T) {
	tr := NewOrchestrationTraceTrigger(nil, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "")
	if err != nil {
		t.Fatalf("nil reader should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("nil reader should return no suggestions, got %d", len(got))
	}
}

func TestOrchestrationTraceTrigger_Empty(t *testing.T) {
	tr := NewOrchestrationTraceTrigger(&mockOrchestrationTraceReader{}, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("no traces should return no suggestions, got %d", len(got))
	}
}

func TestOrchestrationTraceTrigger_ReaderError(t *testing.T) {
	tr := NewOrchestrationTraceTrigger(&mockOrchestrationTraceReader{err: errors.New("db down")}, 0, 0, loggateway.NewNoop())
	if _, err := tr.Check(context.Background(), ""); err == nil {
		t.Fatal("reader error should propagate")
	}
}

func TestOrchestrationTraceTrigger_ClusterDedup(t *testing.T) {
	reader := &mockOrchestrationTraceReader{traces: []OrchestrationTrace{
		{OrchestrationID: "a", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonDoomLoop)},
		{OrchestrationID: "b", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonDoomLoop)},
		{OrchestrationID: "c", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonTimeout)},
	}}
	tr := NewOrchestrationTraceTrigger(reader, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("2 clusters (FM-1.3 x2, FM-1.5 x1) should produce 2 suggestions, got %d", len(got))
	}
	byMode := map[string]UnifiedEvolutionSuggestion{}
	for _, s := range got {
		mode, _ := siTestMeta(t, s)["mast_mode"].(string)
		byMode[mode] = s
		if s.TriggerSource != TriggerSourceOrchestrationTrace {
			t.Errorf("trigger source = %q", s.TriggerSource)
		}
		if s.Status != string(UnifiedEvolutionStatePending) {
			t.Errorf("status = %q, want pending", s.Status)
		}
	}
	fm13, ok := byMode[string(MASTStepRepetition)]
	if !ok {
		t.Fatal("missing FM-1.3 cluster suggestion")
	}
	if n, _ := siTestMeta(t, fm13)["cluster_count"].(float64); int(n) != 2 {
		t.Errorf("FM-1.3 cluster_count = %v, want 2", n)
	}
	samples, _ := siTestMeta(t, fm13)["sample_orchestration_ids"].([]any)
	if len(samples) == 0 {
		t.Error("cluster should carry sample orchestration ids")
	}
}

func TestOrchestrationTraceTrigger_ActionTypeMapping(t *testing.T) {
	// FM-1.x (specification) → patch_prompt；FM-3.x (verification) → tune_config。
	reader := &mockOrchestrationTraceReader{traces: []OrchestrationTrace{
		{OrchestrationID: "spec", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonDoomLoop)},
		{OrchestrationID: "verif", Status: string(OrchestrationStatusFailed), Strategy: string(StrategyCoordinator), TeamCount: 2, DurationMS: 3_000},
	}}
	tr := NewOrchestrationTraceTrigger(reader, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	byMode := map[string]UnifiedEvolutionSuggestion{}
	for _, s := range got {
		mode, _ := siTestMeta(t, s)["mast_mode"].(string)
		byMode[mode] = s
	}
	if s, ok := byMode[string(MASTStepRepetition)]; !ok || s.ActionType != EvolutionActionPatchPrompt {
		t.Errorf("FM-1.3 should map to patch_prompt, got %q", s.ActionType)
	}
	if s, ok := byMode[string(MASTPrematureTermination)]; !ok || s.ActionType != EvolutionActionTuneConfig {
		t.Errorf("FM-3.1 should map to tune_config, got %q", s.ActionType)
	}
}

func TestOrchestrationTraceTrigger_SkipsUserCancels(t *testing.T) {
	reader := &mockOrchestrationTraceReader{traces: []OrchestrationTrace{
		{OrchestrationID: "u1", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonUser)},
		{OrchestrationID: "u2", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonParent)},
		{OrchestrationID: "ok", Status: string(OrchestrationStatusCompleted)},
	}}
	tr := NewOrchestrationTraceTrigger(reader, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("user/parent cancels and completed runs should produce no suggestions, got %d", len(got))
	}
}

func TestOrchestrationTraceTrigger_SignatureCarriesMode(t *testing.T) {
	reader := &mockOrchestrationTraceReader{traces: []OrchestrationTrace{
		{OrchestrationID: "x", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonDoomLoop)},
	}}
	tr := NewOrchestrationTraceTrigger(reader, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 suggestion, got %d", len(got))
	}
	sig, _ := siTestMeta(t, got[0])[EvoMetaTriggerSignature].(string)
	if sig == "" {
		t.Error("suggestion should carry trigger signature for dedup")
	}
	hash, _ := siTestMeta(t, got[0])[EvoMetaPatternHash].(string)
	if hash != sig {
		t.Error("pattern_hash should mirror trigger signature for pending dedup")
	}
}

// ── P3-2 反哺闭环集成 ────────────────────────────────────────────────────────

// 编排 bad case → MAST 聚类建议落库（pending 待人工评审）→ observe 物化 run。
// 验证 trigger → orchestrator.CheckAndCreate → ScanOnce 全链路：
// FM-1.3 集群 → patch_prompt 建议 + run；FM-3.1 集群 → tune_config 建议 + run。
func TestOrchestrationTraceTrigger_ObserveClosedLoop(t *testing.T) {
	reader := &mockOrchestrationTraceReader{traces: []OrchestrationTrace{
		{OrchestrationID: "bad-1", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonDoomLoop)},
		{OrchestrationID: "bad-2", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonDoomLoop)},
		{OrchestrationID: "bad-3", Status: string(OrchestrationStatusFailed), Strategy: string(StrategyCoordinator), TeamCount: 2, DurationMS: 2_000},
		// 用户取消不入闭环（非系统失败）。
		{OrchestrationID: "user-1", Status: string(OrchestrationStatusCancelled), CancelReason: string(CancelReasonUser)},
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, writer, loggateway.NewNoop())
	orch.RegisterTrigger(NewOrchestrationTraceTrigger(reader, 0, 0, loggateway.NewNoop()))

	suggestions := &siStubSuggestionReader{created: &writer.created}
	runs := newSIStubRunStore()
	uc := NewSelfImprovementObserveUsecase(orch, suggestions, runs, runs, loggateway.NewNoop())

	created, err := uc.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	// 2 个 MAST 集群（FM-1.3 x2、FM-3.1 x1）→ 2 建议 + 2 run。
	if len(writer.created) != 2 {
		t.Fatalf("orchestrator created = %d, want 2", len(writer.created))
	}
	if created != 2 {
		t.Fatalf("runs created = %d, want 2", created)
	}

	byMode := map[string]UnifiedEvolutionSuggestion{}
	for _, s := range writer.created {
		mode, _ := siTestMeta(t, s)["mast_mode"].(string)
		byMode[mode] = s
		// 建议必须 pending（先人工评审后应用），且以 platform 为目标。
		if s.Status != string(UnifiedEvolutionStatePending) {
			t.Errorf("suggestion status = %q, want pending", s.Status)
		}
		if s.TargetType != EvolutionTargetPlatform {
			t.Errorf("target type = %q, want platform", s.TargetType)
		}
		// 每个建议必须物化出 run，且 run 标注 trigger_source 供后续阶段路由。
		run := runs.bySuggestion[s.ID]
		if run == nil {
			t.Fatalf("no run materialized for suggestion %q (mode %s)", s.ID, mode)
		}
		if run.Status != RunStatusDetected {
			t.Errorf("run.Status = %q, want detected", run.Status)
		}
		if run.TriggerSource != TriggerSourceOrchestrationTrace {
			t.Errorf("run.TriggerSource = %q, want %q", run.TriggerSource, TriggerSourceOrchestrationTrace)
		}
	}
	// 反哺动作映射：规范类 → patch_prompt；验证类 → tune_config。
	if s, ok := byMode[string(MASTStepRepetition)]; !ok || s.ActionType != EvolutionActionPatchPrompt {
		t.Errorf("FM-1.3 should route to patch_prompt, got %q", s.ActionType)
	}
	if s, ok := byMode[string(MASTPrematureTermination)]; !ok || s.ActionType != EvolutionActionTuneConfig {
		t.Errorf("FM-3.1 should route to tune_config, got %q", s.ActionType)
	}
	// 高置信集群（doom_loop 0.95 / count=2）应升优先级。
	if s, ok := byMode[string(MASTStepRepetition)]; ok && s.Priority < 2 {
		t.Errorf("FM-1.3 cluster priority = %d, want >= 2", s.Priority)
	}
}
