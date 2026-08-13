package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ── Stub repos for SandboxRunner tests ────────────────────────────────────────

type stubSuggestionReader struct {
	suggestion *biz.SkillEvolutionSuggestion
	err        error
}

func (s *stubSuggestionReader) ListBySkill(_ context.Context, _ string, _ biz.EvolutionSuggestionStatus, _, _ int) ([]biz.SkillEvolutionSuggestion, error) {
	return nil, nil
}
func (s *stubSuggestionReader) GetByID(_ context.Context, _ string) (*biz.SkillEvolutionSuggestion, error) {
	return s.suggestion, s.err
}
func (s *stubSuggestionReader) ListPending(_ context.Context, _, _ int) ([]biz.SkillEvolutionSuggestion, error) {
	return nil, nil
}
func (s *stubSuggestionReader) GetLatestBySkill(_ context.Context, _ string) (*biz.SkillEvolutionSuggestion, error) {
	return nil, nil
}
func (s *stubSuggestionReader) CountBySkill(_ context.Context, _ string, _ biz.EvolutionSuggestionStatus) (int, error) {
	return 0, nil
}

type stubSuggestionWriter struct {
	lastLifecycleStatus biz.EvolutionLifecycleStatus
	lastSandboxPassed   bool
	lastSandboxResult   json.RawMessage
	err                 error
}

func (s *stubSuggestionWriter) Create(_ context.Context, _ biz.SkillEvolutionSuggestion) error {
	return s.err
}
func (s *stubSuggestionWriter) UpdateStatus(_ context.Context, _ string, _ biz.EvolutionSuggestionStatus, _, _ string) error {
	return s.err
}
func (s *stubSuggestionWriter) UpdateDraftBody(_ context.Context, _, _ string) error { return s.err }
func (s *stubSuggestionWriter) UpdateSandboxResult(_ context.Context, _ string, passed bool, result json.RawMessage) error {
	s.lastSandboxPassed = passed
	s.lastSandboxResult = result
	return s.err
}
func (s *stubSuggestionWriter) UpdateLifecycleStatus(_ context.Context, _ string, lifecycleStatus biz.EvolutionLifecycleStatus) error {
	s.lastLifecycleStatus = lifecycleStatus
	return s.err
}

// stubUnifiedEvolutionStore implements biz.UnifiedEvolutionStore backed by the
// legacy-view reader/writer stubs (A6). Only the methods exercised by the
// SandboxRunner and SkillCuratorService tests (GetByID, UpdateDraftBody,
// UpdateLifecycleStatus, UpdateSandboxResult) have working implementations.
type stubUnifiedEvolutionStore struct {
	*stubSuggestionReader
	*stubSuggestionWriter
}

// legacyToUnifiedView mirrors biz.unifiedToLegacySuggestion for seeding.
func legacyToUnifiedView(s *biz.SkillEvolutionSuggestion) *biz.UnifiedEvolutionSuggestion {
	if s == nil {
		return nil
	}
	metadata, _ := json.Marshal(map[string]string{
		biz.EvoMetaLegacyType: string(s.Type),
	})
	return &biz.UnifiedEvolutionSuggestion{
		ID:              s.ID,
		TargetType:      biz.EvolutionTargetSkill,
		TargetID:        s.SkillID,
		ActionType:      biz.EvolutionActionImprove,
		TriggerSource:   "health",
		TriggerReason:   s.TriggerReason,
		Status:          string(s.Status),
		Priority:        1,
		DraftBody:       s.DraftSkillBody,
		LifecycleStatus: string(s.LifecycleStatus),
		SandboxPassed:   s.SandboxPassed,
		SandboxResult:   s.SandboxResult,
		Metadata:        metadata,
		CreatedAt:       s.CreatedAt,
		ApprovedBy:      s.ApprovedBy,
	}
}

func (s *stubUnifiedEvolutionStore) GetByID(ctx context.Context, id string) (*biz.UnifiedEvolutionSuggestion, error) {
	sug, err := s.stubSuggestionReader.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return legacyToUnifiedView(sug), nil
}

func (s *stubUnifiedEvolutionStore) UpdateDraftBody(ctx context.Context, id string, draftBody string) error {
	return s.stubSuggestionWriter.UpdateDraftBody(ctx, id, draftBody)
}

func (s *stubUnifiedEvolutionStore) UpdateLifecycleStatus(ctx context.Context, id string, lifecycleStatus string) error {
	return s.stubSuggestionWriter.UpdateLifecycleStatus(ctx, id, biz.EvolutionLifecycleStatus(lifecycleStatus))
}

func (s *stubUnifiedEvolutionStore) UpdateSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	return s.stubSuggestionWriter.UpdateSandboxResult(ctx, id, passed, result)
}

