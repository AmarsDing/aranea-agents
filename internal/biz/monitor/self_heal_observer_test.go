package monitor_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// --- mock: HealRecordRepo ---

type mockHealRecordRepo struct {
	insertFn func(ctx context.Context, record monitor.HealRecord) error
	listFn   func(ctx context.Context, query monitor.HealRecordQuery) (monitor.HealRecordListResult, error)
	deleteFn func(ctx context.Context, olderThan time.Time) (int, error)
}

func (m *mockHealRecordRepo) InsertHealRecord(ctx context.Context, record monitor.HealRecord) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, record)
	}
	return nil
}

func (m *mockHealRecordRepo) ListHealRecords(ctx context.Context, query monitor.HealRecordQuery) (monitor.HealRecordListResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, query)
	}
	return monitor.HealRecordListResult{}, nil
}

func (m *mockHealRecordRepo) DeleteHealRecordsOlderThan(ctx context.Context, olderThan time.Time) (int, error) {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, olderThan)
	}
	return 0, nil
}

// --- helpers ---

func newTestObserver(repo monitor.HealRecordRepo, notifier *AlertNotifierCapture) *monitor.SelfHealObserver {
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
	o, err := monitor.NewSelfHealObserver(repo, engine, notifier, loggateway.NewNoop())
	if err != nil {
		panic(err)
	}
	return o
}

// AlertNotifierCapture captures Notify calls for assertions.
type AlertNotifierCapture struct {
	Calls []AlertNotifyCall
}

type AlertNotifyCall struct {
	Rule    monitor.AlertRule
	Payload map[string]any
}

func (n *AlertNotifierCapture) Notify(_ context.Context, rule monitor.AlertRule, payload map[string]any) {
	n.Calls = append(n.Calls, AlertNotifyCall{Rule: rule, Payload: payload})
}

func errorMeta(stepID string, extra ...map[string]any) map[string]any {
	m := map[string]any{
		"flow_phase": "error",
		"step_id":    stepID,
		"trace_id":   "trace-1",
		"session_id": "sess-1",
	}
	for _, e := range extra {
		for k, v := range e {
			m[k] = v
		}
	}
	return m
}

// --- tests ---

func TestNewSelfHealObserver_NilDeps(t *testing.T) {
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
	repo := &mockHealRecordRepo{}
	notifier := &AlertNotifierCapture{}

	// nil repo
	_, err := monitor.NewSelfHealObserver(nil, engine, notifier, loggateway.NewNoop())
	if err == nil {
		t.Error("nil repo should return error")
	}

	// nil engine
	_, err = monitor.NewSelfHealObserver(repo, nil, notifier, loggateway.NewNoop())
	if err == nil {
		t.Error("nil engine should return error")
	}

	// normal
	o, err := monitor.NewSelfHealObserver(repo, engine, notifier, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("normal params should succeed, got: %v", err)
	}
	if o == nil {
		t.Error("observer should not be nil")
	}
}

func TestSelfHealObserver_NilReceiver(t *testing.T) {
	var o *monitor.SelfHealObserver

	// ObserveFlowLogEvent should not panic
	o.ObserveFlowLogEvent(context.Background(), map[string]any{"flow_phase": "error"})

	// GetHealStats should return zero value
	stats, err := o.GetHealStats(context.Background())
	if err != nil {
		t.Fatalf("nil receiver GetHealStats should return nil error, got: %v", err)
	}
	if stats.TotalHeals != 0 || stats.SuccessRate != 0 {
		t.Errorf("nil receiver GetHealStats should return zero, got: %+v", stats)
	}

	// ListHealRecords should return zero value
	result, err := o.ListHealRecords(context.Background(), monitor.HealRecordQuery{})
	if err != nil {
		t.Fatalf("nil receiver ListHealRecords should return nil error, got: %v", err)
	}
	if result.Total != 0 || len(result.Items) != 0 {
		t.Errorf("nil receiver ListHealRecords should return zero, got: %+v", result)
	}
}

