package monitor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

type mockRepo struct {
	listAuditLogsFn                      func(ctx context.Context, query monitor.AuditQuery) (monitor.AuditListResult, error)
	insertAuditLogFn                     func(ctx context.Context, entry monitor.AuditLog) error
	deleteAuditLogsFn                    func(ctx context.Context) (int, error)
	insertMonitorEventFn                 func(ctx context.Context, ev monitor.EventWrite) error
	listMonitorEventsFn                  func(ctx context.Context, query monitor.EventsQuery) (monitor.ListResult, error)
	getMonitorEventFn                    func(ctx context.Context, id string) (monitor.PlatformRow, error)
	listMonitorTracesFn                  func(ctx context.Context, query monitor.TracesQuery) (monitor.ListResult, error)
	getMonitorTraceFn                    func(ctx context.Context, id string) (monitor.PlatformRow, error)
	listAlertRulesFn                     func(ctx context.Context) ([]monitor.AlertRule, error)
	replaceAlertRulesFn                  func(ctx context.Context, rules []monitor.AlertRule) error
	updateAlertFiringStateFn             func(ctx context.Context, id string, state monitor.AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error
	countMonitorEventsSinceFn            func(ctx context.Context, eventKey, status, sinceRFC3339, untilRFC3339 string) (int32, error)
	avgRunnerCompletionDurationMsSinceFn func(ctx context.Context, sinceRFC3339 string) (float64, error)
	latencyPercentilesSinceFn            func(ctx context.Context, sinceRFC3339 string) (p50, p95, p99 float64, err error)
	existsRunnerCompletionFn             func(ctx context.Context, sessionID, invocationID string) (bool, error)
	patchRunnerCompletionMetadataFn      func(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error)
	insertMonitorTraceFn                 func(ctx context.Context, tw monitor.TraceWrite) error
	upsertMonitorTraceSpanFn             func(ctx context.Context, sw monitor.TraceSpanWrite) error
	updateMonitorTraceCompletionFn       func(ctx context.Context, traceID string, c monitor.TraceCompletion) error
	interruptStaleTracesFn               func(ctx context.Context, olderThan time.Time) (int64, error)
	ensureTraceSchemaFn                  func(ctx context.Context) error
	listRecentRunnerCompletionsFn        func(ctx context.Context, since time.Duration, limit int) ([]monitor.RunnerCompletionRow, error)
}

func (m *mockRepo) ListAuditLogs(ctx context.Context, query monitor.AuditQuery) (monitor.AuditListResult, error) {
	if m.listAuditLogsFn != nil {
		return m.listAuditLogsFn(ctx, query)
	}
	return monitor.AuditListResult{}, nil
}

func (m *mockRepo) InsertAuditLog(ctx context.Context, entry monitor.AuditLog) error {
	if m.insertAuditLogFn != nil {
		return m.insertAuditLogFn(ctx, entry)
	}
	return nil
}

func (m *mockRepo) DeleteAuditLogs(ctx context.Context) (int, error) {
	if m.deleteAuditLogsFn != nil {
		return m.deleteAuditLogsFn(ctx)
	}
	return 0, nil
}

func (m *mockRepo) InsertMonitorEvent(ctx context.Context, ev monitor.EventWrite) error {
	if m.insertMonitorEventFn != nil {
		return m.insertMonitorEventFn(ctx, ev)
	}
	return nil
}

func (m *mockRepo) ListMonitorEvents(ctx context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
	if m.listMonitorEventsFn != nil {
		return m.listMonitorEventsFn(ctx, query)
	}
	return monitor.ListResult{}, nil
}

func (m *mockRepo) GetMonitorEvent(ctx context.Context, id string) (monitor.PlatformRow, error) {
	if m.getMonitorEventFn != nil {
		return m.getMonitorEventFn(ctx, id)
	}
	return monitor.PlatformRow{}, nil
}

func (m *mockRepo) ListMonitorTraces(ctx context.Context, query monitor.TracesQuery) (monitor.ListResult, error) {
	if m.listMonitorTracesFn != nil {
		return m.listMonitorTracesFn(ctx, query)
	}
	return monitor.ListResult{}, nil
}

func (m *mockRepo) GetMonitorTrace(ctx context.Context, id string) (monitor.PlatformRow, error) {
	if m.getMonitorTraceFn != nil {
		return m.getMonitorTraceFn(ctx, id)
	}
	return monitor.PlatformRow{}, nil
}

func (m *mockRepo) ListAlertRules(ctx context.Context) ([]monitor.AlertRule, error) {
	if m.listAlertRulesFn != nil {
		return m.listAlertRulesFn(ctx)
	}
	return nil, nil
}

func (m *mockRepo) ReplaceAlertRules(ctx context.Context, rules []monitor.AlertRule) error {
	if m.replaceAlertRulesFn != nil {
		return m.replaceAlertRulesFn(ctx, rules)
	}
	return nil
}

func (m *mockRepo) UpdateAlertFiringState(ctx context.Context, id string, state monitor.AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error {
	if m.updateAlertFiringStateFn != nil {
		return m.updateAlertFiringStateFn(ctx, id, state, lastFiredAt, lastFiredValue, recoveredAt)
	}
	return nil
}

func (m *mockRepo) CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339, untilRFC3339 string) (int32, error) {
	if m.countMonitorEventsSinceFn != nil {
		return m.countMonitorEventsSinceFn(ctx, eventKey, status, sinceRFC3339, untilRFC3339)
	}
	return 0, nil
}

func (m *mockRepo) AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error) {
	if m.avgRunnerCompletionDurationMsSinceFn != nil {
		return m.avgRunnerCompletionDurationMsSinceFn(ctx, sinceRFC3339)
	}
	return 0, nil
}

func (m *mockRepo) LatencyPercentilesSince(ctx context.Context, sinceRFC3339 string) (float64, float64, float64, error) {
	if m.latencyPercentilesSinceFn != nil {
		return m.latencyPercentilesSinceFn(ctx, sinceRFC3339)
	}
	return 0, 0, 0, nil
}

func (m *mockRepo) ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error) {
	if m.existsRunnerCompletionFn != nil {
		return m.existsRunnerCompletionFn(ctx, sessionID, invocationID)
	}
	return false, nil
}

func (m *mockRepo) PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error) {
	if m.patchRunnerCompletionMetadataFn != nil {
		return m.patchRunnerCompletionMetadataFn(ctx, sessionID, runID, invocationID, patchJSON)
	}
	return false, nil
}

func (m *mockRepo) InsertMonitorTrace(ctx context.Context, tw monitor.TraceWrite) error {
	if m.insertMonitorTraceFn != nil {
		return m.insertMonitorTraceFn(ctx, tw)
	}
	return nil
}

func (m *mockRepo) UpsertMonitorTraceSpan(ctx context.Context, sw monitor.TraceSpanWrite) error {
	if m.upsertMonitorTraceSpanFn != nil {
		return m.upsertMonitorTraceSpanFn(ctx, sw)
	}
	return nil
}

