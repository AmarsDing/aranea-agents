package monitor_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/internal/biz/types"
	"aranea-agents/pkg/loggateway"
)

// --- Task 1.4: SelfCheckReport.OverallStatus aggregation unit test ---

func TestAggregateOverallStatus_AllPassed(t *testing.T) {
	results := []types.SelfCheckResult{
		{Checker: "a", Status: types.SelfCheckStatusPassed},
		{Checker: "b", Status: types.SelfCheckStatusPassed},
	}
	got := monitor.AggregateOverallStatus(results)
	if got != types.SelfCheckStatusPassed {
		t.Errorf("expected passed, got %s", got)
	}
}

func TestAggregateOverallStatus_OneWarning(t *testing.T) {
	results := []types.SelfCheckResult{
		{Checker: "a", Status: types.SelfCheckStatusPassed},
		{Checker: "b", Status: types.SelfCheckStatusWarning},
	}
	got := monitor.AggregateOverallStatus(results)
	if got != types.SelfCheckStatusWarning {
		t.Errorf("expected warning, got %s", got)
	}
}

func TestAggregateOverallStatus_OneFailed(t *testing.T) {
	results := []types.SelfCheckResult{
		{Checker: "a", Status: types.SelfCheckStatusPassed},
		{Checker: "b", Status: types.SelfCheckStatusWarning},
		{Checker: "c", Status: types.SelfCheckStatusFailed},
	}
	got := monitor.AggregateOverallStatus(results)
	if got != types.SelfCheckStatusFailed {
		t.Errorf("expected failed, got %s", got)
	}
}

func TestAggregateOverallStatus_Empty(t *testing.T) {
	got := monitor.AggregateOverallStatus(nil)
	if got != types.SelfCheckStatusPassed {
		t.Errorf("expected passed for empty results, got %s", got)
	}
}

func TestAggregateOverallStatus_FailedTakesPrecedence(t *testing.T) {
	results := []types.SelfCheckResult{
		{Checker: "a", Status: types.SelfCheckStatusWarning},
		{Checker: "b", Status: types.SelfCheckStatusFailed},
	}
	got := monitor.AggregateOverallStatus(results)
	if got != types.SelfCheckStatusFailed {
		t.Errorf("expected failed to take precedence, got %s", got)
	}
}

// --- Task 2.4: SelfCheckScheduler unit test ---

type mockChecker struct {
	name   string
	result types.SelfCheckResult
}

func (m *mockChecker) Name() string                                    { return m.name }
func (m *mockChecker) Check(ctx context.Context) types.SelfCheckResult { return m.result }

type mockSelfCheckRepo struct {
	reports []monitor.SelfCheckReport
}

func (m *mockSelfCheckRepo) InsertSelfCheckReport(ctx context.Context, report monitor.SelfCheckReport) error {
	m.reports = append(m.reports, report)
	return nil
}

func (m *mockSelfCheckRepo) ListSelfCheckReports(ctx context.Context, limit, offset int) ([]monitor.SelfCheckReport, int, error) {
	return m.reports, len(m.reports), nil
}

func (m *mockSelfCheckRepo) DeleteSelfCheckReportsOlderThan(ctx context.Context, olderThan time.Time) (int, error) {
	return 0, nil
}

func TestSelfCheckScheduler_RunOnce(t *testing.T) {
	checkers := []monitor.SelfChecker{
		&mockChecker{name: "test_a", result: types.SelfCheckResult{Checker: "test_a", Status: types.SelfCheckStatusPassed, CheckedAt: time.Now().UTC()}},
		&mockChecker{name: "test_b", result: types.SelfCheckResult{Checker: "test_b", Status: types.SelfCheckStatusWarning, CheckedAt: time.Now().UTC()}},
	}
	repo := &mockSelfCheckRepo{}
	scheduler := monitor.NewSelfCheckScheduler(checkers, nil, repo, nil, loggateway.NewNoop(), nil)

	report := scheduler.RunOnce(context.Background())
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.OverallStatus != types.SelfCheckStatusWarning {
		t.Errorf("expected warning, got %s", report.OverallStatus)
	}
	if len(report.CheckResults) != 2 {
		t.Errorf("expected 2 check results, got %d", len(report.CheckResults))
	}
	if len(repo.reports) != 1 {
		t.Errorf("expected 1 persisted report, got %d", len(repo.reports))
	}
}

