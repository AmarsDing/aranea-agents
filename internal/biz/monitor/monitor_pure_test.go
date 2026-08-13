package monitor_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

func TestRecoveryThreshold_DefaultFactor(t *testing.T) {
	rule := monitor.AlertRule{Threshold: 100, RecoveryFactor: 0}
	got := monitor.RecoveryThreshold(rule)
	if got != 90 {
		t.Errorf("RecoveryThreshold() = %.2f, want 90 (100 * 0.9)", got)
	}
}

func TestRecoveryThreshold_CustomFactor(t *testing.T) {
	rule := monitor.AlertRule{Threshold: 100, RecoveryFactor: 0.5}
	got := monitor.RecoveryThreshold(rule)
	if got != 50 {
		t.Errorf("RecoveryThreshold() = %.2f, want 50 (100 * 0.5)", got)
	}
}

func TestRecoveryThreshold_FactorOverOne(t *testing.T) {
	rule := monitor.AlertRule{Threshold: 100, RecoveryFactor: 1.5}
	got := monitor.RecoveryThreshold(rule)
	if got != 90 {
		t.Errorf("RecoveryThreshold() = %.2f, want 90 (factor >1 defaults to 0.9)", got)
	}
}

func TestRecoveryThreshold_NegativeFactor(t *testing.T) {
	rule := monitor.AlertRule{Threshold: 100, RecoveryFactor: -0.5}
	got := monitor.RecoveryThreshold(rule)
	if got != 90 {
		t.Errorf("RecoveryThreshold() = %.2f, want 90 (negative factor defaults to 0.9)", got)
	}
}

func TestRecoveryThreshold_ZeroThreshold(t *testing.T) {
	rule := monitor.AlertRule{Threshold: 0, RecoveryFactor: 0.9}
	got := monitor.RecoveryThreshold(rule)
	if got != 0 {
		t.Errorf("RecoveryThreshold() = %.2f, want 0", got)
	}
}

func TestRecoveryThreshold_FactorOne(t *testing.T) {
	rule := monitor.AlertRule{Threshold: 100, RecoveryFactor: 1.0}
	got := monitor.RecoveryThreshold(rule)
	if got != 100 {
		t.Errorf("RecoveryThreshold() = %.2f, want 100", got)
	}
}

func TestMergeRunnerCompletionUsagePatch_BothFields(t *testing.T) {
	got := monitor.MergeRunnerCompletionUsagePatch("usage-123", "trace-456")
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["usage_event_id"] != "usage-123" {
		t.Errorf("usage_event_id = %v, want %q", m["usage_event_id"], "usage-123")
	}
	if m["trace_id"] != "trace-456" {
		t.Errorf("trace_id = %v, want %q", m["trace_id"], "trace-456")
	}
	if m["schema_version"] != "runner.completion/v1" {
		t.Errorf("schema_version = %v, want %q", m["schema_version"], "runner.completion/v1")
	}
}

func TestMergeRunnerCompletionUsagePatch_OnlyUsageEventID(t *testing.T) {
	got := monitor.MergeRunnerCompletionUsagePatch("usage-123", "")
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := m["usage_event_id"]; !ok {
		t.Error("usage_event_id missing")
	}
	if _, ok := m["trace_id"]; ok {
		t.Error("trace_id should not be present when empty")
	}
}

func TestMergeRunnerCompletionUsagePatch_OnlyTraceID(t *testing.T) {
	got := monitor.MergeRunnerCompletionUsagePatch("", "trace-456")
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := m["usage_event_id"]; ok {
		t.Error("usage_event_id should not be present when empty")
	}
	if _, ok := m["trace_id"]; !ok {
		t.Error("trace_id missing")
	}
}

func TestMergeRunnerCompletionUsagePatch_BothEmpty(t *testing.T) {
	got := monitor.MergeRunnerCompletionUsagePatch("", "")
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(m) != 1 {
		t.Errorf("expected only schema_version, got %d keys", len(m))
	}
}