// Remaining interface methods are unused by these tests.
func (s *stubUnifiedEvolutionStore) HasPendingForTarget(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *stubUnifiedEvolutionStore) GetLatestByTarget(context.Context, string, string) (*biz.UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (s *stubUnifiedEvolutionStore) GetLatestByTargetAndAction(context.Context, string, string, string) (*biz.UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (s *stubUnifiedEvolutionStore) ListByTarget(context.Context, string, string, string, int, int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (s *stubUnifiedEvolutionStore) CountByTarget(context.Context, string, string, string) (int, error) {
	return 0, nil
}
func (s *stubUnifiedEvolutionStore) ListByTargetAndAction(context.Context, string, string, string, string, int, int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (s *stubUnifiedEvolutionStore) CountByTargetAndAction(context.Context, string, string, string, string) (int, error) {
	return 0, nil
}
func (s *stubUnifiedEvolutionStore) Create(context.Context, biz.UnifiedEvolutionSuggestion) error {
	return s.stubSuggestionWriter.err
}
func (s *stubUnifiedEvolutionStore) UpdateStatus(context.Context, string, string, string, string) error {
	return s.stubSuggestionWriter.err
}
func (s *stubUnifiedEvolutionStore) UpdateStatusCAS(ctx context.Context, id string, _ []string, to string, actor string, reason string) (bool, error) {
	if err := s.stubSuggestionWriter.UpdateStatus(ctx, id, biz.EvolutionSuggestionStatus(to), actor, reason); err != nil {
		return false, err
	}
	return true, nil
}
func (s *stubUnifiedEvolutionStore) UpdateMetadataKey(context.Context, string, string, string) error {
	return s.stubSuggestionWriter.err
}
func (s *stubUnifiedEvolutionStore) ExpireOlderThan(context.Context, time.Time) (int, error) {
	return 0, s.stubSuggestionWriter.err
}

func newTestSandboxRunner(reader *stubSuggestionReader, writer *stubSuggestionWriter) *SandboxRunner {
	store := &stubUnifiedEvolutionStore{stubSuggestionReader: reader, stubSuggestionWriter: writer}
	uc := biz.NewSkillIntelligenceUsecase(nil, nil, store, nil, loggateway.NewNoop())
	// Pass nil factory → rule-based only (no CodeExecutor)
	return NewSandboxRunner(uc, nil, loggateway.NewNoop())
}

// ── SandboxRunner Tests ──────────────────────────────────────────────────────

func TestSandboxRunner_ValidateSuggestion_Pass(t *testing.T) {
	reader := &stubSuggestionReader{
		suggestion: &biz.SkillEvolutionSuggestion{
			ID:             "sug-1",
			SkillID:        "skill-1",
			DraftSkillBody: "# Draft\nSome content here",
			Status:         biz.EvoSuggestionPending,
		},
	}
	writer := &stubSuggestionWriter{}
	runner := newTestSandboxRunner(reader, writer)

	passed, resultJSON, err := runner.ValidateSuggestion(context.Background(), "sug-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected validation to pass for valid suggestion")
	}
	if writer.lastLifecycleStatus != biz.EvoLifecycleReady {
		t.Errorf("expected lifecycle_status=ready, got %q", writer.lastLifecycleStatus)
	}
	if !writer.lastSandboxPassed {
		t.Error("expected sandbox_passed=true")
	}

	var result sandboxValidationResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if len(result.Checks) != 3 {
		t.Errorf("expected 3 checks, got %d", len(result.Checks))
	}
}

func TestSandboxRunner_ValidateSuggestion_EmptyDraftBody(t *testing.T) {
	reader := &stubSuggestionReader{
		suggestion: &biz.SkillEvolutionSuggestion{
			ID:             "sug-2",
			SkillID:        "skill-1",
			DraftSkillBody: "",
			Status:         biz.EvoSuggestionPending,
		},
	}
	writer := &stubSuggestionWriter{}
	runner := newTestSandboxRunner(reader, writer)

	passed, _, err := runner.ValidateSuggestion(context.Background(), "sug-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected validation to fail for empty draft body")
	}
	if writer.lastLifecycleStatus != biz.EvoLifecycleDraft {
		t.Errorf("expected lifecycle_status=draft, got %q", writer.lastLifecycleStatus)
	}
}

func TestSandboxRunner_ValidateSuggestion_NotFound(t *testing.T) {
	reader := &stubSuggestionReader{suggestion: nil}
	writer := &stubSuggestionWriter{}
	runner := newTestSandboxRunner(reader, writer)

	_, _, err := runner.ValidateSuggestion(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent suggestion")
	}
}

func TestSandboxRunner_ValidateSuggestion_DraftBodyTooLong(t *testing.T) {
	longBody := make([]byte, 10001)
	for i := range longBody {
		longBody[i] = 'a'
	}
	reader := &stubSuggestionReader{
		suggestion: &biz.SkillEvolutionSuggestion{
			ID:             "sug-3",
			SkillID:        "skill-1",
			DraftSkillBody: string(longBody),
			Status:         biz.EvoSuggestionPending,
		},
	}
	writer := &stubSuggestionWriter{}
	runner := newTestSandboxRunner(reader, writer)

	passed, _, err := runner.ValidateSuggestion(context.Background(), "sug-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected validation to fail for draft body too long")
	}
}

// ── SkillCuratorService Tests ────────────────────────────────────────────────

func newTestCuratorService(reader *stubSuggestionReader, writer *stubSuggestionWriter) *SkillCuratorService {
	store := &stubUnifiedEvolutionStore{stubSuggestionReader: reader, stubSuggestionWriter: writer}
	uc := biz.NewSkillIntelligenceUsecase(nil, nil, store, nil, loggateway.NewNoop())
	return NewSkillCuratorService(uc, loggateway.NewNoop())
}

func TestSkillCuratorService_GenerateDraft(t *testing.T) {
	writer := &stubSuggestionWriter{}
	reader := &stubSuggestionReader{
		suggestion: &biz.SkillEvolutionSuggestion{
			ID:             "sug-1",
			SkillID:        "skill-1",
			Type:           biz.EvoSuggestionFixFailure,
			TriggerReason:  "7d success rate 30% below threshold 60%",
			DraftSkillBody: "",
			Status:         biz.EvoSuggestionPending,
			CreatedAt:      time.Now().UTC(),
		},
	}
	svc := newTestCuratorService(reader, writer)

	draft, err := svc.GenerateDraft(context.Background(), "sug-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if draft == "" {
		t.Error("expected non-empty draft")
	}
	if !containsSubstring(draft, "fix_failure") {
		t.Errorf("expected draft to mention fix_failure, got: %s", draft)
	}
}

func TestSkillCuratorService_GenerateDraft_NotFound(t *testing.T) {
	reader := &stubSuggestionReader{suggestion: nil}
	writer := &stubSuggestionWriter{}
	svc := newTestCuratorService(reader, writer)

	_, err := svc.GenerateDraft(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent suggestion")
	}
}

func TestSkillCuratorService_GenerateDraft_BoostEfficiency(t *testing.T) {
	writer := &stubSuggestionWriter{}
	reader := &stubSuggestionReader{
		suggestion: &biz.SkillEvolutionSuggestion{
			ID:             "sug-2",
			SkillID:        "skill-2",
			Type:           biz.EvoSuggestionBoostEfficiency,
			TriggerReason:  "Skill score 40 below threshold 60",
			DraftSkillBody: "",
			Status:         biz.EvoSuggestionPending,
			CreatedAt:      time.Now().UTC(),
		},
	}
	svc := newTestCuratorService(reader, writer)

	draft, err := svc.GenerateDraft(context.Background(), "sug-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsSubstring(draft, "boost_efficiency") {
		t.Errorf("expected draft to mention boost_efficiency, got: %s", draft)
	}
}

// ── ExtractCodeBlocks Tests ──────────────────────────────────────────────────

func TestSandboxRunner_ExtractCodeBlocks_NoBlocks(t *testing.T) {
	runner := &SandboxRunner{lg: loggateway.NewNoop()}
	blocks := runner.extractCodeBlocks("# Hello\nNo code here")
	if len(blocks) != 0 {
		t.Errorf("expected 0 code blocks, got %d", len(blocks))
	}
}

func TestSandboxRunner_ExtractCodeBlocks_SingleBlock(t *testing.T) {
	runner := &SandboxRunner{lg: loggateway.NewNoop()}
	input := "# Title\n```python\nprint('hello')\n```\nEnd"
	blocks := runner.extractCodeBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 code block, got %d", len(blocks))
	}
	if blocks[0].Language != "python" {
		t.Errorf("expected language=python, got %q", blocks[0].Language)
	}
	if blocks[0].Code != "print('hello')" {
		t.Errorf("unexpected code: %q", blocks[0].Code)
	}
}

func TestSandboxRunner_ExtractCodeBlocks_MultipleBlocks(t *testing.T) {
	runner := &SandboxRunner{lg: loggateway.NewNoop()}
	input := "```go\nfmt.Println(\"a\")\n```\nText\n```javascript\nconsole.log(\"b\")\n```"
	blocks := runner.extractCodeBlocks(input)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 code blocks, got %d", len(blocks))
	}
	if blocks[0].Language != "go" {
		t.Errorf("expected language=go, got %q", blocks[0].Language)
	}
	if blocks[1].Language != "javascript" {
		t.Errorf("expected language=javascript, got %q", blocks[1].Language)
	}
}

func TestSandboxRunner_ExtractCodeBlocks_EmptyBlock(t *testing.T) {
	runner := &SandboxRunner{lg: loggateway.NewNoop()}
	input := "```python\n```"
	blocks := runner.extractCodeBlocks(input)
	if len(blocks) != 0 {
		t.Errorf("expected 0 code blocks for empty block, got %d", len(blocks))
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && findSubstring(s, sub)))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