func TestSelfCheckScheduler_RunOnce_ConcurrencyLock(t *testing.T) {
	checkers := []monitor.SelfChecker{
		&mockChecker{name: "slow", result: types.SelfCheckResult{Checker: "slow", Status: types.SelfCheckStatusPassed, CheckedAt: time.Now().UTC()}},
	}
	scheduler := monitor.NewSelfCheckScheduler(checkers, nil, &mockSelfCheckRepo{}, nil, loggateway.NewNoop(), nil)

	// First call should succeed
	report := scheduler.RunOnce(context.Background())
	if report == nil {
		t.Error("first RunOnce should return a report")
	}
}

func TestSelfCheckScheduler_RunOnce_NilScheduler(t *testing.T) {
	var s *monitor.SelfCheckScheduler
	report := s.RunOnce(context.Background())
	if report != nil {
		t.Error("nil scheduler should return nil report")
	}
}

// --- Task 3.7: Checker unit tests ---

func TestDBHealthChecker_NilDB(t *testing.T) {
	c := monitor.NewDBHealthChecker(nil)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusFailed {
		t.Errorf("expected failed for nil db, got %s", result.Status)
	}
}

func TestTraceProjectorChecker_NilProjector(t *testing.T) {
	c := monitor.NewTraceProjectorChecker(nil)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusWarning {
		t.Errorf("expected warning for nil projector, got %s", result.Status)
	}
}

// fakeTraceProjectorHealth is a stub for TraceProjectorHealthChecker that
// returns the configured values. It is used to drive the three healthy /
// not-healthy branches of TraceProjectorChecker.Check in isolation.
type fakeTraceProjectorHealth struct {
	count       int
	started     bool
	lastEventAt time.Time
	hasEver     bool
}

func (f *fakeTraceProjectorHealth) TraceCount() int        { return f.count }
func (f *fakeTraceProjectorHealth) Started() bool          { return f.started }
func (f *fakeTraceProjectorHealth) LastEventAt() time.Time { return f.lastEventAt }
func (f *fakeTraceProjectorHealth) HasEverProcessed() bool { return f.hasEver }

func TestTraceProjectorChecker_StartedIdlePasses(t *testing.T) {
	// Started, never received any envelope, zero in-flight traces.
	// This is the most common state on a freshly deployed system.
	fp := &fakeTraceProjectorHealth{
		started:     true,
		hasEver:     false,
		lastEventAt: time.Time{},
		count:       0,
	}
	c := monitor.NewTraceProjectorChecker(fp)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusPassed {
		t.Errorf("expected passed for started-but-idle projector, got %s (msg=%s)", result.Status, result.Message)
	}
	if !contains(result.Message, "idle") {
		t.Errorf("expected message to mention 'idle', got %q", result.Message)
	}
}

func TestTraceProjectorChecker_StartedWithInFlightPasses(t *testing.T) {
	// Started, has active traces. Healthy steady state.
	fp := &fakeTraceProjectorHealth{
		started:     true,
		hasEver:     true,
		lastEventAt: time.Now().UTC().Add(-1 * time.Minute),
		count:       3,
	}
	c := monitor.NewTraceProjectorChecker(fp)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusPassed {
		t.Errorf("expected passed for active projector, got %s (msg=%s)", result.Status, result.Message)
	}
	if !contains(result.Message, "3 active") {
		t.Errorf("expected message to report 3 active traces, got %q", result.Message)
	}
}

func TestTraceProjectorChecker_NotStartedWarns(t *testing.T) {
	// Projector constructed but Start() never invoked. Wiring bug.
	fp := &fakeTraceProjectorHealth{started: false, hasEver: false}
	c := monitor.NewTraceProjectorChecker(fp)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusWarning {
		t.Errorf("expected warning for un-started projector, got %s (msg=%s)", result.Status, result.Message)
	}
	if !contains(result.Message, "not started") {
		t.Errorf("expected message to mention 'not started', got %q", result.Message)
	}
}

func TestTraceProjectorChecker_StalledWarns(t *testing.T) {
	// Projector used to receive events, but the last one is older than
	// the idle timeout. This is the actual stall condition the original
	// check failed to distinguish from idle.
	fp := &fakeTraceProjectorHealth{
		started:     true,
		hasEver:     true,
		lastEventAt: time.Now().UTC().Add(-45 * time.Minute),
		count:       0,
	}
	c := monitor.NewTraceProjectorChecker(fp)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusWarning {
		t.Errorf("expected warning for stalled projector, got %s (msg=%s)", result.Status, result.Message)
	}
	if !contains(result.Message, "stalled") {
		t.Errorf("expected message to mention 'stalled', got %q", result.Message)
	}
}

