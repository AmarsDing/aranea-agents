package monitor_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// --- mocks for PredictiveHealUsecase ---

// mockSystemMetricsReader captures metric reads for test assertions.
type mockSystemMetricsReader struct {
	metrics monitor.SystemMetrics
	err     error
}

func (m *mockSystemMetricsReader) ReadSystemMetrics(_ context.Context) (monitor.SystemMetrics, error) {
	if m.err != nil {
		return monitor.SystemMetrics{}, m.err
	}
	return m.metrics, nil
}

// mockFailurePatternReaderForPredictive captures pattern reads for test assertions.
type mockFailurePatternReaderForPredictive struct {
	patterns []monitor.FailurePattern
	err      error
}

func (m *mockFailurePatternReaderForPredictive) ListActive(_ context.Context) ([]monitor.FailurePattern, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.patterns, nil
}

func (m *mockFailurePatternReaderForPredictive) ListBySource(_ context.Context, _ monitor.FailurePatternSource) ([]monitor.FailurePattern, error) {
	return m.ListActive(nil)
}

func (m *mockFailurePatternReaderForPredictive) GetByPatternHash(_ context.Context, _ string) (*monitor.FailurePattern, error) {
	return nil, nil
}

// mockHealHandlerForPredictive records fix action calls for test assertions.
type mockHealHandlerForPredictive struct {
	calls      atomic.Int32
	lastAction monitor.FixAction
	lastMeta   map[string]any
	shouldErr  bool
}

func (h *mockHealHandlerForPredictive) HandleFixAction(_ context.Context, action monitor.FixAction, meta map[string]any) error {
	h.calls.Add(1)
	h.lastAction = action
	h.lastMeta = meta
	if h.shouldErr {
		return errPredictiveFixFailed
	}
	return nil
}

var errPredictiveFixFailed = &errPredictiveMarker{"predictive fix action failed"}

type errPredictiveMarker struct{ msg string }

func (e *errPredictiveMarker) Error() string { return e.msg }

// --- helper to create a PredictiveHealUsecase for tests ---

func newTestPredictiveHealUsecase(
	reader *mockSystemMetricsReader,
	patternReader *mockFailurePatternReaderForPredictive,
	handler *mockHealHandlerForPredictive,
	healRepo monitor.HealRecordRepo,
) *monitor.PredictiveHealUsecase {
	return monitor.NewPredictiveHealUsecase(reader, patternReader, handler, healRepo, loggateway.NewNoop())
}

// --- tests ---

func TestPredictiveHealUsecase_NilReceiver(t *testing.T) {
	var uc *monitor.PredictiveHealUsecase
	records, err := uc.PredictAndHeal(context.Background())
	if err == nil {
		t.Error("nil receiver should return error")
	}
	if records != nil {
		t.Error("nil receiver should return nil records")
	}
}

func TestPredictiveHealUsecase_NilDeps(t *testing.T) {
	reader := &mockSystemMetricsReader{}
	patternReader := &mockFailurePatternReaderForPredictive{}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	// nil metrics reader
	if monitor.NewPredictiveHealUsecase(nil, patternReader, handler, repo, loggateway.NewNoop()) != nil {
		t.Error("nil metrics reader should return nil")
	}
	// nil pattern reader
	if monitor.NewPredictiveHealUsecase(reader, nil, handler, repo, loggateway.NewNoop()) != nil {
		t.Error("nil pattern reader should return nil")
	}
	// nil handler
	if monitor.NewPredictiveHealUsecase(reader, patternReader, nil, repo, loggateway.NewNoop()) != nil {
		t.Error("nil handler should return nil")
	}
	// nil repo
	if monitor.NewPredictiveHealUsecase(reader, patternReader, handler, nil, loggateway.NewNoop()) != nil {
		t.Error("nil repo should return nil")
	}
}

func TestPredictiveHealUsecase_LowConfidence_NoAction(t *testing.T) {
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 500,
			MemoryUsagePct:    60,
			SessionBacklog:    5,
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{
			{
				ID:         "fp-low-conf",
				Type:       "provider_timeout",
				Confidence: 0.5, // Below 0.8 threshold
				IsActive:   true,
				FixAction:  monitor.FixAction{Type: "retry", MaxAttempts: 2},
			},
		},
	}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	records, err := uc.PredictAndHeal(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce a record with skipped_low_confidence status
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Status != string(monitor.HealStatusSkippedLowConfidence) {
		t.Errorf("Status = %q, want %q", records[0].Status, monitor.HealStatusSkippedLowConfidence)
	}
	// Handler should NOT be called
	if handler.calls.Load() != 0 {
		t.Error("HandleFixAction should not be called for low confidence")
	}
}

