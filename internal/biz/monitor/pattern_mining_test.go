package monitor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// --- mocks for PatternMiningUsecase ---

type mockPatternMiningHealRepo struct {
	records []monitor.HealRecord
	err     error
}

func (m *mockPatternMiningHealRepo) InsertHealRecord(_ context.Context, _ monitor.HealRecord) error {
	return nil
}

func (m *mockPatternMiningHealRepo) ListHealRecords(_ context.Context, query monitor.HealRecordQuery) (monitor.HealRecordListResult, error) {
	if m.err != nil {
		return monitor.HealRecordListResult{}, m.err
	}
	// Filter by status if specified
	var filtered []monitor.HealRecord
	for _, r := range m.records {
		if query.Status != "" && r.Status != query.Status {
			continue
		}
		filtered = append(filtered, r)
	}
	if query.Limit > 0 && len(filtered) > query.Limit {
		filtered = filtered[:query.Limit]
	}
	return monitor.HealRecordListResult{Items: filtered, Total: len(m.records)}, nil
}

func (m *mockPatternMiningHealRepo) DeleteHealRecordsOlderThan(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

type mockPatternMiningPatternReader struct {
	patterns []monitor.FailurePattern
	byHash   map[string]*monitor.FailurePattern
	err      error
}

func (m *mockPatternMiningPatternReader) ListBySource(_ context.Context, source monitor.FailurePatternSource) ([]monitor.FailurePattern, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []monitor.FailurePattern
	for _, p := range m.patterns {
		if p.Source == source {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockPatternMiningPatternReader) GetByPatternHash(_ context.Context, hash string) (*monitor.FailurePattern, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.byHash != nil {
		if p, ok := m.byHash[hash]; ok {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockPatternMiningPatternReader) ListActive(_ context.Context) ([]monitor.FailurePattern, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.patterns, nil
}

type mockPatternMiningPatternWriter struct {
	created []monitor.FailurePattern
	updated []monitor.FailurePattern
	err     error
}

func (m *mockPatternMiningPatternWriter) Create(_ context.Context, pattern monitor.FailurePattern) error {
	if m.err != nil {
		return m.err
	}
	m.created = append(m.created, pattern)
	return nil
}

func (m *mockPatternMiningPatternWriter) Update(_ context.Context, pattern monitor.FailurePattern) error {
	if m.err != nil {
		return m.err
	}
	m.updated = append(m.updated, pattern)
	return nil
}

func (m *mockPatternMiningPatternWriter) IncrementSuccess(_ context.Context, _ string) error {
	return nil
}
func (m *mockPatternMiningPatternWriter) IncrementFail(_ context.Context, _ string) error { return nil }
func (m *mockPatternMiningPatternWriter) Deactivate(_ context.Context, _ string) error    { return nil }

// --- helpers ---

func newTestPatternMiningUsecase(
	healRepo monitor.HealRecordRepo,
	patternReader monitor.FailurePatternReader,
	patternWriter monitor.FailurePatternWriter,
) *monitor.PatternMiningUsecase {
	return monitor.NewPatternMiningUsecase(healRepo, patternReader, patternWriter, loggateway.NewNoop())
}

// makeAppliedHealRecord creates a HealRecord with status "applied" for testing.
func makeAppliedHealRecord(ruleID, errorCode, stackTrace string, fixAction monitor.FixAction) monitor.HealRecord {
	return monitor.HealRecord{
		ID:          fmt.Sprintf("hr-%s-%d", errorCode, time.Now().UnixNano()),
		RuleID:      ruleID,
		TriggerType: "auto",
		Status:      string(monitor.HealStatusApplied),
		FixAction:   fixAction,
		Confidence:  0.8,
		CreatedAt:   time.Now().Format(time.RFC3339),
		Metadata: map[string]any{
			"error_code":  errorCode,
			"stack_trace": stackTrace,
		},
	}
}

// makeFailedHealRecord creates a HealRecord with status "failed" for testing.
func makeFailedHealRecord(ruleID, errorCode, stackTrace string, fixAction monitor.FixAction) monitor.HealRecord {
	return monitor.HealRecord{
		ID:          fmt.Sprintf("hr-fail-%s-%d", errorCode, time.Now().UnixNano()),
		RuleID:      ruleID,
		TriggerType: "auto",
		Status:      string(monitor.HealStatusFailed),
		FixAction:   fixAction,
		Confidence:  0.8,
		CreatedAt:   time.Now().Format(time.RFC3339),
		Metadata: map[string]any{
			"error_code":  errorCode,
			"stack_trace": stackTrace,
		},
	}
}

// --- tests ---

func TestPatternMiningUsecase_NilReceiver(t *testing.T) {
	var uc *monitor.PatternMiningUsecase
	result, err := uc.Mine(context.Background())
	if err == nil {
		t.Error("nil receiver should return error")
	}
	if result != (monitor.PatternMiningResult{}) {
		t.Error("nil receiver should return zero result")
	}
}

func TestPatternMiningUsecase_NilDeps(t *testing.T) {
	healRepo := &mockPatternMiningHealRepo{}
	reader := &mockPatternMiningPatternReader{}
	writer := &mockPatternMiningPatternWriter{}

	if monitor.NewPatternMiningUsecase(nil, reader, writer, loggateway.NewNoop()) != nil {
		t.Error("nil healRepo should return nil")
	}
	if monitor.NewPatternMiningUsecase(healRepo, nil, writer, loggateway.NewNoop()) != nil {
		t.Error("nil patternReader should return nil")
	}
	if monitor.NewPatternMiningUsecase(healRepo, reader, nil, loggateway.NewNoop()) != nil {
		t.Error("nil patternWriter should return nil")
	}
}

func TestPatternMiningUsecase_ClusteringSimilarFailureModes(t *testing.T) {
	fixAction := monitor.FixAction{Type: "retry", MaxAttempts: 2, Params: map[string]any{"backoff_ms": 2000}}
	healRepo := &mockPatternMiningHealRepo{
		records: []monitor.HealRecord{
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:100", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:101", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:102", fixAction),
		},
	}
	reader := &mockPatternMiningPatternReader{patterns: nil}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	result, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should create 1 mined pattern from the cluster of 3 successful fixes
	if result.PatternsCreated != 1 {
		t.Errorf("PatternsCreated = %d, want 1", result.PatternsCreated)
	}
	if len(writer.created) != 1 {
		t.Fatalf("created patterns = %d, want 1", len(writer.created))
	}

	p := writer.created[0]
	if p.Source != monitor.FailurePatternSourceMined {
		t.Errorf("Source = %q, want %q", p.Source, monitor.FailurePatternSourceMined)
	}
	if p.Confidence != 0.5 {
		t.Errorf("Confidence = %.2f, want 0.5", p.Confidence)
	}
	if !p.IsActive {
		t.Error("IsActive should be true for new mined patterns")
	}
	if p.Version != 1 {
		t.Errorf("Version = %d, want 1", p.Version)
	}
	if p.FixAction.Type != "retry" {
		t.Errorf("FixAction.Type = %q, want %q", p.FixAction.Type, "retry")
	}
}

func TestPatternMiningUsecase_InsufficientData_NoTemplate(t *testing.T) {
	fixAction := monitor.FixAction{Type: "retry", MaxAttempts: 2}
	healRepo := &mockPatternMiningHealRepo{
		records: []monitor.HealRecord{
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42", fixAction),
			// Only 2 successful fixes, need >= 3
		},
	}
	reader := &mockPatternMiningPatternReader{patterns: nil}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	result, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT create a pattern with < 3 successful fixes
	if result.PatternsCreated != 0 {
		t.Errorf("PatternsCreated = %d, want 0 (insufficient data)", result.PatternsCreated)
	}
	if len(writer.created) != 0 {
		t.Errorf("created patterns = %d, want 0", len(writer.created))
	}
}

func TestPatternMiningUsecase_ConfidencePromotion(t *testing.T) {
	// Existing mined pattern with 3 successful verifications should get confidence 0.8
	fixAction := monitor.FixAction{Type: "retry", MaxAttempts: 2, Params: map[string]any{"backoff_ms": 2000}}
	// The normalized stack trace from "runtime/llm.go:42\nruntime/llm.go:100" is "runtime/llm.go\nruntime/llm.go"
	hash := monitor.MinedPatternHash("TIMEOUT", "runtime/llm.go\nruntime/llm.go")

	existingPattern := &monitor.FailurePattern{
		ID:           "fp-mined-1",
		Source:       monitor.FailurePatternSourceMined,
		Type:         "TIMEOUT",
		PatternHash:  hash,
		PatternRegex: "TIMEOUT",
		FixAction:    fixAction,
		Confidence:   0.5,
		SuccessCount: 3,
		FailCount:    0,
		Version:      1,
		IsActive:     true,
		CreatedAt:    time.Now().Add(-24 * time.Hour),
		UpdatedAt:    time.Now().Add(-24 * time.Hour),
	}

	healRepo := &mockPatternMiningHealRepo{
		records: []monitor.HealRecord{
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:100", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:101", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:102", fixAction),
		},
	}
	reader := &mockPatternMiningPatternReader{
		byHash: map[string]*monitor.FailurePattern{hash: existingPattern},
	}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	result, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should update existing pattern (not create new one) and promote confidence
	if result.PatternsCreated != 0 {
		t.Errorf("PatternsCreated = %d, want 0 (should update, not create)", result.PatternsCreated)
	}
	if result.PatternsUpdated != 1 {
		t.Errorf("PatternsUpdated = %d, want 1", result.PatternsUpdated)
	}
	if len(writer.updated) != 1 {
		t.Fatalf("updated patterns = %d, want 1", len(writer.updated))
	}

	updated := writer.updated[0]
	if updated.Confidence != 0.8 {
		t.Errorf("Confidence = %.2f, want 0.8 (promoted after 3 successes)", updated.Confidence)
	}
	if updated.Version != 2 {
		t.Errorf("Version = %d, want 2 (version increment)", updated.Version)
	}
}

func TestPatternMiningUsecase_AutoDisable(t *testing.T) {
	// Pattern with fail_count > success_count * 2 should be deactivated
	// After update: success_count = 1 + 3 = 4, fail_count = 10 + 0 = 10
	// 10 > 4 * 2 = 8 → true, should be deactivated
	fixAction := monitor.FixAction{Type: "retry", MaxAttempts: 2}
	hash := monitor.MinedPatternHash("TIMEOUT", "runtime/llm.go\nruntime/llm.go")

	existingPattern := &monitor.FailurePattern{
		ID:           "fp-mined-bad",
		Source:       monitor.FailurePatternSourceMined,
		Type:         "TIMEOUT",
		PatternHash:  hash,
		PatternRegex: "TIMEOUT",
		FixAction:    fixAction,
		Confidence:   0.5,
		SuccessCount: 1,
		FailCount:    10,
		Version:      1,
		IsActive:     true,
		CreatedAt:    time.Now().Add(-24 * time.Hour),
		UpdatedAt:    time.Now().Add(-24 * time.Hour),
	}

	healRepo := &mockPatternMiningHealRepo{
		records: []monitor.HealRecord{
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:100", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:101", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:102", fixAction),
			makeFailedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:100", fixAction),
			makeFailedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:101", fixAction),
			makeFailedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:102", fixAction),
			makeFailedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:103", fixAction),
			makeFailedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:104", fixAction),
			makeFailedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:105", fixAction),
			makeFailedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:106", fixAction),
		},
	}
	reader := &mockPatternMiningPatternReader{
		byHash: map[string]*monitor.FailurePattern{hash: existingPattern},
	}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	result, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should deactivate the pattern
	if result.PatternsDeactivated != 1 {
		t.Errorf("PatternsDeactivated = %d, want 1", result.PatternsDeactivated)
	}
	if len(writer.updated) != 1 {
		t.Fatalf("updated patterns = %d, want 1", len(writer.updated))
	}

	updated := writer.updated[0]
	if updated.IsActive {
		t.Error("IsActive should be false after auto-disable")
	}
}

func TestPatternMiningUsecase_MultipleClusters(t *testing.T) {
	fixRetry := monitor.FixAction{Type: "retry", MaxAttempts: 2, Params: map[string]any{"backoff_ms": 2000}}
	fixReconnect := monitor.FixAction{Type: "reconnect", MaxAttempts: 3, Params: map[string]any{"backoff_ms": 3000}}

	healRepo := &mockPatternMiningHealRepo{
		records: []monitor.HealRecord{
			// Cluster 1: TIMEOUT errors
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:100", fixRetry),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:101", fixRetry),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:102", fixRetry),
			// Cluster 2: CONNECTION_REFUSED errors
			makeAppliedHealRecord("rc-mcp-connection-failure", "CONNECTION_REFUSED", "runtime/mcp.go:10\nruntime/mcp.go:50", fixReconnect),
			makeAppliedHealRecord("rc-mcp-connection-failure", "CONNECTION_REFUSED", "runtime/mcp.go:10\nruntime/mcp.go:51", fixReconnect),
			makeAppliedHealRecord("rc-mcp-connection-failure", "CONNECTION_REFUSED", "runtime/mcp.go:10\nruntime/mcp.go:52", fixReconnect),
		},
	}
	reader := &mockPatternMiningPatternReader{patterns: nil}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	result, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should create 2 mined patterns from 2 clusters
	if result.PatternsCreated != 2 {
		t.Errorf("PatternsCreated = %d, want 2", result.PatternsCreated)
	}
	if len(writer.created) != 2 {
		t.Fatalf("created patterns = %d, want 2", len(writer.created))
	}
}

func TestPatternMiningUsecase_VersionIncrement(t *testing.T) {
	// Existing mined pattern with same hash → version increment
	fixAction := monitor.FixAction{Type: "retry", MaxAttempts: 2}
	hash := monitor.MinedPatternHash("TIMEOUT", "runtime/llm.go\nruntime/llm.go")

	existingPattern := &monitor.FailurePattern{
		ID:           "fp-mined-v1",
		Source:       monitor.FailurePatternSourceMined,
		Type:         "TIMEOUT",
		PatternHash:  hash,
		PatternRegex: "TIMEOUT",
		FixAction:    fixAction,
		Confidence:   0.5,
		SuccessCount: 1,
		FailCount:    0,
		Version:      1,
		IsActive:     true,
		CreatedAt:    time.Now().Add(-48 * time.Hour),
		UpdatedAt:    time.Now().Add(-48 * time.Hour),
	}

	healRepo := &mockPatternMiningHealRepo{
		records: []monitor.HealRecord{
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:100", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:101", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:102", fixAction),
		},
	}
	reader := &mockPatternMiningPatternReader{
		byHash: map[string]*monitor.FailurePattern{hash: existingPattern},
	}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	result, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PatternsUpdated != 1 {
		t.Errorf("PatternsUpdated = %d, want 1", result.PatternsUpdated)
	}
	if len(writer.updated) != 1 {
		t.Fatalf("updated patterns = %d, want 1", len(writer.updated))
	}

	updated := writer.updated[0]
	if updated.Version != 2 {
		t.Errorf("Version = %d, want 2 (incremented)", updated.Version)
	}
}

func TestPatternMiningUsecase_HealRepoError(t *testing.T) {
	healRepo := &mockPatternMiningHealRepo{err: fmt.Errorf("db error")}
	reader := &mockPatternMiningPatternReader{}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	_, err := uc.Mine(context.Background())
	if err == nil {
		t.Error("expected error when heal repo fails")
	}
}

func TestPatternMiningUsecase_NoRecords(t *testing.T) {
	healRepo := &mockPatternMiningHealRepo{records: nil}
	reader := &mockPatternMiningPatternReader{}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	result, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PatternsCreated != 0 {
		t.Errorf("PatternsCreated = %d, want 0 for no records", result.PatternsCreated)
	}
}

func TestPatternMiningUsecase_OnlyAppliedRecordsCount(t *testing.T) {
	fixAction := monitor.FixAction{Type: "retry", MaxAttempts: 2}
	healRepo := &mockPatternMiningHealRepo{
		records: []monitor.HealRecord{
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42", fixAction),
			makeFailedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42", fixAction),
			// Only 2 applied, 1 failed → not enough for template
		},
	}
	reader := &mockPatternMiningPatternReader{patterns: nil}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	result, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 2 applied records, need >= 3
	if result.PatternsCreated != 0 {
		t.Errorf("PatternsCreated = %d, want 0 (only 2 applied records)", result.PatternsCreated)
	}
}

func TestPatternMiningUsecase_ConfidencePromotion_RequiresThreeSuccesses(t *testing.T) {
	// Pattern with only 2 successes should NOT be promoted yet
	fixAction := monitor.FixAction{Type: "retry", MaxAttempts: 2}
	hash := monitor.MinedPatternHash("TIMEOUT", "runtime/llm.go\nruntime/llm.go")

	existingPattern := &monitor.FailurePattern{
		ID:           "fp-mined-2succ",
		Source:       monitor.FailurePatternSourceMined,
		Type:         "TIMEOUT",
		PatternHash:  hash,
		PatternRegex: "TIMEOUT",
		FixAction:    fixAction,
		Confidence:   0.5,
		SuccessCount: 2, // Not yet 3
		FailCount:    0,
		Version:      1,
		IsActive:     true,
		CreatedAt:    time.Now().Add(-24 * time.Hour),
		UpdatedAt:    time.Now().Add(-24 * time.Hour),
	}

	healRepo := &mockPatternMiningHealRepo{
		records: []monitor.HealRecord{
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:100", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:101", fixAction),
			makeAppliedHealRecord("rc-provider-timeout", "TIMEOUT", "runtime/llm.go:42\nruntime/llm.go:102", fixAction),
		},
	}
	reader := &mockPatternMiningPatternReader{
		byHash: map[string]*monitor.FailurePattern{hash: existingPattern},
	}
	writer := &mockPatternMiningPatternWriter{}

	uc := newTestPatternMiningUsecase(healRepo, reader, writer)
	_, err := uc.Mine(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(writer.updated) != 1 {
		t.Fatalf("updated patterns = %d, want 1", len(writer.updated))
	}

	updated := writer.updated[0]
	// 2 existing + 3 new = 5 successes, should be promoted
	if updated.Confidence != 0.8 {
		t.Errorf("Confidence = %.2f, want 0.8 (5 successes >= 3)", updated.Confidence)
	}
}

func TestMinedPatternHash_Deterministic(t *testing.T) {
	h1 := monitor.MinedPatternHash("TIMEOUT", "runtime/llm.go:42")
	h2 := monitor.MinedPatternHash("TIMEOUT", "runtime/llm.go:42")
	h3 := monitor.MinedPatternHash("TIMEOUT", "runtime/llm.go:99")

	if h1 != h2 {
		t.Error("same inputs should produce same hash")
	}
	if h1 == h3 {
		t.Error("different stack traces should produce different hashes")
	}
}