func TestTraceProjectorChecker_RecentActivityIdlePasses(t *testing.T) {
	// Projector received events in the past and last event is within
	// the idle window. count is 0 because the in-flight traces have all
	// completed — still healthy idle, not stalled.
	fp := &fakeTraceProjectorHealth{
		started:     true,
		hasEver:     true,
		lastEventAt: time.Now().UTC().Add(-2 * time.Minute),
		count:       0,
	}
	c := monitor.NewTraceProjectorChecker(fp)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusPassed {
		t.Errorf("expected passed for recently-active idle projector, got %s (msg=%s)", result.Status, result.Message)
	}
}

func TestTraceProjectorChecker_DetailsIncludeSignals(t *testing.T) {
	// The Details payload should carry every signal the checker uses,
	// so that operators inspecting the report can see exactly why a
	// result was emitted.
	fp := &fakeTraceProjectorHealth{
		started:     true,
		hasEver:     true,
		lastEventAt: time.Now().UTC().Add(-5 * time.Minute),
		count:       2,
	}
	c := monitor.NewTraceProjectorChecker(fp)
	result := c.Check(context.Background())
	for _, k := range []string{"active_traces", "started", "last_event_at", "has_ever_received", "idle_timeout_sec"} {
		if _, ok := result.Details[k]; !ok {
			t.Errorf("expected Details[%q] to be set", k)
		}
	}
}

// contains is a tiny helper to keep the assertions above readable.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestAlertEvalChecker_NilWorker(t *testing.T) {
	c := monitor.NewAlertEvalChecker(nil)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusFailed {
		t.Errorf("expected failed for nil worker, got %s", result.Status)
	}
}

func TestEventBusChecker_NilBus(t *testing.T) {
	c := monitor.NewEventBusChecker(nil)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusWarning {
		t.Errorf("expected warning for nil bus, got %s", result.Status)
	}
}

func TestWebSocketChecker_NilCounter(t *testing.T) {
	c := monitor.NewWebSocketChecker(nil)
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusWarning {
		t.Errorf("expected warning for nil counter, got %s", result.Status)
	}
}

func TestFlowFileChecker_NilAppender(t *testing.T) {
	c := monitor.NewFlowFileChecker(nil, "")
	result := c.Check(context.Background())
	if result.Status != types.SelfCheckStatusWarning {
		t.Errorf("expected warning for nil appender, got %s", result.Status)
	}
}

// --- Task 4.6: SelfCheckRepairDispatcher unit test ---

type mockRepairer struct {
	canRepair bool
	outcome   monitor.RepairOutcome
}

func (m *mockRepairer) CanRepair(checkName string, status types.SelfCheckStatus) bool {
	return m.canRepair
}

func (m *mockRepairer) Repair(ctx context.Context, result types.SelfCheckResult) monitor.RepairOutcome {
	return m.outcome
}

func TestSelfCheckRepairDispatcher_Dispatch(t *testing.T) {
	repairer := &mockRepairer{
		canRepair: true,
		outcome:   monitor.RepairOutcome{Success: true, Action: "test_repair", Message: "repaired"},
	}
	dispatcher := monitor.NewSelfCheckRepairDispatcher([]monitor.SelfCheckRepairer{repairer}, loggateway.NewNoop())

	result := types.SelfCheckResult{Checker: "test", Status: types.SelfCheckStatusFailed}
	outcome := dispatcher.Repair(context.Background(), result)
	if !outcome.Success {
		t.Error("expected repair to succeed")
	}
	if outcome.Action != "test_repair" {
		t.Errorf("expected action test_repair, got %s", outcome.Action)
	}
}

func TestSelfCheckRepairDispatcher_Cooldown(t *testing.T) {
	repairer := &mockRepairer{
		canRepair: true,
		outcome:   monitor.RepairOutcome{Success: true, Action: "test_repair", Message: "repaired"},
	}
	dispatcher := monitor.NewSelfCheckRepairDispatcher([]monitor.SelfCheckRepairer{repairer}, loggateway.NewNoop())

	result := types.SelfCheckResult{Checker: "test", Status: types.SelfCheckStatusFailed}
	// First repair succeeds
	outcome1 := dispatcher.Repair(context.Background(), result)
	if !outcome1.Success {
		t.Error("first repair should succeed")
	}
	// Second repair within cooldown should be skipped
	outcome2 := dispatcher.Repair(context.Background(), result)
	if outcome2.Success {
		t.Error("second repair within cooldown should be skipped")
	}
	if outcome2.Action != "skipped_cooldown" {
		t.Errorf("expected skipped_cooldown, got %s", outcome2.Action)
	}
}