func TestPredictiveHealUsecase_HighConfidence_ActionApplied(t *testing.T) {
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 5000, // High latency
			MemoryUsagePct:    60,
			SessionBacklog:    5,
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{
			{
				ID:         "fp-provider-timeout",
				Type:       "provider_timeout",
				Confidence: 0.9, // Above 0.8 threshold
				IsActive:   true,
				FixAction:  monitor.FixAction{Type: "retry", MaxAttempts: 2, Params: map[string]any{"backoff_ms": 2000}},
			},
		},
	}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	records, err := uc.PredictAndHeal(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Status != string(monitor.HealStatusApplied) {
		t.Errorf("Status = %q, want %q", records[0].Status, monitor.HealStatusApplied)
	}
	if records[0].Confidence < 0.8 {
		t.Errorf("Confidence = %.2f, want >= 0.8", records[0].Confidence)
	}
	if records[0].TriggerType != "predictive" {
		t.Errorf("TriggerType = %q, want %q", records[0].TriggerType, "predictive")
	}
	if handler.calls.Load() != 1 {
		t.Errorf("HandleFixAction called %d times, want 1", handler.calls.Load())
	}
	if handler.lastAction.Type != "retry" {
		t.Errorf("FixAction.Type = %q, want %q", handler.lastAction.Type, "retry")
	}
}

func TestPredictiveHealUsecase_CooldownPreventsRepeat(t *testing.T) {
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 5000,
			MemoryUsagePct:    60,
			SessionBacklog:    5,
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{
			{
				ID:         "fp-provider-timeout",
				Type:       "provider_timeout",
				Confidence: 0.9,
				IsActive:   true,
				FixAction:  monitor.FixAction{Type: "retry", MaxAttempts: 2},
			},
		},
	}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	// First call should succeed
	records1, _ := uc.PredictAndHeal(context.Background())
	if len(records1) != 1 || records1[0].Status != string(monitor.HealStatusApplied) {
		t.Fatalf("first call: Status = %q, want applied", records1[0].Status)
	}

	// Second call immediately after should be in cooldown
	records2, _ := uc.PredictAndHeal(context.Background())
	if len(records2) != 1 {
		t.Fatalf("second call: expected 1 record, got %d", len(records2))
	}
	if records2[0].Status != string(monitor.HealStatusSkippedCooldown) {
		t.Errorf("second call: Status = %q, want %q", records2[0].Status, monitor.HealStatusSkippedCooldown)
	}
	// Handler should only be called once (from first call)
	if handler.calls.Load() != 1 {
		t.Errorf("HandleFixAction called %d times, want 1", handler.calls.Load())
	}
}

func TestPredictiveHealUsecase_CooldownExpires(t *testing.T) {
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 5000,
			MemoryUsagePct:    60,
			SessionBacklog:    5,
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{
			{
				ID:         "fp-provider-timeout",
				Type:       "provider_timeout",
				Confidence: 0.9,
				IsActive:   true,
				FixAction:  monitor.FixAction{Type: "retry", MaxAttempts: 2},
			},
		},
	}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	// First call
	records1, _ := uc.PredictAndHeal(context.Background())
	if records1[0].Status != string(monitor.HealStatusApplied) {
		t.Fatalf("first call: Status = %q, want applied", records1[0].Status)
	}

	// Manually expire cooldown for testing
	uc.SetCooldownForTest("provider_timeout", time.Now().Add(-31*time.Minute))

	// After cooldown expires, should be able to apply again
	records3, _ := uc.PredictAndHeal(context.Background())
	if len(records3) != 1 {
		t.Fatalf("third call: expected 1 record, got %d", len(records3))
	}
	if records3[0].Status != string(monitor.HealStatusApplied) {
		t.Errorf("third call after cooldown: Status = %q, want %q", records3[0].Status, monitor.HealStatusApplied)
	}
	if handler.calls.Load() != 2 {
		t.Errorf("HandleFixAction called %d times, want 2", handler.calls.Load())
	}
}

func TestPredictiveHealUsecase_AuditRecordCreated(t *testing.T) {
	var insertedRecords []monitor.HealRecord
	repo := &mockHealRecordRepo{
		insertFn: func(_ context.Context, record monitor.HealRecord) error {
			insertedRecords = append(insertedRecords, record)
			return nil
		},
	}
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 5000,
			MemoryUsagePct:    60,
			SessionBacklog:    5,
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{
			{
				ID:         "fp-provider-timeout",
				Type:       "provider_timeout",
				Confidence: 0.9,
				IsActive:   true,
				FixAction:  monitor.FixAction{Type: "retry", MaxAttempts: 2},
			},
		},
	}
	handler := &mockHealHandlerForPredictive{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	records, _ := uc.PredictAndHeal(context.Background())
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	// Verify audit record was persisted
	if len(insertedRecords) != 1 {
		t.Fatalf("expected 1 inserted record, got %d", len(insertedRecords))
	}
	ir := insertedRecords[0]
	if ir.TriggerType != "predictive" {
		t.Errorf("inserted TriggerType = %q, want %q", ir.TriggerType, "predictive")
	}
	if ir.Status != string(monitor.HealStatusApplied) {
		t.Errorf("inserted Status = %q, want %q", ir.Status, monitor.HealStatusApplied)
	}
	if ir.RuleID != "fp-provider-timeout" {
		t.Errorf("inserted RuleID = %q, want %q", ir.RuleID, "fp-provider-timeout")
	}
	if ir.Confidence < 0.8 {
		t.Errorf("inserted Confidence = %.2f, want >= 0.8", ir.Confidence)
	}
}