func (m *mockRepo) UpdateMonitorTraceCompletion(ctx context.Context, traceID string, c monitor.TraceCompletion) error {
	if m.updateMonitorTraceCompletionFn != nil {
		return m.updateMonitorTraceCompletionFn(ctx, traceID, c)
	}
	return nil
}

func (m *mockRepo) InterruptStaleTraces(ctx context.Context, olderThan time.Time) (int64, error) {
	if m.interruptStaleTracesFn != nil {
		return m.interruptStaleTracesFn(ctx, olderThan)
	}
	return 0, nil
}

func (m *mockRepo) DeleteMonitorEventsOlderThan(ctx context.Context, olderThan time.Time) (int, error) {
	return 0, nil
}

func (m *mockRepo) EnsureTraceSchema(ctx context.Context) error {
	if m.ensureTraceSchemaFn != nil {
		return m.ensureTraceSchemaFn(ctx)
	}
	return nil
}

func (m *mockRepo) ListRecentRunnerCompletions(ctx context.Context, since time.Duration, limit int) ([]monitor.RunnerCompletionRow, error) {
	if m.listRecentRunnerCompletionsFn != nil {
		return m.listRecentRunnerCompletionsFn(ctx, since, limit)
	}
	return nil, nil
}

type mockNotifier struct {
	notifyFn func(ctx context.Context, rule monitor.AlertRule, payload map[string]any)
}

func (m *mockNotifier) Notify(ctx context.Context, rule monitor.AlertRule, payload map[string]any) {
	if m.notifyFn != nil {
		m.notifyFn(ctx, rule, payload)
	}
}

type mockFsHealth struct {
	fn func(ctx context.Context) (missingCount int, pendingCount int, err error)
}

func (m *mockFsHealth) FilesystemHealthStats(ctx context.Context) (int, int, error) {
	if m.fn != nil {
		return m.fn(ctx)
	}
	return 0, 0, nil
}

type mockRunnerCompletionBridge struct {
	registerTurnUsageFn func(sessionID, runID, usageEventID, traceID, agentID, agentKey string)
	pendingUsageFn      func(sessionID, runID string) (usageEventID, traceID string, ok bool)
	clearTurnFn         func(sessionID, runID string)
}

func (m *mockRunnerCompletionBridge) RegisterTurnUsage(sessionID, runID, usageEventID, traceID, agentID, agentKey string) {
	if m.registerTurnUsageFn != nil {
		m.registerTurnUsageFn(sessionID, runID, usageEventID, traceID, agentID, agentKey)
	}
}

func (m *mockRunnerCompletionBridge) PendingUsage(sessionID, runID string) (string, string, bool) {
	if m.pendingUsageFn != nil {
		return m.pendingUsageFn(sessionID, runID)
	}
	return "", "", false
}

func (m *mockRunnerCompletionBridge) ClearTurn(sessionID, runID string) {
	if m.clearTurnFn != nil {
		m.clearTurnFn(sessionID, runID)
	}
}