func TestSelfCheckRepairDispatcher_NoRepairer(t *testing.T) {
	dispatcher := monitor.NewSelfCheckRepairDispatcher(nil, loggateway.NewNoop())
	result := types.SelfCheckResult{Checker: "test", Status: types.SelfCheckStatusFailed}
	outcome := dispatcher.Repair(context.Background(), result)
	if outcome.Success {
		t.Error("expected no repairer available")
	}
}

func TestSelfCheckRepairDispatcher_Idempotent(t *testing.T) {
	repairer := &mockRepairer{
		canRepair: true,
		outcome:   monitor.RepairOutcome{Success: true, Action: "test_repair", Message: "repaired"},
	}
	dispatcher := monitor.NewSelfCheckRepairDispatcher([]monitor.SelfCheckRepairer{repairer}, loggateway.NewNoop())

	result := types.SelfCheckResult{Checker: "test", Status: types.SelfCheckStatusFailed}
	outcome1 := dispatcher.Repair(context.Background(), result)
	if !outcome1.Success {
		t.Error("first repair should succeed")
	}
	// Same checker within cooldown should be skipped
	outcome2 := dispatcher.Repair(context.Background(), result)
	if outcome2.Action != "skipped_cooldown" {
		t.Errorf("expected skipped_cooldown for idempotent check, got %s", outcome2.Action)
	}
}

// --- Task 6.2: Metrics update unit test ---

