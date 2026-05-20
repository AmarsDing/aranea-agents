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
	return []MonitorAlertRule{{
		ID: "r1", Name: "test", MetricKey: "runner.error_rate",
		Threshold: 0.5, WindowMinutes: 60, Enabled: true, CooldownMinutes: 60,
	}}, nil
}

func (r *alertMonitorRepo) ReplaceAlertRules(ctx context.Context, rules []MonitorAlertRule) error {
	return nil
}

func (r *alertMonitorRepo) CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339 string) (int32, error) {
	if status == "error" {
		return r.errors, nil
	}
	return r.total, nil
}

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

func TestShouldFireAlert_respectsCooldown(t *testing.T) {
	uc := NewMonitorUsecase(nil, nil)
	rule := MonitorAlertRule{ID: "x", CooldownMinutes: 30}
	now := time.Now()
	if !uc.shouldFireAlert(rule, now) {
		t.Fatal("first fire should be allowed")
	}
	uc.markAlertFired(rule.ID, now)
	if uc.shouldFireAlert(rule, now.Add(5*time.Minute)) {
		t.Fatal("should be in cooldown")
	}
	if !uc.shouldFireAlert(rule, now.Add(31*time.Minute)) {
		t.Fatal("cooldown should expire")
	}
}