func TestMergeRunnerCompletionUsagePatch_WhitespaceOnly(t *testing.T) {
	got := monitor.MergeRunnerCompletionUsagePatch("  ", "  ")
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := m["usage_event_id"]; ok {
		t.Error("usage_event_id should not be present for whitespace-only input")
	}
	if _, ok := m["trace_id"]; ok {
		t.Error("trace_id should not be present for whitespace-only input")
	}
}

func TestShouldFireAlert_NilUsecase(t *testing.T) {
	var u *monitor.Usecase
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 60}
	if u.ShouldFireAlert(rule, time.Now()) {
		t.Error("nil.ShouldFireAlert() = true, want false")
	}
}

func TestShouldFireAlert_NoLastFired(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 60}
	if !u.ShouldFireAlert(rule, time.Now()) {
		t.Error("ShouldFireAlert() = false, want true (no last fired)")
	}
}

func TestShouldFireAlert_WithinCooldown(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	lastFired := now.Add(-30 * time.Minute)
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 60, LastFiredAt: &lastFired}
	if u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = true, want false (within cooldown)")
	}
}

func TestShouldFireAlert_AfterCooldown(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	lastFired := now.Add(-61 * time.Minute)
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 60, LastFiredAt: &lastFired}
	if !u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = false, want true (after cooldown)")
	}
}

func TestShouldFireAlert_ZeroCooldown(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	lastFired := now.Add(-30 * time.Minute)
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 0, LastFiredAt: &lastFired}
	if u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = true, want false (zero cooldown defaults to 60)")
	}
}

func TestShouldFireAlert_RecoveredWithinCooldown(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	recoveredAt := now.Add(-10 * time.Minute)
	rule := monitor.AlertRule{
		ID:              "r1",
		CooldownMinutes: 60,
		FiringState:     monitor.AlertFiringStateRecovered,
		RecoveredAt:     &recoveredAt,
	}
	// Recovered state no longer enforces a separate RecoveredAt cooldown;
	// only LastFiredAt cooldown applies. Without LastFiredAt, firing is allowed.
	if !u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = false, want true (recovered no longer blocks via RecoveredAt cooldown)")
	}
}

func TestShouldFireAlert_RecoveredAfterCooldown(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	recoveredAt := now.Add(-61 * time.Minute)
	rule := monitor.AlertRule{
		ID:              "r1",
		CooldownMinutes: 60,
		FiringState:     monitor.AlertFiringStateRecovered,
		RecoveredAt:     &recoveredAt,
	}
	if !u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = false, want true (recovered after cooldown)")
	}
}

func TestShouldFireAlert_InMemoryFallback(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 60}
	u.MarkAlertFired("r1", now)
	if u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = true, want false (in-memory cooldown)")
	}
}

func TestShouldFireAlert_InMemoryFallbackExpired(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	past := now.Add(-61 * time.Minute)
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 60}
	u.MarkAlertFired("r1", past)
	if !u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = false, want true (in-memory cooldown expired)")
	}
}

func TestShouldFireAlert_DBPersistedTakesPrecedence(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	lastFired := now.Add(-5 * time.Minute)
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 60, LastFiredAt: &lastFired}
	u.MarkAlertFired("r1", now.Add(-61*time.Minute))
	if u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = true, want false (DB LastFiredAt takes precedence)")
	}
}

func TestMarkAlertFired(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	u.MarkAlertFired("r1", now)
	rule := monitor.AlertRule{ID: "r1", CooldownMinutes: 60}
	if u.ShouldFireAlert(rule, now) {
		t.Error("ShouldFireAlert() = true after MarkAlertFired, want false")
	}
}

func TestMarkAlertFired_NilUsecase(t *testing.T) {
	var u *monitor.Usecase
	u.MarkAlertFired("r1", time.Now())
}