func TestSelfHealObserver_ObserveFlowLogEvent_NonErrorPhase(t *testing.T) {
	insertCalled := false
	repo := &mockHealRecordRepo{
		insertFn: func(_ context.Context, _ monitor.HealRecord) error {
			insertCalled = true
			return nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	o.ObserveFlowLogEvent(context.Background(), map[string]any{
		"flow_phase": "success",
		"step_id":    "mcp-connect",
	})

	if insertCalled {
		t.Error("repo should not be called for non-error phase")
	}
}

func TestSelfHealObserver_ObserveFlowLogEvent_EmptyStepID(t *testing.T) {
	insertCalled := false
	repo := &mockHealRecordRepo{
		insertFn: func(_ context.Context, _ monitor.HealRecord) error {
			insertCalled = true
			return nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	o.ObserveFlowLogEvent(context.Background(), map[string]any{
		"flow_phase": "error",
		"step_id":    "",
	})

	if insertCalled {
		t.Error("repo should not be called for empty step_id")
	}
}

func TestSelfHealObserver_ObserveFlowLogEvent_NilMeta(t *testing.T) {
	var o *monitor.SelfHealObserver
	// Should not panic
	o.ObserveFlowLogEvent(context.Background(), nil)

	// Also test with non-nil observer but nil meta
	repo := &mockHealRecordRepo{}
	notifier := &AlertNotifierCapture{}
	obs := newTestObserver(repo, notifier)
	obs.ObserveFlowLogEvent(context.Background(), nil)
}

func TestSelfHealObserver_AutoHealSuccess(t *testing.T) {
	var inserted monitor.HealRecord
	repo := &mockHealRecordRepo{
		insertFn: func(_ context.Context, record monitor.HealRecord) error {
			inserted = record
			return nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
		"auto_healed":  true,
		"heal_success": true,
	}))

	if inserted.Status != "observed_healed" {
		t.Errorf("status = %q, want %q", inserted.Status, "observed_healed")
	}
	if !inserted.RuntimeAutoHealed {
		t.Error("RuntimeAutoHealed should be true")
	}
}

func TestSelfHealObserver_AutoHealFailure_SlidingWindow(t *testing.T) {
	repo := &mockHealRecordRepo{}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	// Send 4 failures → should NOT trigger circuit open
	for i := 0; i < 4; i++ {
		o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
			"auto_healed":  true,
			"heal_success": false,
		}))
	}
	if len(notifier.Calls) != 0 {
		t.Errorf("4 failures should not trigger circuit open, got %d calls", len(notifier.Calls))
	}

	// 5th failure → should trigger circuit open
	o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
		"auto_healed":  true,
		"heal_success": false,
	}))
	if len(notifier.Calls) != 1 {
		t.Fatalf("5th failure should trigger circuit open, got %d calls", len(notifier.Calls))
	}
	cb, _ := notifier.Calls[0].Payload["circuit_breaker"].(bool)
	if !cb {
		t.Error("circuit_breaker should be true in payload")
	}
}

func TestSelfHealObserver_AutoHealFailure_WindowPruning(t *testing.T) {
	repo := &mockHealRecordRepo{}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	// Send 5 failures → triggers circuit open
	for i := 0; i < 5; i++ {
		o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
			"auto_healed":  true,
			"heal_success": false,
		}))
	}
	// Reset notifier
	notifier.Calls = nil

	// Send 1 success → resets window (deletes stepID from healEvents)
	o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
		"auto_healed":  true,
		"heal_success": true,
	}))

	// Now send 4 more failures → should NOT trigger circuit open (only 4 in window)
	for i := 0; i < 4; i++ {
		o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
			"auto_healed":  true,
			"heal_success": false,
		}))
	}
	if len(notifier.Calls) != 0 {
		t.Errorf("4 failures after reset should not trigger circuit open, got %d calls", len(notifier.Calls))
	}
}

func TestSelfHealObserver_AutoHealSuccess_ResetsWindow(t *testing.T) {
	repo := &mockHealRecordRepo{}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	// Send 4 failures
	for i := 0; i < 4; i++ {
		o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
			"auto_healed":  true,
			"heal_success": false,
		}))
	}

	// Send 1 success → resets window
	o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
		"auto_healed":  true,
		"heal_success": true,
	}))

	// Send 4 more failures → should NOT trigger circuit open
	for i := 0; i < 4; i++ {
		o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
			"auto_healed":  true,
			"heal_success": false,
		}))
	}
	if len(notifier.Calls) != 0 {
		t.Errorf("4 failures after window reset should not trigger circuit open, got %d calls", len(notifier.Calls))
	}
}