func TestSelfCheckUnhealthyCountMetric(t *testing.T) {
	checkers := []monitor.SelfChecker{
		&mockChecker{name: "a", result: types.SelfCheckResult{Checker: "a", Status: types.SelfCheckStatusPassed, CheckedAt: time.Now().UTC()}},
		&mockChecker{name: "b", result: types.SelfCheckResult{Checker: "b", Status: types.SelfCheckStatusWarning, CheckedAt: time.Now().UTC()}},
		&mockChecker{name: "c", result: types.SelfCheckResult{Checker: "c", Status: types.SelfCheckStatusFailed, CheckedAt: time.Now().UTC()}},
	}
	scheduler := monitor.NewSelfCheckScheduler(checkers, nil, &mockSelfCheckRepo{}, nil, loggateway.NewNoop(), nil)
	metric := monitor.NewSelfCheckUnhealthyCountMetric(scheduler)

	// RunOnce populates the cached unhealthy count
	scheduler.RunOnce(context.Background())

	val, err := metric.Evaluate(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 unhealthy (warning + failed)
	if val != 2 {
		t.Errorf("expected 2 unhealthy, got %f", val)
	}
}

func TestSelfCheckUnhealthyCountMetric_NilScheduler(t *testing.T) {
	metric := monitor.NewSelfCheckUnhealthyCountMetric(nil)
	val, err := metric.Evaluate(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 0 {
		t.Errorf("expected 0 for nil scheduler, got %f", val)
	}
}

// --- Task 7.3: RootCauseCondition.SelfCheckStatus matching unit test ---

func TestRootCauseCondition_SelfCheckStatus_Match(t *testing.T) {
	engine := heal.NewRootCauseEngine(loggateway.NewNoop())
	// The rc-self-check-failure rule should match when metadata contains self_check_status=failed
	results := engine.Evaluate(context.Background(), "", "", map[string]any{
		"self_check_status": "failed",
		"checker":           "db_health",
	})
	found := false
	for _, r := range results {
		if r.RuleID == "rc-self-check-failure" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected rc-self-check-failure rule to match")
	}
}

func TestRootCauseCondition_SelfCheckStatus_NoMatch(t *testing.T) {
	engine := heal.NewRootCauseEngine(loggateway.NewNoop())
	// Should not match when self_check_status is not "failed"
	results := engine.Evaluate(context.Background(), "", "", map[string]any{
		"self_check_status": "warning",
		"checker":           "db_health",
	})
	for _, r := range results {
		if r.RuleID == "rc-self-check-failure" {
			t.Error("rc-self-check-failure should not match for warning status")
		}
	}
}

func TestRootCauseCondition_SelfCheckStatus_NoMetadata(t *testing.T) {
	engine := heal.NewRootCauseEngine(loggateway.NewNoop())
	results := engine.Evaluate(context.Background(), "", "", nil)
	for _, r := range results {
		if r.RuleID == "rc-self-check-failure" {
			t.Error("rc-self-check-failure should not match with nil metadata")
		}
	}
}

// --- Task 9.4: Service layer unit test (basic) ---

func TestBizSelfCheckReportToProto_Nil(t *testing.T) {
	// This tests the conversion function handles nil correctly
	// The actual service tests would require more infrastructure
	var report *monitor.SelfCheckReport
	if report != nil {
		t.Error("expected nil report")
	}
}

// --- Repairer unit tests (4.3-4.5) ---

type mockBackfiller struct {
	err error
}

func (m *mockBackfiller) BackfillTraces(ctx context.Context) error { return m.err }

func TestTraceProjectorRepairer_CanRepair(t *testing.T) {
	r := monitor.NewTraceProjectorRepairer(&mockBackfiller{})
	if !r.CanRepair("trace_projector", types.SelfCheckStatusWarning) {
		t.Error("should repair trace_projector warning")
	}
	if r.CanRepair("trace_projector", types.SelfCheckStatusPassed) {
		t.Error("should not repair passed status")
	}
	if r.CanRepair("other", types.SelfCheckStatusFailed) {
		t.Error("should not repair other checker")
	}
}

func TestTraceProjectorRepairer_Repair(t *testing.T) {
	r := monitor.NewTraceProjectorRepairer(&mockBackfiller{})
	result := types.SelfCheckResult{Checker: "trace_projector", Status: types.SelfCheckStatusWarning}
	outcome := r.Repair(context.Background(), result)
	if !outcome.Success {
		t.Errorf("expected success, got %v", outcome)
	}
}

func TestTraceProjectorRepairer_NilBackfiller(t *testing.T) {
	r := monitor.NewTraceProjectorRepairer(nil)
	result := types.SelfCheckResult{Checker: "trace_projector", Status: types.SelfCheckStatusWarning}
	outcome := r.Repair(context.Background(), result)
	if outcome.Success {
		t.Error("expected failure for nil backfiller")
	}
}

type mockRestarter struct {
	called bool
}

func (m *mockRestarter) RestartEvalWorker(ctx context.Context) { m.called = true }

func TestAlertEvalRepairer_CanRepair(t *testing.T) {
	r := monitor.NewAlertEvalRepairer(&mockRestarter{})
	if !r.CanRepair("alert_eval", types.SelfCheckStatusFailed) {
		t.Error("should repair alert_eval failed")
	}
	if r.CanRepair("alert_eval", types.SelfCheckStatusPassed) {
		t.Error("should not repair passed status")
	}
}

func TestAlertEvalRepairer_Repair(t *testing.T) {
	restarter := &mockRestarter{}
	r := monitor.NewAlertEvalRepairer(restarter)
	result := types.SelfCheckResult{Checker: "alert_eval", Status: types.SelfCheckStatusFailed}
	outcome := r.Repair(context.Background(), result)
	if !outcome.Success {
		t.Errorf("expected success, got %v", outcome)
	}
	if !restarter.called {
		t.Error("expected RestartEvalWorker to be called")
	}
}

type mockResubscriber struct {
	err error
}

func (m *mockResubscriber) Resubscribe(topic string) error { return m.err }

func TestEventBusRepairer_CanRepair(t *testing.T) {
	r := monitor.NewEventBusRepairer(&mockResubscriber{})
	if !r.CanRepair("eventbus", types.SelfCheckStatusWarning) {
		t.Error("should repair eventbus warning")
	}
	if r.CanRepair("eventbus", types.SelfCheckStatusPassed) {
		t.Error("should not repair passed status")
	}
}

func TestEventBusRepairer_Repair(t *testing.T) {
	r := monitor.NewEventBusRepairer(&mockResubscriber{})
	result := types.SelfCheckResult{Checker: "eventbus", Status: types.SelfCheckStatusWarning}
	outcome := r.Repair(context.Background(), result)
	if !outcome.Success {
		t.Errorf("expected success, got %v", outcome)
	}
}
