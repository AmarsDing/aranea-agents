package biz

import (
	"context"
	"sync"
	"testing"
	"time"
)

type alertNotifySpy struct {
	mu    sync.Mutex
	calls int
}

func (s *alertNotifySpy) Notify(ctx context.Context, rule MonitorAlertRule, payload map[string]any) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
}

func (s *alertNotifySpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type alertMonitorRepo struct {
	total  int32
	errors int32
	rules  []MonitorAlertRule
}

func (r *alertMonitorRepo) ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error) {
	return AuditListResult{}, nil
}

func (r *alertMonitorRepo) InsertAuditLog(ctx context.Context, entry AuditLog) error {
	return nil
}

func (r *alertMonitorRepo) InsertMonitorEvent(ctx context.Context, ev MonitorEventWrite) error {
	return nil
}

func (r *alertMonitorRepo) ListMonitorEvents(ctx context.Context, query MonitorEventsQuery) (MonitorListResult, error) {
	return MonitorListResult{}, nil
}

func (r *alertMonitorRepo) GetMonitorEvent(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return MonitorPlatformRow{}, nil
}

func (r *alertMonitorRepo) ListMonitorTraces(ctx context.Context, query MonitorTracesQuery) (MonitorListResult, error) {
	return MonitorListResult{}, nil
}

func (r *alertMonitorRepo) GetMonitorTrace(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return MonitorPlatformRow{}, nil
}

func (r *alertMonitorRepo) ListAlertRules(ctx context.Context) ([]MonitorAlertRule, error) {
	if len(r.rules) > 0 {
		return r.rules, nil
	}
	return []MonitorAlertRule{{
		ID: "r1", Name: "test", MetricKey: "runner.error_rate",
		Threshold: 0.5, WindowMinutes: 60, Enabled: true, CooldownMinutes: 60,
	}}, nil
}

func (r *alertMonitorRepo) ReplaceAlertRules(ctx context.Context, rules []MonitorAlertRule) error {
	r.rules = append([]MonitorAlertRule(nil), rules...)
	return nil
}

func (r *alertMonitorRepo) UpdateAlertFiringState(_ context.Context, _ string, _ MonitorAlertFiringState, _ *time.Time, _ float64, _ *time.Time) error {
	return nil
}

func (r *alertMonitorRepo) CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339 string) (int32, error) {
	if status == "error" {
		return r.errors, nil
	}
	return r.total, nil
}

func (r *alertMonitorRepo) AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error) {
	return 0, nil
}

func (r *alertMonitorRepo) ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error) {
	return false, nil
}

func (r *alertMonitorRepo) PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error) {
	return false, nil
}

func (r *alertMonitorRepo) EnsureTraceSchema(context.Context) error { return nil }

func TestEvaluateAlerts_cooldownSuppressesRepeatFire(t *testing.T) {
	repo := &alertMonitorRepo{total: 10, errors: 8}
	spy := &alertNotifySpy{}
	uc := NewMonitorUsecase(repo, spy)
	ctx := context.Background()

	uc.EvaluateAlerts(ctx)
	if spy.count() != 1 {
		t.Fatalf("expected 1 notify, got %d", spy.count())
	}
	uc.EvaluateAlerts(ctx)
	if spy.count() != 1 {
		t.Fatalf("expected cooldown to suppress second notify, got %d", spy.count())
	}
}

func TestEvaluateAlerts_skillFilesystemMissingCount(t *testing.T) {
	repo := &alertMonitorRepo{}
	spy := &alertNotifySpy{}
	uc := NewMonitorUsecase(repo, spy)
	uc.SetFilesystemHealthReader(filesystemHealthStub{missing: 3})
	ctx := context.Background()

	rules := []MonitorAlertRule{{
		ID: "fs1", Name: "skill missing", MetricKey: "skill.filesystem_missing_count",
		Threshold: 1, Enabled: true, CooldownMinutes: 60, Severity: "warn",
	}}
	repo.rules = rules
	uc.EvaluateAlerts(ctx)
	if spy.count() != 1 {
		t.Fatalf("expected 1 notify, got %d", spy.count())
	}
}

type filesystemHealthStub struct {
	missing int
	pending int
}

func (s filesystemHealthStub) FilesystemHealthStats(_ context.Context) (int, int, error) {
	return s.missing, s.pending, nil
}

func TestShouldFireAlert_respectsCooldown(t *testing.T) {
	uc := NewMonitorUsecase(nil, nil)
	rule := MonitorAlertRule{ID: "x", CooldownMinutes: 30}
	now := time.Now()
	if !uc.ShouldFireAlert(rule, now) {
		t.Fatal("first fire should be allowed")
	}
	uc.MarkAlertFired(rule.ID, now)
	if uc.ShouldFireAlert(rule, now.Add(5*time.Minute)) {
		t.Fatal("should be in cooldown")
	}
	if !uc.ShouldFireAlert(rule, now.Add(31*time.Minute)) {
		t.Fatal("cooldown should expire")
	}
}