func TestPredictiveHealUsecase_NoMatchingPatterns(t *testing.T) {
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 100, // Normal metrics
			MemoryUsagePct:    40,
			SessionBacklog:    2,
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{}, // No patterns
	}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	records, err := uc.PredictAndHeal(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No patterns → no records
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
	if handler.calls.Load() != 0 {
		t.Error("HandleFixAction should not be called when no patterns match")
	}
}

func TestPredictiveHealUsecase_FixActionFailed(t *testing.T) {
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 5000,
			MemoryUsagePct:    60,
			SessionBacklog:    5,
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{
			{
				ID:         "fp-provider-timeout",
				Type:       "provider_timeout",
				Confidence: 0.9,
				IsActive:   true,
				FixAction:  monitor.FixAction{Type: "retry", MaxAttempts: 2},
			},
		},
	}
	handler := &mockHealHandlerForPredictive{shouldErr: true}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	records, _ := uc.PredictAndHeal(context.Background())
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Status != string(monitor.HealStatusFailed) {
		t.Errorf("Status = %q, want %q", records[0].Status, monitor.HealStatusFailed)
	}
}

func TestPredictiveHealUsecase_MultiplePatterns(t *testing.T) {
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 5000,
			MemoryUsagePct:    90, // High memory
			SessionBacklog:    50, // High backlog
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{
			{
				ID:         "fp-provider-timeout",
				Type:       "provider_timeout",
				Confidence: 0.85,
				IsActive:   true,
				FixAction:  monitor.FixAction{Type: "retry", MaxAttempts: 2},
			},
			{
				ID:         "fp-memory-pressure",
				Type:       "memory_pressure",
				Confidence: 0.6, // Below threshold even with metric boost (0.6+0.15=0.75 < 0.8)
				IsActive:   true,
				FixAction:  monitor.FixAction{Type: "fallback", MaxAttempts: 1},
			},
		},
	}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	records, _ := uc.PredictAndHeal(context.Background())

	// Should produce 2 records: one applied (high conf), one skipped (low conf)
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	appliedCount := 0
	skippedCount := 0
	for _, r := range records {
		if r.Status == string(monitor.HealStatusApplied) {
			appliedCount++
		} else if r.Status == string(monitor.HealStatusSkippedLowConfidence) {
			skippedCount++
		}
	}
	if appliedCount != 1 {
		t.Errorf("applied count = %d, want 1", appliedCount)
	}
	if skippedCount != 1 {
		t.Errorf("skipped count = %d, want 1", skippedCount)
	}
}

func TestPredictiveHealUsecase_MetricsReaderError(t *testing.T) {
	reader := &mockSystemMetricsReader{
		err: errPredictiveFixFailed,
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{},
	}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	_, err := uc.PredictAndHeal(context.Background())
	if err == nil {
		t.Error("expected error when metrics reader fails")
	}
}

func TestPredictiveHealUsecase_InactivePatternSkipped(t *testing.T) {
	reader := &mockSystemMetricsReader{
		metrics: monitor.SystemMetrics{
			ProviderLatencyMs: 5000,
			MemoryUsagePct:    60,
			SessionBacklog:    5,
		},
	}
	patternReader := &mockFailurePatternReaderForPredictive{
		patterns: []monitor.FailurePattern{
			{
				ID:         "fp-inactive",
				Type:       "provider_timeout",
				Confidence: 0.9,
				IsActive:   false, // Inactive
				FixAction:  monitor.FixAction{Type: "retry", MaxAttempts: 2},
			},
		},
	}
	handler := &mockHealHandlerForPredictive{}
	repo := &mockHealRecordRepo{}

	uc := newTestPredictiveHealUsecase(reader, patternReader, handler, repo)

	records, _ := uc.PredictAndHeal(context.Background())

	// Inactive patterns should be skipped entirely
	if len(records) != 0 {
		t.Errorf("expected 0 records for inactive patterns, got %d", len(records))
	}
	if handler.calls.Load() != 0 {
		t.Error("HandleFixAction should not be called for inactive patterns")
	}
}