func TestSelfHealObserver_RootCauseAnalysis_HighConfidence(t *testing.T) {
	var insertedRecords []monitor.HealRecord
	repo := &mockHealRecordRepo{
		insertFn: func(_ context.Context, record monitor.HealRecord) error {
			insertedRecords = append(insertedRecords, record)
			return nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	// "mcp-connect" matches "mcp*" rule with pattern "connection refused", phase=error → confidence >= 0.7
	o.ObserveFlowLogEvent(context.Background(), errorMeta("mcp-connect", map[string]any{
		"auto_healed":   false,
		"error_message": "connection refused",
	}))

	// First inserted record should be observed_failed
	foundObservedFailed := false
	for _, r := range insertedRecords {
		if r.Status == "observed_failed" {
			foundObservedFailed = true
		}
	}
	if !foundObservedFailed {
		t.Errorf("expected observed_failed record, got statuses: %v", statusesOf(insertedRecords))
	}
	if len(notifier.Calls) == 0 {
		t.Error("high confidence root cause should fire alert")
	}
}

func TestSelfHealObserver_RootCauseAnalysis_LowConfidence(t *testing.T) {
	repo := &mockHealRecordRepo{}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	// Use a stepID that doesn't match any builtin rule → engine returns empty → no alert
	o.ObserveFlowLogEvent(context.Background(), errorMeta("unknown-step-xyz", map[string]any{
		"auto_healed": false,
	}))

	if len(notifier.Calls) != 0 {
		t.Error("no matching rule should not fire alert")
	}
}

func TestSelfHealObserver_RootCauseAnalysis_NoCauses(t *testing.T) {
	insertCalled := false
	repo := &mockHealRecordRepo{
		insertFn: func(_ context.Context, _ monitor.HealRecord) error {
			insertCalled = true
			return nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	// "unknown-step" doesn't match any builtin rule pattern
	o.ObserveFlowLogEvent(context.Background(), errorMeta("unknown-step-xyz", map[string]any{
		"auto_healed": false,
	}))

	if insertCalled {
		t.Error("no matching rule should not insert record")
	}
}

func TestSelfHealObserver_Cooldown(t *testing.T) {
	repo := &mockHealRecordRepo{}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	meta := errorMeta("mcp-connect", map[string]any{
		"auto_healed":   false,
		"error_message": "connection refused",
	})

	// First call → fires alert
	o.ObserveFlowLogEvent(context.Background(), meta)
	if len(notifier.Calls) != 1 {
		t.Fatalf("first call should fire alert, got %d calls", len(notifier.Calls))
	}

	// Immediate second call → blocked by cooldown (critical = 30min)
	o.ObserveFlowLogEvent(context.Background(), meta)
	if len(notifier.Calls) != 1 {
		t.Errorf("second call within cooldown should be blocked, got %d calls", len(notifier.Calls))
	}
}

func TestSelfHealObserver_DiagnoseAndObserve_NoCauses(t *testing.T) {
	var inserted monitor.HealRecord
	repo := &mockHealRecordRepo{
		insertFn: func(_ context.Context, record monitor.HealRecord) error {
			inserted = record
			return nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	// "unknown-step" doesn't match any builtin rule
	record, err := o.DiagnoseAndObserve(context.Background(), "trace-1", "sess-1", "unknown-step-xyz", "manual", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Status != "skipped_no_action" {
		t.Errorf("status = %q, want %q", record.Status, "skipped_no_action")
	}
	if inserted.Status != "skipped_no_action" {
		t.Errorf("inserted status = %q, want %q", inserted.Status, "skipped_no_action")
	}
}

func TestSelfHealObserver_DiagnoseAndObserve_HighConfidence(t *testing.T) {
	var inserted monitor.HealRecord
	repo := &mockHealRecordRepo{
		insertFn: func(_ context.Context, record monitor.HealRecord) error {
			inserted = record
			return nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	// "tool-exec" matches "tool*" rule (no pattern required), phase=error → confidence >= 0.7
	record, err := o.DiagnoseAndObserve(context.Background(), "trace-1", "sess-1", "tool-exec", "manual", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Status != "observed_failed" {
		t.Errorf("status = %q, want %q", record.Status, "observed_failed")
	}
	if record.RuleID == "" {
		t.Error("RuleID should not be empty for high confidence match")
	}
	if inserted.RuleID != record.RuleID {
		t.Errorf("inserted RuleID = %q, want %q", inserted.RuleID, record.RuleID)
	}
}

func TestSelfHealObserver_GetHealStats(t *testing.T) {
	repo := &mockHealRecordRepo{
		listFn: func(_ context.Context, _ monitor.HealRecordQuery) (monitor.HealRecordListResult, error) {
			return monitor.HealRecordListResult{
				Items: []monitor.HealRecord{
					{Status: "observed_healed", RuleID: "r1"},
					{Status: "observed_healed", RuleID: "r2"},
					{Status: "observed_failed", RuleID: "r3"},
					{Status: "observed_failed", RuleID: "r3"},
					{Status: "applied", RuleID: "r4"},
				},
				Total: 5,
			}, nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	stats, err := o.GetHealStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalHeals != 5 {
		t.Errorf("TotalHeals = %d, want 5", stats.TotalHeals)
	}
	// observed_healed(2) + applied(1) = 3 successes out of 5
	if stats.SuccessRate != 0.6 {
		t.Errorf("SuccessRate = %.2f, want 0.60", stats.SuccessRate)
	}
	// r3 appears twice in failed
	if len(stats.TopFailRules) != 1 {
		t.Fatalf("TopFailRules len = %d, want 1", len(stats.TopFailRules))
	}
	if stats.TopFailRules[0].RuleID != "r3" || stats.TopFailRules[0].Count != 2 {
		t.Errorf("TopFailRules[0] = %+v, want r3/2", stats.TopFailRules[0])
	}
}

func TestSelfHealObserver_ListHealRecords(t *testing.T) {
	want := monitor.HealRecordListResult{
		Items: []monitor.HealRecord{{ID: "h1"}, {ID: "h2"}},
		Total: 2,
	}
	repo := &mockHealRecordRepo{
		listFn: func(_ context.Context, query monitor.HealRecordQuery) (monitor.HealRecordListResult, error) {
			if query.Limit != 10 {
				t.Errorf("Limit = %d, want 10", query.Limit)
			}
			return want, nil
		},
	}
	notifier := &AlertNotifierCapture{}
	o := newTestObserver(repo, notifier)

	result, err := o.ListHealRecords(context.Background(), monitor.HealRecordQuery{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
}

// statusesOf returns the statuses of the given records for error messages.
func statusesOf(records []monitor.HealRecord) []string {
	s := make([]string, len(records))
	for i, r := range records {
		s[i] = r.Status
	}
	return s
}