func TestCleanupStaleLastFired(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	now := time.Now()
	u.MarkAlertFired("old", now.Add(-25*time.Hour))
	u.MarkAlertFired("recent", now.Add(-30*time.Minute))

	u.CleanupStaleLastFired(now, 24*time.Hour)

	rule := monitor.AlertRule{ID: "old", CooldownMinutes: 60}
	if !u.ShouldFireAlert(rule, now) {
		t.Error("old entry should have been cleaned up")
	}

	ruleRecent := monitor.AlertRule{ID: "recent", CooldownMinutes: 60}
	if u.ShouldFireAlert(ruleRecent, now) {
		t.Error("recent entry should still be within cooldown")
	}
}

func TestCleanupStaleLastFired_NilUsecase(t *testing.T) {
	var u *monitor.Usecase
	u.CleanupStaleLastFired(time.Now(), time.Hour)
}

func TestCleanupStaleLastFired_ZeroMaxAge(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	u.MarkAlertFired("r1", time.Now().Add(-25*time.Hour))
	u.CleanupStaleLastFired(time.Now(), 0)
}

func TestCleanupStaleLastFired_NegativeMaxAge(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	u.MarkAlertFired("r1", time.Now().Add(-25*time.Hour))
	u.CleanupStaleLastFired(time.Now(), -1*time.Hour)
}

func TestAlertFiringState_Constants(t *testing.T) {
	if monitor.AlertFiringStateIdle != "idle" {
		t.Errorf("AlertFiringStateIdle = %q, want %q", monitor.AlertFiringStateIdle, "idle")
	}
	if monitor.AlertFiringStateFiring != "firing" {
		t.Errorf("AlertFiringStateFiring = %q, want %q", monitor.AlertFiringStateFiring, "firing")
	}
	if monitor.AlertFiringStateRecovered != "recovered" {
		t.Errorf("AlertFiringStateRecovered = %q, want %q", monitor.AlertFiringStateRecovered, "recovered")
	}
}

func TestNewUsecase_WithOptions(t *testing.T) {
	reg := monitor.NewAlertMetricRegistry()
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil,
		monitor.WithRegistry(reg),
	)
	if u == nil {
		t.Fatal("NewUsecase() = nil")
	}
}

func TestUsecase_SetEvalWorker(t *testing.T) {
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	w := monitor.NewAlertEvalWorker(u, monitor.NewMetricRingBuffer(), loggateway.NewNoop())
	u.SetEvalWorker(w)
	if u.EvalWorker() != w {
		t.Error("SetEvalWorker did not set the worker")
	}
}

func TestUsecase_SetEvalWorker_Nil(t *testing.T) {
	var u *monitor.Usecase
	u.SetEvalWorker(nil)
}

func TestUsecase_EvalWorker_NilUsecase(t *testing.T) {
	var u *monitor.Usecase
	if u.EvalWorker() != nil {
		t.Error("nil.EvalWorker() should return nil")
	}
}

func TestUsecase_WithRegistry(t *testing.T) {
	reg := monitor.NewAlertMetricRegistry()
	u := monitor.NewUsecase(nil, nil, nil, nil, nil, nil, monitor.WithRegistry(reg))
	if u.Registry() != reg {
		t.Error("WithRegistry did not set the registry")
	}
}

func TestUsecase_Registry_NilUsecase(t *testing.T) {
	var u *monitor.Usecase
	if u.Registry() != nil {
		t.Error("nil.Registry() should return nil")
	}
}

func TestMergeRunnerCompletionUsagePatch_AlwaysHasSchemaVersion(t *testing.T) {
	tests := []struct {
		name         string
		usageEventID string
		traceID      string
	}{
		{"both_empty", "", ""},
		{"usage_only", "u1", ""},
		{"trace_only", "", "t1"},
		{"both_set", "u1", "t1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.MergeRunnerCompletionUsagePatch(tt.usageEventID, tt.traceID)
			if !strings.Contains(got, "schema_version") {
				t.Errorf("patch %q missing schema_version", got)
			}
		})
	}
}