func TestRecordAuditLog_Success(t *testing.T) {
	called := false
	repo := &mockRepo{
		insertAuditLogFn: func(_ context.Context, entry monitor.AuditLog) error {
			called = true
			if entry.ID == "" {
				t.Error("ID should be auto-generated when empty")
			}
			if entry.Action != "create" {
				t.Errorf("Action = %q, want %q", entry.Action, "create")
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordAuditLog(context.Background(), monitor.AuditLog{Action: "create"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("InsertAuditLog not called")
	}
}

func TestRecordAuditLog_PreservesExistingID(t *testing.T) {
	repo := &mockRepo{
		insertAuditLogFn: func(_ context.Context, entry monitor.AuditLog) error {
			if entry.ID != "existing-id" {
				t.Errorf("ID = %q, want %q", entry.ID, "existing-id")
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordAuditLog(context.Background(), monitor.AuditLog{ID: "existing-id", Action: "delete"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordAuditLog_RepoError(t *testing.T) {
	repo := &mockRepo{
		insertAuditLogFn: func(context.Context, monitor.AuditLog) error {
			return fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordAuditLog(context.Background(), monitor.AuditLog{Action: "create"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRecordAuditLog_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	err := uc.RecordAuditLog(context.Background(), monitor.AuditLog{Action: "create"})
	if err != nil {
		t.Fatalf("nil usecase should return nil, got: %v", err)
	}
}

func TestRecordAuditLog_NilRepo(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	err := uc.RecordAuditLog(context.Background(), monitor.AuditLog{Action: "create"})
	if err != nil {
		t.Fatalf("nil repo should return nil, got: %v", err)
	}
}

func TestRecordAuditLog_EmptyWhitespaceID(t *testing.T) {
	repo := &mockRepo{
		insertAuditLogFn: func(_ context.Context, entry monitor.AuditLog) error {
			if entry.ID == "" {
				t.Error("whitespace-only ID should be replaced with generated UUID")
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordAuditLog(context.Background(), monitor.AuditLog{ID: "   ", Action: "update"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordMonitorEvent_Success(t *testing.T) {
	called := false
	repo := &mockRepo{
		insertMonitorEventFn: func(_ context.Context, ev monitor.EventWrite) error {
			called = true
			if ev.EventKey != "runner.completion" {
				t.Errorf("EventKey = %q, want %q", ev.EventKey, "runner.completion")
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordMonitorEvent(context.Background(), monitor.EventWrite{EventKey: "runner.completion"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("InsertMonitorEvent not called")
	}
}

func TestRecordMonitorEvent_RepoError(t *testing.T) {
	repo := &mockRepo{
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordMonitorEvent(context.Background(), monitor.EventWrite{EventKey: "runner.completion"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRecordMonitorEvent_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	err := uc.RecordMonitorEvent(context.Background(), monitor.EventWrite{EventKey: "test"})
	if err != nil {
		t.Fatalf("nil usecase should return nil, got: %v", err)
	}
}

func TestRecordMonitorEvent_NilRepo(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	err := uc.RecordMonitorEvent(context.Background(), monitor.EventWrite{EventKey: "test"})
	if err != nil {
		t.Fatalf("nil repo should return nil, got: %v", err)
	}
}

func TestListAuditLogs_Success(t *testing.T) {
	want := monitor.AuditListResult{
		Items: []monitor.AuditLog{{ID: "a1"}, {ID: "a2"}},
		Total: 2,
	}
	repo := &mockRepo{
		listAuditLogsFn: func(_ context.Context, query monitor.AuditQuery) (monitor.AuditListResult, error) {
			return want, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.ListAuditLogs(context.Background(), monitor.AuditQuery{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 2 {
		t.Errorf("Total = %d, want 2", got.Total)
	}
}

func TestListAuditLogs_DefaultLimit(t *testing.T) {
	repo := &mockRepo{
		listAuditLogsFn: func(_ context.Context, query monitor.AuditQuery) (monitor.AuditListResult, error) {
			if query.Limit != 200 {
				t.Errorf("Limit = %d, want default 200", query.Limit)
			}
			return monitor.AuditListResult{}, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.ListAuditLogs(context.Background(), monitor.AuditQuery{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListAuditLogs_NegativeLimit(t *testing.T) {
	repo := &mockRepo{
		listAuditLogsFn: func(_ context.Context, query monitor.AuditQuery) (monitor.AuditListResult, error) {
			if query.Limit != 200 {
				t.Errorf("Limit = %d, want default 200", query.Limit)
			}
			return monitor.AuditListResult{}, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.ListAuditLogs(context.Background(), monitor.AuditQuery{Limit: -5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListAuditLogs_RepoError(t *testing.T) {
	repo := &mockRepo{
		listAuditLogsFn: func(context.Context, monitor.AuditQuery) (monitor.AuditListResult, error) {
			return monitor.AuditListResult{}, fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.ListAuditLogs(context.Background(), monitor.AuditQuery{Limit: 10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListMonitorEvents_Success(t *testing.T) {
	want := monitor.ListResult{
		Items: []monitor.PlatformRow{{ID: "e1"}},
		Total: 1,
	}
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			return want, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.ListMonitorEvents(context.Background(), monitor.EventsQuery{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 1 {
		t.Errorf("Total = %d, want 1", got.Total)
	}
}

func TestListMonitorEvents_DefaultLimit(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
			if query.Limit != 100 {
				t.Errorf("Limit = %d, want default 100", query.Limit)
			}
			return monitor.ListResult{}, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.ListMonitorEvents(context.Background(), monitor.EventsQuery{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListMonitorEvents_RepoError(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(context.Context, monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.ListMonitorEvents(context.Background(), monitor.EventsQuery{Limit: 10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListAlertRules_Success(t *testing.T) {
	rules := []monitor.AlertRule{{ID: "r1"}, {ID: "r2"}}
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return rules, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.ListAlertRules(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestListAlertRules_Empty(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return nil, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.ListAlertRules(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestListAlertRules_RepoError(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.ListAlertRules(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListAlertRules_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	got, err := uc.ListAlertRules(context.Background())
	if err != nil {
		t.Fatalf("nil usecase should return nil, nil, got: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestReplaceAlertRules_Success(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{{ID: "old1"}}, nil
		},
		replaceAlertRulesFn: func(_ context.Context, rules []monitor.AlertRule) error {
			if len(rules) != 1 {
				t.Errorf("len(rules) = %d, want 1", len(rules))
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.ReplaceAlertRules(context.Background(), []monitor.AlertRule{validAlertRule("new1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// validAlertRule 返回通过 ReplaceAlertRules 边界校验的规则（2026-08-11 ISSUE-001）。
func validAlertRule(id string) monitor.AlertRule {
	return monitor.AlertRule{ID: id, Name: "rule-" + id, MetricKey: "runner.error_rate", Threshold: 0.5, WindowMinutes: 5}
}

func TestReplaceAlertRules_ValidationRejectsInvalidRules(t *testing.T) {
	replaceCalled := false
	repo := &mockRepo{
		replaceAlertRulesFn: func(context.Context, []monitor.AlertRule) error {
			replaceCalled = true
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	cases := map[string]monitor.AlertRule{
		"empty name":      {MetricKey: "runner.error_rate", Threshold: 0.5, WindowMinutes: 5},
		"empty metric":    {Name: "r", Threshold: 0.5, WindowMinutes: 5},
		"zero threshold":  {Name: "r", MetricKey: "runner.error_rate", WindowMinutes: 5},
		"zero window":     {Name: "r", MetricKey: "runner.error_rate", Threshold: 0.5},
		"negative window": {Name: "r", MetricKey: "runner.error_rate", Threshold: 0.5, WindowMinutes: -1},
	}
	for name, rule := range cases {
		if err := uc.ReplaceAlertRules(context.Background(), []monitor.AlertRule{rule}); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
	if replaceCalled {
		t.Error("repo ReplaceAlertRules must not be called when validation fails")
	}
}

func TestReplaceAlertRules_DeletesStaleLastFired(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{{ID: "old1"}, {ID: "old2"}}, nil
		},
		replaceAlertRulesFn: func(context.Context, []monitor.AlertRule) error {
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.MarkAlertFired("old1", time.Now())
	uc.MarkAlertFired("old2", time.Now())

	err := uc.ReplaceAlertRules(context.Background(), []monitor.AlertRule{validAlertRule("old1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rule := monitor.AlertRule{ID: "old2", CooldownMinutes: 60}
	if uc.ShouldFireAlert(rule, time.Now()) != true {
		t.Error("old2 lastFired should have been deleted")
	}
}

func TestReplaceAlertRules_RepoError(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return nil, nil
		},
		replaceAlertRulesFn: func(context.Context, []monitor.AlertRule) error {
			return fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.ReplaceAlertRules(context.Background(), []monitor.AlertRule{validAlertRule("r1")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReplaceAlertRules_ListOldRulesError(t *testing.T) {
	replaceCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return nil, fmt.Errorf("list error")
		},
		replaceAlertRulesFn: func(context.Context, []monitor.AlertRule) error {
			replaceCalled = true
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.ReplaceAlertRules(context.Background(), []monitor.AlertRule{validAlertRule("r1")})
	if err != nil {
		t.Fatalf("should not fail when ListAlertRules fails, got: %v", err)
	}
	if !replaceCalled {
		t.Error("ReplaceAlertRules should still be called")
	}
}

func TestReplaceAlertRules_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	err := uc.ReplaceAlertRules(context.Background(), nil)
	if err != nil {
		t.Fatalf("nil usecase should return nil, got: %v", err)
	}
}

func TestReplaceAlertRules_EmptyRules(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{{ID: "old1"}}, nil
		},
		replaceAlertRulesFn: func(_ context.Context, rules []monitor.AlertRule) error {
			if len(rules) != 0 {
				t.Errorf("len(rules) = %d, want 0", len(rules))
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.ReplaceAlertRules(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMonitorEvent_Success(t *testing.T) {
	want := monitor.PlatformRow{ID: "e1", Name: "test-event"}
	repo := &mockRepo{
		getMonitorEventFn: func(_ context.Context, id string) (monitor.PlatformRow, error) {
			if id != "e1" {
				t.Errorf("id = %q, want %q", id, "e1")
			}
			return want, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.GetMonitorEvent(context.Background(), "e1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "e1" {
		t.Errorf("ID = %q, want %q", got.ID, "e1")
	}
}

func TestGetMonitorEvent_RepoError(t *testing.T) {
	repo := &mockRepo{
		getMonitorEventFn: func(context.Context, string) (monitor.PlatformRow, error) {
			return monitor.PlatformRow{}, fmt.Errorf("not found")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.GetMonitorEvent(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListMonitorTraces_Success(t *testing.T) {
	want := monitor.ListResult{
		Items: []monitor.PlatformRow{{ID: "t1"}},
		Total: 1,
	}
	repo := &mockRepo{
		listMonitorTracesFn: func(_ context.Context, query monitor.TracesQuery) (monitor.ListResult, error) {
			return want, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.ListMonitorTraces(context.Background(), monitor.TracesQuery{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 1 {
		t.Errorf("Total = %d, want 1", got.Total)
	}
}

func TestListMonitorTraces_DefaultLimit(t *testing.T) {
	repo := &mockRepo{
		listMonitorTracesFn: func(_ context.Context, query monitor.TracesQuery) (monitor.ListResult, error) {
			if query.Limit != 100 {
				t.Errorf("Limit = %d, want default 100", query.Limit)
			}
			return monitor.ListResult{}, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.ListMonitorTraces(context.Background(), monitor.TracesQuery{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListMonitorTraces_RepoError(t *testing.T) {
	repo := &mockRepo{
		listMonitorTracesFn: func(context.Context, monitor.TracesQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.ListMonitorTraces(context.Background(), monitor.TracesQuery{Limit: 10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetMonitorTrace_Success(t *testing.T) {
	want := monitor.PlatformRow{ID: "t1", Name: "test-trace"}
	repo := &mockRepo{
		getMonitorTraceFn: func(_ context.Context, id string) (monitor.PlatformRow, error) {
			if id != "t1" {
				t.Errorf("id = %q, want %q", id, "t1")
			}
			return want, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.GetMonitorTrace(context.Background(), "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "t1" {
		t.Errorf("ID = %q, want %q", got.ID, "t1")
	}
}

func TestGetMonitorTrace_RepoError(t *testing.T) {
	repo := &mockRepo{
		getMonitorTraceFn: func(context.Context, string) (monitor.PlatformRow, error) {
			return monitor.PlatformRow{}, fmt.Errorf("not found")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.GetMonitorTrace(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetRunnerMetrics_Success(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(_ context.Context, eventKey, status, since, until string) (int32, error) {
			if status == "" {
				return 100, nil
			}
			return 10, nil
		},
		avgRunnerCompletionDurationMsSinceFn: func(_ context.Context, since string) (float64, error) {
			return 250.5, nil
		},
		latencyPercentilesSinceFn: func(_ context.Context, since string) (float64, float64, float64, error) {
			return 200.0, 400.0, 600.0, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.GetRunnerMetrics(context.Background(), 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TotalRuns != 100 {
		t.Errorf("TotalRuns = %d, want 100", got.TotalRuns)
	}
	if got.ErrorRuns != 10 {
		t.Errorf("ErrorRuns = %d, want 10", got.ErrorRuns)
	}
	if got.ErrorRate != 0.1 {
		t.Errorf("ErrorRate = %.4f, want 0.1", got.ErrorRate)
	}
	if got.SuccessRate != 0.9 {
		t.Errorf("SuccessRate = %.4f, want 0.9", got.SuccessRate)
	}
	if got.AvgDurationMs != 250.5 {
		t.Errorf("AvgDurationMs = %.2f, want 250.5", got.AvgDurationMs)
	}
	if got.P50DurationMs != 200.0 {
		t.Errorf("P50DurationMs = %.2f, want 200.0", got.P50DurationMs)
	}
	if got.P95DurationMs != 400.0 {
		t.Errorf("P95DurationMs = %.2f, want 400.0", got.P95DurationMs)
	}
	if got.P99DurationMs != 600.0 {
		t.Errorf("P99DurationMs = %.2f, want 600.0", got.P99DurationMs)
	}
	if got.WindowMinutes != 30 {
		t.Errorf("WindowMinutes = %d, want 30", got.WindowMinutes)
	}
}

func TestGetRunnerMetrics_DefaultWindow(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(_ context.Context, eventKey, status, since, until string) (int32, error) {
			return 0, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.GetRunnerMetrics(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.WindowMinutes != 60 {
		t.Errorf("WindowMinutes = %d, want default 60", got.WindowMinutes)
	}
}

func TestGetRunnerMetrics_NegativeWindow(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 0, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.GetRunnerMetrics(context.Background(), -10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.WindowMinutes != 60 {
		t.Errorf("WindowMinutes = %d, want default 60", got.WindowMinutes)
	}
}

func TestGetRunnerMetrics_ZeroTotalRuns(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 0, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.GetRunnerMetrics(context.Background(), 60)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ErrorRate != 0 {
		t.Errorf("ErrorRate = %.4f, want 0", got.ErrorRate)
	}
	if got.SuccessRate != 0 {
		t.Errorf("SuccessRate = %.4f, want 0", got.SuccessRate)
	}
}

func TestGetRunnerMetrics_CountTotalError(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(_ context.Context, eventKey, status, since, until string) (int32, error) {
			if status == "" {
				return 0, fmt.Errorf("count error")
			}
			return 0, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.GetRunnerMetrics(context.Background(), 60)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetRunnerMetrics_CountErrorsError(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(_ context.Context, eventKey, status, since, until string) (int32, error) {
			if status == "error" {
				return 0, fmt.Errorf("count error")
			}
			return 100, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.GetRunnerMetrics(context.Background(), 60)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetRunnerMetrics_AvgDurationError(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 100, nil
		},
		avgRunnerCompletionDurationMsSinceFn: func(context.Context, string) (float64, error) {
			return 0, fmt.Errorf("avg error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.GetRunnerMetrics(context.Background(), 60)
	if err != nil {
		t.Fatalf("avg error should not fail, got: %v", err)
	}
	if got.AvgDurationMs != 0 {
		t.Errorf("AvgDurationMs = %.2f, want 0 on error", got.AvgDurationMs)
	}
}

func TestGetRunnerMetrics_PercentileError(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 100, nil
		},
		latencyPercentilesSinceFn: func(context.Context, string) (float64, float64, float64, error) {
			return 0, 0, 0, fmt.Errorf("percentile error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	got, err := uc.GetRunnerMetrics(context.Background(), 60)
	if err != nil {
		t.Fatalf("percentile error should not fail, got: %v", err)
	}
	if got.P50DurationMs != 0 || got.P95DurationMs != 0 || got.P99DurationMs != 0 {
		t.Errorf("percentiles should be 0 on error, got p50=%.2f p95=%.2f p99=%.2f", got.P50DurationMs, got.P95DurationMs, got.P99DurationMs)
	}
}

func TestGetRunnerMetrics_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	got, err := uc.GetRunnerMetrics(context.Background(), 60)
	if err != nil {
		t.Fatalf("nil usecase should return zero, nil, got: %v", err)
	}
	if got.WindowMinutes != 60 {
		t.Errorf("WindowMinutes = %d, want 60 (passed through from input)", got.WindowMinutes)
	}
	if got.TotalRuns != 0 {
		t.Errorf("TotalRuns = %d, want 0", got.TotalRuns)
	}
}

func TestRecordRunnerCompletion_NewCompletion(t *testing.T) {
	insertCalled := false
	patchCalled := false
	clearCalled := false

	repo := &mockRepo{
		existsRunnerCompletionFn: func(context.Context, string, string) (bool, error) {
			return false, nil
		},
		insertMonitorEventFn: func(_ context.Context, ev monitor.EventWrite) error {
			insertCalled = true
			if ev.EventKey != "runner.completion" {
				t.Errorf("EventKey = %q, want %q", ev.EventKey, "runner.completion")
			}
			return nil
		},
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			patchCalled = true
			return true, nil
		},
	}
	bridge := &mockRunnerCompletionBridge{
		clearTurnFn: func(string, string) { clearCalled = true },
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordRunnerCompletion(
		context.Background(),
		monitor.EventWrite{EventKey: "runner.completion"},
		monitor.RunnerCompletionLinkParams{
			SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
			UsageEventID: "usage1", TraceID: "trace1", Bridge: bridge,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !insertCalled {
		t.Error("InsertMonitorEvent not called")
	}
	if !patchCalled {
		t.Error("PatchRunnerCompletionMetadata not called")
	}
	if !clearCalled {
		t.Error("ClearTurn not called after patch")
	}
}

func TestRecordRunnerCompletion_ExistingCompletion(t *testing.T) {
	insertCalled := false
	patchCalled := false

	repo := &mockRepo{
		existsRunnerCompletionFn: func(context.Context, string, string) (bool, error) {
			return true, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			insertCalled = true
			return nil
		},
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			patchCalled = true
			return true, nil
		},
	}
	bridge := &mockRunnerCompletionBridge{
		clearTurnFn: func(string, string) {},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordRunnerCompletion(
		context.Background(),
		monitor.EventWrite{EventKey: "runner.completion"},
		monitor.RunnerCompletionLinkParams{
			SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
			UsageEventID: "usage1", TraceID: "trace1", Bridge: bridge,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if insertCalled {
		t.Error("InsertMonitorEvent should not be called for existing completion")
	}
	if !patchCalled {
		t.Error("PatchRunnerCompletionMetadata should be called for existing completion")
	}
}

func TestRecordRunnerCompletion_ExistsCheckError(t *testing.T) {
	repo := &mockRepo{
		existsRunnerCompletionFn: func(context.Context, string, string) (bool, error) {
			return false, fmt.Errorf("exists check error")
		},
	}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordRunnerCompletion(
		context.Background(),
		monitor.EventWrite{EventKey: "runner.completion"},
		monitor.RunnerCompletionLinkParams{
			SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
			Bridge: bridge,
		},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRecordRunnerCompletion_InsertError(t *testing.T) {
	repo := &mockRepo{
		existsRunnerCompletionFn: func(context.Context, string, string) (bool, error) {
			return false, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return fmt.Errorf("insert error")
		},
	}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordRunnerCompletion(
		context.Background(),
		monitor.EventWrite{EventKey: "runner.completion"},
		monitor.RunnerCompletionLinkParams{
			SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
			Bridge: bridge,
		},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRecordRunnerCompletion_PatchError(t *testing.T) {
	repo := &mockRepo{
		existsRunnerCompletionFn: func(context.Context, string, string) (bool, error) {
			return false, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return nil
		},
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			return false, fmt.Errorf("patch error")
		},
	}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordRunnerCompletion(
		context.Background(),
		monitor.EventWrite{EventKey: "runner.completion"},
		monitor.RunnerCompletionLinkParams{
			SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
			UsageEventID: "usage1", TraceID: "trace1", Bridge: bridge,
		},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRecordRunnerCompletion_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	err := uc.RecordRunnerCompletion(
		context.Background(),
		monitor.EventWrite{EventKey: "runner.completion"},
		monitor.RunnerCompletionLinkParams{
			SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
		},
	)
	if err != nil {
		t.Fatalf("nil usecase should return nil, got: %v", err)
	}
}

func TestRecordRunnerCompletion_EmptySessionID(t *testing.T) {
	insertCalled := false
	repo := &mockRepo{
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			insertCalled = true
			return nil
		},
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			return false, nil
		},
	}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordRunnerCompletion(
		context.Background(),
		monitor.EventWrite{EventKey: "runner.completion"},
		monitor.RunnerCompletionLinkParams{
			RunID: "run1", InvocationID: "inv1", Bridge: bridge,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !insertCalled {
		t.Error("InsertMonitorEvent should be called when sessionID is empty (skip exists check)")
	}
}

func TestRecordRunnerCompletion_UsageEventIDClearsTurn(t *testing.T) {
	clearCalled := false
	repo := &mockRepo{
		existsRunnerCompletionFn: func(context.Context, string, string) (bool, error) {
			return false, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return nil
		},
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			return false, nil
		},
	}
	bridge := &mockRunnerCompletionBridge{
		clearTurnFn: func(string, string) { clearCalled = true },
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.RecordRunnerCompletion(
		context.Background(),
		monitor.EventWrite{EventKey: "runner.completion"},
		monitor.RunnerCompletionLinkParams{
			SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
			UsageEventID: "usage1", Bridge: bridge,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !clearCalled {
		t.Error("ClearTurn should be called when usageEventID is non-empty even if patch returns false")
	}
}

func TestEvaluateAlerts_RunnerErrorRateFires(t *testing.T) {
	notifyCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					Name:            "High Error Rate",
					MetricKey:       "runner.error_rate",
					Threshold:       0.5,
					WindowMinutes:   60,
					Enabled:         true,
					Severity:        "critical",
					CooldownMinutes: 60,
				},
			}, nil
		},
		countMonitorEventsSinceFn: func(_ context.Context, eventKey, status, since, until string) (int32, error) {
			if status == "" {
				return 100, nil
			}
			return 80, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return nil
		},
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(_ context.Context, rule monitor.AlertRule, payload map[string]any) {
			notifyCalled = true
			if rule.ID != "r1" {
				t.Errorf("rule ID = %q, want %q", rule.ID, "r1")
			}
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier)
	uc.EvaluateAlerts(context.Background())
	if !notifyCalled {
		t.Error("Notify should be called when error rate exceeds threshold")
	}
}

func TestEvaluateAlerts_RunnerErrorRateBelowThreshold(t *testing.T) {
	notifyCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "runner.error_rate",
					Threshold:       0.5,
					WindowMinutes:   60,
					Enabled:         true,
					CooldownMinutes: 60,
				},
			}, nil
		},
		countMonitorEventsSinceFn: func(_ context.Context, eventKey, status, since, until string) (int32, error) {
			if status == "" {
				return 100, nil
			}
			return 10, nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {
			notifyCalled = true
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier)
	uc.EvaluateAlerts(context.Background())
	if notifyCalled {
		t.Error("Notify should not be called when error rate is below threshold")
	}
}

func TestEvaluateAlerts_DisabledRule(t *testing.T) {
	notifyCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:        "r1",
					MetricKey: "runner.error_rate",
					Threshold: 0.5,
					Enabled:   false,
				},
			}, nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {
			notifyCalled = true
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier)
	uc.EvaluateAlerts(context.Background())
	if notifyCalled {
		t.Error("Disabled rule should not trigger notification")
	}
}

func TestEvaluateAlerts_ZeroTotalDoesNotAutoRecover(t *testing.T) {
	// Empty data window provides no evidence for recovery; state must persist.
	recoveredCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "runner.error_rate",
					Threshold:       0.5,
					WindowMinutes:   60,
					Enabled:         true,
					CooldownMinutes: 60,
					FiringState:     monitor.AlertFiringStateFiring,
				},
			}, nil
		},
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 0, nil
		},
		updateAlertFiringStateFn: func(_ context.Context, id string, state monitor.AlertFiringState, _ *time.Time, _ float64, _ *time.Time) error {
			if state == monitor.AlertFiringStateRecovered {
				recoveredCalled = true
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.EvaluateAlerts(context.Background())
	if recoveredCalled {
		t.Error("Firing alert should NOT auto-recover when total=0 (empty window is ambiguous)")
	}
}

func TestEvaluateAlerts_RegistryNoDataDoesNotAutoRecover(t *testing.T) {
	// Registry path: empty data window signals NoData; the state machine must
	// not interpret it as value=0 and falsely recover a firing alert.
	recoveredCalled := false
	notifyCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "runner.error_rate",
					Threshold:       0.5,
					WindowMinutes:   60,
					Enabled:         true,
					CooldownMinutes: 60,
					FiringState:     monitor.AlertFiringStateFiring,
				},
			}, nil
		},
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 0, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return nil
		},
		updateAlertFiringStateFn: func(_ context.Context, _ string, state monitor.AlertFiringState, _ *time.Time, _ float64, _ *time.Time) error {
			if state == monitor.AlertFiringStateRecovered {
				recoveredCalled = true
			}
			return nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {
			notifyCalled = true
		},
	}
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(monitor.NewRunnerErrorRateMetric(repo, nil))
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier, monitor.WithRegistry(reg))
	uc.EvaluateAlerts(context.Background())
	if recoveredCalled {
		t.Error("Firing alert should NOT auto-recover when metric reports NoData")
	}
	if notifyCalled {
		t.Error("Notify should not be called when metric reports NoData")
	}
}

func TestEvaluateAlerts_RegistryMetric(t *testing.T) {
	notifyCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "custom.metric",
					Threshold:       50,
					WindowMinutes:   60,
					Enabled:         true,
					Severity:        "warning",
					CooldownMinutes: 60,
				},
			}, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return nil
		},
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {
			notifyCalled = true
		},
	}
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(&stubAlertMetric{keyVal: "custom.metric", evalFn: func(context.Context, time.Duration) (float64, error) { return 80.0, nil }})
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier, monitor.WithRegistry(reg))
	uc.EvaluateAlerts(context.Background())
	if !notifyCalled {
		t.Error("Registry metric above threshold should fire alert")
	}
}

func TestEvaluateAlerts_RegistryMetricBelowThreshold(t *testing.T) {
	notifyCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "custom.metric",
					Threshold:       50,
					WindowMinutes:   60,
					Enabled:         true,
					CooldownMinutes: 60,
				},
			}, nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {
			notifyCalled = true
		},
	}
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(&stubAlertMetric{keyVal: "custom.metric", evalFn: func(context.Context, time.Duration) (float64, error) { return 30.0, nil }})
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier, monitor.WithRegistry(reg))
	uc.EvaluateAlerts(context.Background())
	if notifyCalled {
		t.Error("Registry metric below threshold should not fire alert")
	}
}

func TestEvaluateAlerts_RegistryMetricRecovers(t *testing.T) {
	recoveredEvent := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "custom.metric",
					Threshold:       50,
					WindowMinutes:   60,
					Enabled:         true,
					CooldownMinutes: 60,
					FiringState:     monitor.AlertFiringStateFiring,
					RecoveryFactor:  0.9,
				},
			}, nil
		},
		insertMonitorEventFn: func(_ context.Context, ev monitor.EventWrite) error {
			if ev.EventKey == "alert.recovered" {
				recoveredEvent = true
			}
			return nil
		},
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {},
	}
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(&stubAlertMetric{keyVal: "custom.metric", evalFn: func(context.Context, time.Duration) (float64, error) { return 40.0, nil }})
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier, monitor.WithRegistry(reg))
	uc.EvaluateAlerts(context.Background())
	if !recoveredEvent {
		t.Error("Firing alert with metric below recovery threshold should recover")
	}
}

func TestEvaluateAlerts_SkillFilesystemMissingCount(t *testing.T) {
	notifyCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "skill.filesystem_missing_count",
					Threshold:       5,
					WindowMinutes:   60,
					Enabled:         true,
					Severity:        "warning",
					CooldownMinutes: 60,
				},
			}, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return nil
		},
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {
			notifyCalled = true
		},
	}
	fsHealth := &mockFsHealth{
		fn: func(context.Context) (int, int, error) {
			return 10, 0, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier, monitor.WithFilesystemHealthReader(fsHealth))
	uc.EvaluateAlerts(context.Background())
	if !notifyCalled {
		t.Error("Missing count above threshold should fire alert")
	}
}

func TestEvaluateAlerts_SkillFilesystemNilHealth(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "skill.filesystem_missing_count",
					Threshold:       5,
					WindowMinutes:   60,
					Enabled:         true,
					CooldownMinutes: 60,
				},
			}, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.EvaluateAlerts(context.Background())
}

func TestEvaluateAlerts_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	uc.EvaluateAlerts(context.Background())
}

func TestEvaluateAlerts_EmptyRules(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return nil, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.EvaluateAlerts(context.Background())
}

func TestEvaluateAlerts_ListRulesError(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.EvaluateAlerts(context.Background())
}

func TestEvaluateAlerts_CountTotalError(t *testing.T) {
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "runner.error_rate",
					Threshold:       0.5,
					WindowMinutes:   60,
					Enabled:         true,
					CooldownMinutes: 60,
				},
			}, nil
		},
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 0, fmt.Errorf("count error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.EvaluateAlerts(context.Background())
}

func TestMarkAlertFiredPersistent_Success(t *testing.T) {
	updateCalled := false
	repo := &mockRepo{
		updateAlertFiringStateFn: func(_ context.Context, id string, state monitor.AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error {
			updateCalled = true
			if id != "r1" {
				t.Errorf("id = %q, want %q", id, "r1")
			}
			if state != monitor.AlertFiringStateFiring {
				t.Errorf("state = %q, want %q", state, monitor.AlertFiringStateFiring)
			}
			if lastFiredValue != 0.8 {
				t.Errorf("lastFiredValue = %.2f, want 0.8", lastFiredValue)
			}
			if recoveredAt != nil {
				t.Error("recoveredAt should be nil for firing")
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	now := time.Now().UTC()
	uc.MarkAlertFiredPersistent(context.Background(), monitor.AlertRule{ID: "r1"}, now, 0.8)
	if !updateCalled {
		t.Error("UpdateAlertFiringState not called")
	}
}

func TestMarkAlertFiredPersistent_RepoError(t *testing.T) {
	repo := &mockRepo{
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.MarkAlertFiredPersistent(context.Background(), monitor.AlertRule{ID: "r1"}, time.Now(), 0.8)
}

func TestMarkAlertFiredPersistent_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	uc.MarkAlertFiredPersistent(context.Background(), monitor.AlertRule{ID: "r1"}, time.Now(), 0.8)
}

func TestMarkAlertFiredPersistent_NilRepo(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	uc.MarkAlertFiredPersistent(context.Background(), monitor.AlertRule{ID: "r1"}, time.Now(), 0.8)
}

func TestMarkAlertRecovered_Success(t *testing.T) {
	updateCalled := false
	repo := &mockRepo{
		updateAlertFiringStateFn: func(_ context.Context, id string, state monitor.AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error {
			updateCalled = true
			if id != "r1" {
				t.Errorf("id = %q, want %q", id, "r1")
			}
			if state != monitor.AlertFiringStateRecovered {
				t.Errorf("state = %q, want %q", state, monitor.AlertFiringStateRecovered)
			}
			if recoveredAt == nil {
				t.Error("recoveredAt should not be nil")
			}
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	now := time.Now().UTC()
	uc.MarkAlertRecovered(context.Background(), monitor.AlertRule{ID: "r1", FiringState: monitor.AlertFiringStateFiring}, now)
	if !updateCalled {
		t.Error("UpdateAlertFiringState not called")
	}
}

func TestMarkAlertRecovered_RepoError(t *testing.T) {
	repo := &mockRepo{
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return fmt.Errorf("db error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.MarkAlertRecovered(context.Background(), monitor.AlertRule{ID: "r1", FiringState: monitor.AlertFiringStateFiring}, time.Now())
}

func TestMarkAlertRecovered_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	uc.MarkAlertRecovered(context.Background(), monitor.AlertRule{ID: "r1", FiringState: monitor.AlertFiringStateFiring}, time.Now())
}

func TestMarkAlertRecovered_NilRepo(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	uc.MarkAlertRecovered(context.Background(), monitor.AlertRule{ID: "r1", FiringState: monitor.AlertFiringStateFiring}, time.Now())
}

func TestRebuildRingBuffer_Success(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(_ context.Context, eventKey, status, since, until string) (int32, error) {
			if status == "" {
				return 10, nil
			}
			return 2, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	rb := monitor.NewMetricRingBuffer()
	rebuilt := uc.RebuildRingBuffer(context.Background(), rb)
	if rebuilt <= 0 {
		t.Errorf("rebuilt = %d, want > 0", rebuilt)
	}
}

func TestRebuildRingBuffer_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	rebuilt := uc.RebuildRingBuffer(context.Background(), monitor.NewMetricRingBuffer())
	if rebuilt != 0 {
		t.Errorf("rebuilt = %d, want 0", rebuilt)
	}
}

func TestRebuildRingBuffer_NilBuffer(t *testing.T) {
	repo := &mockRepo{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	rebuilt := uc.RebuildRingBuffer(context.Background(), nil)
	if rebuilt != 0 {
		t.Errorf("rebuilt = %d, want 0", rebuilt)
	}
}

func TestRebuildRingBuffer_NilRepo(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	rebuilt := uc.RebuildRingBuffer(context.Background(), monitor.NewMetricRingBuffer())
	if rebuilt != 0 {
		t.Errorf("rebuilt = %d, want 0", rebuilt)
	}
}

func TestRebuildRingBuffer_CountError(t *testing.T) {
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 0, fmt.Errorf("count error")
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	rb := monitor.NewMetricRingBuffer()
	rebuilt := uc.RebuildRingBuffer(context.Background(), rb)
	if rebuilt != 0 {
		t.Errorf("rebuilt = %d, want 0 when all counts fail", rebuilt)
	}
}

func TestRebuildRingBuffer_PartialError(t *testing.T) {
	callCount := 0
	repo := &mockRepo{
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			callCount++
			if callCount%4 == 0 {
				return 0, fmt.Errorf("intermittent error")
			}
			return 5, nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	rb := monitor.NewMetricRingBuffer()
	rebuilt := uc.RebuildRingBuffer(context.Background(), rb)
	if rebuilt == 0 {
		t.Error("expected some buckets to be rebuilt despite partial errors")
	}
}

func TestLinkRunnerCompletionUsage_Success(t *testing.T) {
	patchCalled := false
	clearCalled := false
	registerCalled := false

	repo := &mockRepo{
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			patchCalled = true
			return true, nil
		},
	}
	bridge := &mockRunnerCompletionBridge{
		registerTurnUsageFn: func(string, string, string, string, string, string) {
			registerCalled = true
		},
		clearTurnFn: func(string, string) {
			clearCalled = true
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.LinkRunnerCompletionUsage(context.Background(), monitor.RunnerCompletionLinkParams{
		SessionID: "sess1", RunID: "run1", UsageEventID: "usage1", TraceID: "trace1", Bridge: bridge,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !registerCalled {
		t.Error("RegisterTurnUsage should be called")
	}
	if !patchCalled {
		t.Error("PatchRunnerCompletionLink should be called")
	}
	if !clearCalled {
		t.Error("ClearTurn should be called after successful patch")
	}
}

func TestLinkRunnerCompletionUsage_EmptySessionID(t *testing.T) {
	repo := &mockRepo{}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.LinkRunnerCompletionUsage(context.Background(), monitor.RunnerCompletionLinkParams{
		RunID: "run1", UsageEventID: "usage1", TraceID: "trace1", Bridge: bridge,
	})
	if err != nil {
		t.Fatalf("empty sessionID should return nil, got: %v", err)
	}
}

func TestLinkRunnerCompletionUsage_EmptyRunID(t *testing.T) {
	repo := &mockRepo{}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.LinkRunnerCompletionUsage(context.Background(), monitor.RunnerCompletionLinkParams{
		SessionID: "sess1", UsageEventID: "usage1", TraceID: "trace1", Bridge: bridge,
	})
	if err != nil {
		t.Fatalf("empty runID should return nil, got: %v", err)
	}
}

func TestLinkRunnerCompletionUsage_EmptyUsageEventID(t *testing.T) {
	repo := &mockRepo{}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	err := uc.LinkRunnerCompletionUsage(context.Background(), monitor.RunnerCompletionLinkParams{
		SessionID: "sess1", RunID: "run1", TraceID: "trace1", Bridge: bridge,
	})
	if err != nil {
		t.Fatalf("empty usageEventID should return nil, got: %v", err)
	}
}

func TestLinkRunnerCompletionUsage_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	err := uc.LinkRunnerCompletionUsage(context.Background(), monitor.RunnerCompletionLinkParams{
		SessionID: "sess1", RunID: "run1", UsageEventID: "usage1",
	})
	if err != nil {
		t.Fatalf("nil usecase should return nil, got: %v", err)
	}
}

func TestPatchRunnerCompletionLink_WithUsageEventID(t *testing.T) {
	patchCalled := false
	repo := &mockRepo{
		patchRunnerCompletionMetadataFn: func(_ context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error) {
			patchCalled = true
			if sessionID != "sess1" {
				t.Errorf("sessionID = %q, want %q", sessionID, "sess1")
			}
			return true, nil
		},
	}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	patched, err := uc.PatchRunnerCompletionLink(context.Background(), monitor.RunnerCompletionLinkParams{
		SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
		UsageEventID: "usage1", TraceID: "trace1", Bridge: bridge,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !patched {
		t.Error("expected patched = true")
	}
	if !patchCalled {
		t.Error("PatchRunnerCompletionMetadata not called")
	}
}

func TestPatchRunnerCompletionLink_NoUsageEventID_BridgePending(t *testing.T) {
	patchCalled := false
	repo := &mockRepo{
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			patchCalled = true
			return true, nil
		},
	}
	bridge := &mockRunnerCompletionBridge{
		pendingUsageFn: func(sessionID, runID string) (string, string, bool) {
			return "bridge-usage", "bridge-trace", true
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	patched, err := uc.PatchRunnerCompletionLink(context.Background(), monitor.RunnerCompletionLinkParams{
		SessionID: "sess1", RunID: "run1", InvocationID: "inv1", Bridge: bridge,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !patched {
		t.Error("expected patched = true")
	}
	if !patchCalled {
		t.Error("PatchRunnerCompletionMetadata should be called with bridge usage")
	}
}

func TestPatchRunnerCompletionLink_NoUsageEventID_NoBridge(t *testing.T) {
	repo := &mockRepo{}
	bridge := &mockRunnerCompletionBridge{
		pendingUsageFn: func(string, string) (string, string, bool) {
			return "", "", false
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	patched, err := uc.PatchRunnerCompletionLink(context.Background(), monitor.RunnerCompletionLinkParams{
		SessionID: "sess1", RunID: "run1", InvocationID: "inv1", Bridge: bridge,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patched {
		t.Error("expected patched = false when no usage event ID")
	}
}

func TestPatchRunnerCompletionLink_PatchError(t *testing.T) {
	repo := &mockRepo{
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			return false, fmt.Errorf("patch error")
		},
	}
	bridge := &mockRunnerCompletionBridge{}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	_, err := uc.PatchRunnerCompletionLink(context.Background(), monitor.RunnerCompletionLinkParams{
		SessionID: "sess1", RunID: "run1", InvocationID: "inv1",
		UsageEventID: "usage1", Bridge: bridge,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEvaluateAlerts_WithRingBuffer(t *testing.T) {
	notifyCalled := false
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "runner.error_rate",
					Threshold:       0.5,
					WindowMinutes:   60,
					Enabled:         true,
					Severity:        "critical",
					CooldownMinutes: 60,
				},
			}, nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error {
			return nil
		},
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {
			notifyCalled = true
		},
	}
	rb := monitor.NewMetricRingBuffer()
	rb.RecordCompletion("error", 100)
	rb.RecordCompletion("error", 200)
	rb.RecordCompletion("success", 300)
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier, monitor.WithRingBuffer(rb))
	uc.EvaluateAlerts(context.Background())
	if !notifyCalled {
		t.Error("Ring buffer showing 2/3 error rate should fire alert above 0.5 threshold")
	}
}

func TestEvaluateAlerts_CooldownPreventsFire(t *testing.T) {
	notifyCalled := false
	lastFired := time.Now().Add(-5 * time.Minute)
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{
				{
					ID:              "r1",
					MetricKey:       "runner.error_rate",
					Threshold:       0.5,
					WindowMinutes:   60,
					Enabled:         true,
					CooldownMinutes: 60,
					LastFiredAt:     &lastFired,
				},
			}, nil
		},
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 100, nil
		},
	}
	notifier := &mockNotifier{
		notifyFn: func(context.Context, monitor.AlertRule, map[string]any) {
			notifyCalled = true
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier)
	uc.EvaluateAlerts(context.Background())
	if notifyCalled {
		t.Error("Alert within cooldown should not fire")
	}
}

func TestEvaluateAlerts_FiringStateUsesReminderNotSpam(t *testing.T) {
	notifyCount := 0
	lastFired := time.Now().Add(-5 * time.Minute) // within default 30m reminder window
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{{
				ID: "r1", MetricKey: "runner.error_rate", Threshold: 0.1,
				WindowMinutes: 60, Enabled: true, CooldownMinutes: 1,
				FiringState: monitor.AlertFiringStateFiring, LastFiredAt: &lastFired,
			}}, nil
		},
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 10, nil
		},
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return nil
		},
	}
	notifier := &mockNotifier{notifyFn: func(context.Context, monitor.AlertRule, map[string]any) { notifyCount++ }}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier)
	uc.EvaluateAlerts(context.Background())
	if notifyCount != 0 {
		t.Fatalf("firing within reminder window should not re-notify, got %d", notifyCount)
	}
}

func TestEvaluateAlerts_FiringStateReminderAfterInterval(t *testing.T) {
	notifyCount := 0
	lastFired := time.Now().Add(-35 * time.Minute)
	repo := &mockRepo{
		listAlertRulesFn: func(context.Context) ([]monitor.AlertRule, error) {
			return []monitor.AlertRule{{
				ID: "r1", MetricKey: "runner.error_rate", Threshold: 0.1,
				WindowMinutes: 60, Enabled: true, ReminderMinutes: 30,
				FiringState: monitor.AlertFiringStateFiring, LastFiredAt: &lastFired,
			}}, nil
		},
		countMonitorEventsSinceFn: func(context.Context, string, string, string, string) (int32, error) {
			return 10, nil
		},
		updateAlertFiringStateFn: func(context.Context, string, monitor.AlertFiringState, *time.Time, float64, *time.Time) error {
			return nil
		},
		insertMonitorEventFn: func(context.Context, monitor.EventWrite) error { return nil },
	}
	notifier := &mockNotifier{notifyFn: func(context.Context, monitor.AlertRule, map[string]any) { notifyCount++ }}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, notifier)
	uc.EvaluateAlerts(context.Background())
	if notifyCount != 1 {
		t.Fatalf("firing past reminder interval should notify once, got %d", notifyCount)
	}
}

func TestRecordRunnerCompletion_FeedsEvalWorker(t *testing.T) {
	repo := &mockRepo{
		existsRunnerCompletionFn: func(context.Context, string, string) (bool, error) { return false, nil },
		insertMonitorEventFn:     func(context.Context, monitor.EventWrite) error { return nil },
		patchRunnerCompletionMetadataFn: func(context.Context, string, string, string, string) (bool, error) {
			return false, nil
		},
	}
	rb := monitor.NewMetricRingBuffer()
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil, monitor.WithRingBuffer(rb))
	w := monitor.NewAlertEvalWorker(uc, rb, loggateway.NewNoop())
	uc.SetEvalWorker(w)
	err := uc.RecordRunnerCompletion(context.Background(),
		monitor.EventWrite{EventKey: "runner.completion", Status: "error"},
		monitor.RunnerCompletionLinkParams{
			SessionID: "s1", RunID: "r1", InvocationID: "i1", TraceID: "t1",
			Status: "error", DurationMs: 120, Bridge: &mockRunnerCompletionBridge{},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wr := rb.SumLastN(60)
	if wr.Total != 1 || wr.Errors != 1 {
		t.Fatalf("ring buffer not fed: total=%d errors=%d", wr.Total, wr.Errors)
	}
}
