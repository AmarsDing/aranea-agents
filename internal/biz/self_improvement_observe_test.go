package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── stubs ────────────────────────────────────────────────────────────────────

// siStubSuggestionReader records the ListByTarget filter and replays a fixed
// pending list plus (optionally) the orchestrator writer's newly created rows
// — mirroring production where both hit the same suggestions table.
type siStubSuggestionReader struct {
	pending []UnifiedEvolutionSuggestion
	created *[]UnifiedEvolutionSuggestion // 通常指向 orchStubWriter.created

	gotTargetType string
	gotTargetID   string
	gotStatus     string
}

func (r *siStubSuggestionReader) GetByID(_ context.Context, id string) (*UnifiedEvolutionSuggestion, error) {
	for i := range r.pending {
		if r.pending[i].ID == id {
			return &r.pending[i], nil
		}
	}
	return nil, apierror.NotFound("EVO", "suggestion not found")
}
func (r *siStubSuggestionReader) ListByTarget(_ context.Context, targetType, targetID, status string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	r.gotTargetType, r.gotTargetID, r.gotStatus = targetType, targetID, status
	all := append([]UnifiedEvolutionSuggestion{}, r.pending...)
	if r.created != nil {
		all = append(all, *r.created...)
	}
	var out []UnifiedEvolutionSuggestion
	for _, s := range all {
		if string(s.TargetType) == targetType && (targetID == "" || s.TargetID == targetID) && (status == "" || s.Status == status) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (r *siStubSuggestionReader) CountByTarget(context.Context, string, string, string) (int, error) {
	return 0, nil
}
func (r *siStubSuggestionReader) ListByTargetAndAction(context.Context, string, string, string, string, int, int) ([]UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (r *siStubSuggestionReader) CountByTargetAndAction(context.Context, string, string, string, string) (int, error) {
	return 0, nil
}

// siStubRunStore is an in-memory SelfImprovementRunReader+Writer.
type siStubRunStore struct {
	byID         map[string]*SelfImprovementRun
	bySuggestion map[string]*SelfImprovementRun
	createErr    error
	createCalls  int
}

func newSIStubRunStore() *siStubRunStore {
	return &siStubRunStore{byID: map[string]*SelfImprovementRun{}, bySuggestion: map[string]*SelfImprovementRun{}}
}

func (s *siStubRunStore) GetByID(_ context.Context, id string) (*SelfImprovementRun, error) {
	return s.byID[id], nil
}
func (s *siStubRunStore) GetBySuggestionID(_ context.Context, suggestionID string) (*SelfImprovementRun, error) {
	return s.bySuggestion[suggestionID], nil
}
func (s *siStubRunStore) List(context.Context, RunFilter) ([]SelfImprovementRun, error) {
	return nil, nil
}
func (s *siStubRunStore) Count(context.Context, RunFilter) (int, error) {
	return 0, nil
}
func (s *siStubRunStore) ListTerminalPendingOutcome(context.Context, int) ([]SelfImprovementRun, error) {
	return nil, nil
}
func (s *siStubRunStore) Create(_ context.Context, run *SelfImprovementRun) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.createCalls++
	cp := *run
	s.byID[run.ID] = &cp
	s.bySuggestion[run.SuggestionID] = &cp
	return nil
}
func (s *siStubRunStore) Update(context.Context, *SelfImprovementRun, SelfImprovementRunStatus) error {
	return nil
}
func (s *siStubRunStore) RecordAttempt(context.Context, string) error { return nil }

// ── tests ────────────────────────────────────────────────────────────────────

func newPlatformPendingSuggestion(id, source string) UnifiedEvolutionSuggestion {
	return UnifiedEvolutionSuggestion{
		ID:            id,
		TargetType:    EvolutionTargetPlatform,
		TargetID:      source + "/si:abc",
		ActionType:    EvolutionActionPatchCode,
		TriggerSource: source,
		Status:        string(UnifiedEvolutionStatePending),
		CreatedAt:     time.Now().UTC(),
	}
}

// 触发器产出 platform 建议 → ScanOnce 为其建 run(status=detected)。
func TestSIObserve_MaterializesRunForPendingPlatformSuggestion(t *testing.T) {
	trigger := &stubTrigger{
		targetType: EvolutionTargetPlatform,
		actionType: EvolutionActionPatchCode,
		source:     TriggerSourceErrorCluster,
		suggestions: []UnifiedEvolutionSuggestion{
			newPlatformPendingSuggestion("sug-p1", TriggerSourceErrorCluster),
		},
	}
	check := &orchStubCheckReader{}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	orch.RegisterTrigger(trigger)

	suggestions := &siStubSuggestionReader{created: &writer.created}
	runs := newSIStubRunStore()
	uc := NewSelfImprovementObserveUsecase(orch, suggestions, runs, runs, loggateway.NewNoop())

	created, err := uc.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	// 编排器落库的建议与 run 关联：run.SuggestionID 指向新建建议。
	if len(writer.created) != 1 {
		t.Fatalf("orchestrator created = %d, want 1", len(writer.created))
	}
	sugID := writer.created[0].ID
	run := runs.bySuggestion[sugID]
	if run == nil {
		t.Fatalf("no run for suggestion %q", sugID)
	}
	if run.Status != RunStatusDetected {
		t.Errorf("run.Status = %q, want detected", run.Status)
	}
	if run.TriggerSource != TriggerSourceErrorCluster {
		t.Errorf("run.TriggerSource = %q", run.TriggerSource)
	}
	if run.ID == "" || run.CreatedAt.IsZero() {
		t.Errorf("run 缺 ID/CreatedAt: %+v", run)
	}
	// 建议列表查询必须按 platform 过滤（ wildcard targetID ）。
	if suggestions.gotTargetType != string(EvolutionTargetPlatform) || suggestions.gotTargetID != "" || suggestions.gotStatus != string(UnifiedEvolutionStatePending) {
		t.Errorf("ListByTarget filter = (%q,%q,%q)", suggestions.gotTargetType, suggestions.gotTargetID, suggestions.gotStatus)
	}
	// 扫描入口 targetID 常量（HasPendingForTarget 用到）。
	if trigger.checkedForID != SIPlatformScanTargetID {
		t.Errorf("trigger checked targetID = %q, want %q", trigger.checkedForID, SIPlatformScanTargetID)
	}
}

// 已有 run 的建议不重复建（幂等）。
func TestSIObserve_SkipsSuggestionWithExistingRun(t *testing.T) {
	sug := newPlatformPendingSuggestion("sug-p2", TriggerSourcePerfBottleneck)
	suggestions := &siStubSuggestionReader{pending: []UnifiedEvolutionSuggestion{sug}}
	runs := newSIStubRunStore()
	_ = runs.Create(context.Background(), &SelfImprovementRun{
		ID: "run-existing", SuggestionID: "sug-p2", Status: RunStatusDiagnosing,
		TriggerSource: TriggerSourcePerfBottleneck, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	before := runs.createCalls

	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubQueryReader{}, &orchStubWriter{}, loggateway.NewNoop())
	uc := NewSelfImprovementObserveUsecase(orch, suggestions, runs, runs, loggateway.NewNoop())

	created, err := uc.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0（已有 run）", created)
	}
	if runs.createCalls != before {
		t.Errorf("Create 被调用 %d 次，期望不变", runs.createCalls-before)
	}
}

// 非 platform 的 pending 建议不参与（过滤在查询层，双保险在类型断言）。
func TestSIObserve_IgnoresNonPlatformSuggestions(t *testing.T) {
	suggestions := &siStubSuggestionReader{pending: []UnifiedEvolutionSuggestion{
		{ID: "sug-s1", TargetType: EvolutionTargetSkill, TargetID: "skill-1", Status: string(UnifiedEvolutionStatePending)},
	}}
	runs := newSIStubRunStore()
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubQueryReader{}, &orchStubWriter{}, loggateway.NewNoop())
	uc := NewSelfImprovementObserveUsecase(orch, suggestions, runs, runs, loggateway.NewNoop())

	created, err := uc.ScanOnce(context.Background())
	if err != nil || created != 0 {
		t.Fatalf("created=%d err=%v, want 0/nil", created, err)
	}
	if runs.createCalls != 0 {
		t.Errorf("非 platform 建议不应建 run")
	}
}

// 扫描（CheckAndCreate）失败不阻断 run 物化：存量 pending 仍处理。
func TestSIObserve_ScanErrorStillMaterializes(t *testing.T) {
	trigger := &stubTrigger{
		targetType: EvolutionTargetPlatform,
		actionType: EvolutionActionPatchCode,
		source:     TriggerSourceErrorCluster,
		suggestions: []UnifiedEvolutionSuggestion{
			newPlatformPendingSuggestion("sug-x", TriggerSourceErrorCluster),
		},
	}
	writer := &orchStubWriter{createErr: errors.New("db down")}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	orch.RegisterTrigger(trigger)

	suggestions := &siStubSuggestionReader{pending: []UnifiedEvolutionSuggestion{
		newPlatformPendingSuggestion("sug-old", TriggerSourceEvalRegression),
	}}
	runs := newSIStubRunStore()
	uc := NewSelfImprovementObserveUsecase(orch, suggestions, runs, runs, loggateway.NewNoop())

	created, err := uc.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("ScanOnce 应容忍扫描失败: %v", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1（存量 pending 仍建 run）", created)
	}
	if runs.bySuggestion["sug-old"] == nil {
		t.Error("sug-old 应有 run")
	}
}

// run 创建全部失败：返回 0 + error（不静默吞错）。
func TestSIObserve_RunCreateErrorReturnsError(t *testing.T) {
	suggestions := &siStubSuggestionReader{pending: []UnifiedEvolutionSuggestion{
		newPlatformPendingSuggestion("sug-a", TriggerSourceErrorCluster),
		newPlatformPendingSuggestion("sug-b", TriggerSourceTestFailure),
	}}
	runs := newSIStubRunStore()
	runs.createErr = apierror.Internal("SI", "insert failed")
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubQueryReader{}, &orchStubWriter{}, loggateway.NewNoop())
	uc := NewSelfImprovementObserveUsecase(orch, suggestions, runs, runs, loggateway.NewNoop())

	created, err := uc.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("全部建 run 失败应返回 error")
	}
	if created != 0 {
		t.Errorf("created = %d, want 0", created)
	}
}

// nil 依赖安全：不 panic，返回 0。
func TestSIObserve_NilDeps(t *testing.T) {
	uc := NewSelfImprovementObserveUsecase(nil, nil, nil, nil, loggateway.NewNoop())
	created, err := uc.ScanOnce(context.Background())
	if err != nil || created != 0 {
		t.Errorf("nil deps: created=%d err=%v, want 0/nil", created, err)
	}
}
