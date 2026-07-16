package biz

import (
	"context"
	"testing"
	"time"
)

type feedbackMonitorRepo struct {
	events []MonitorEventWrite
}

func (r *feedbackMonitorRepo) InsertMonitorEvent(_ context.Context, ev MonitorEventWrite) error {
	r.events = append(r.events, ev)
	return nil
}

func (r *feedbackMonitorRepo) ListAuditLogs(context.Context, AuditQuery) (AuditListResult, error) {
	return AuditListResult{}, nil
}
func (r *feedbackMonitorRepo) InsertAuditLog(context.Context, AuditLog) error { return nil }
func (r *feedbackMonitorRepo) ListMonitorEvents(context.Context, MonitorEventsQuery) (MonitorListResult, error) {
	return MonitorListResult{}, nil
}
func (r *feedbackMonitorRepo) GetMonitorEvent(context.Context, string) (MonitorPlatformRow, error) {
	return MonitorPlatformRow{}, nil
}
func (r *feedbackMonitorRepo) ListMonitorTraces(context.Context, MonitorTracesQuery) (MonitorListResult, error) {
	return MonitorListResult{}, nil
}
func (r *feedbackMonitorRepo) GetMonitorTrace(context.Context, string) (MonitorPlatformRow, error) {
	return MonitorPlatformRow{}, nil
}
func (r *feedbackMonitorRepo) ListAlertRules(context.Context) ([]MonitorAlertRule, error) {
	return nil, nil
}
func (r *feedbackMonitorRepo) ReplaceAlertRules(context.Context, []MonitorAlertRule) error {
	return nil
}
func (r *feedbackMonitorRepo) UpdateAlertFiringState(context.Context, string, MonitorAlertFiringState, *time.Time, float64, *time.Time) error {
	return nil
}
func (r *feedbackMonitorRepo) CountMonitorEventsSince(context.Context, string, string, string, string) (int32, error) {
	return 0, nil
}
func (r *feedbackMonitorRepo) AvgRunnerCompletionDurationMsSince(context.Context, string) (float64, error) {
	return 0, nil
}
func (r *feedbackMonitorRepo) ExistsRunnerCompletion(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r *feedbackMonitorRepo) PatchRunnerCompletionMetadata(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}
func (r *feedbackMonitorRepo) EnsureTraceSchema(context.Context) error { return nil }
func (r *feedbackMonitorRepo) InsertMonitorTrace(context.Context, MonitorTraceWrite) error {
	return nil
}
func (r *feedbackMonitorRepo) UpsertMonitorTraceSpan(context.Context, MonitorTraceSpanWrite) error {
	return nil
}
func (r *feedbackMonitorRepo) UpdateMonitorTraceCompletion(_ context.Context, _ string, _ string, _ int64, _, _ int, _ int64, _ float64) error {
	return nil
}
func (r *feedbackMonitorRepo) ListRecentRunnerCompletions(_ context.Context, _ time.Duration, _ int) ([]RunnerCompletionRow, error) {
	return nil, nil
}
func (r *feedbackMonitorRepo) LatencyPercentilesSince(_ context.Context, _ string) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}

type testFeedbackEnqueuer struct {
	calls []struct {
		SessionID, MessageID, Rating, Comment string
	}
}

func (e *testFeedbackEnqueuer) EnqueueFeedbackMemory(sessionID, messageID, rating, comment string, _ time.Time) {
	e.calls = append(e.calls, struct {
		SessionID, MessageID, Rating, Comment string
	}{sessionID, messageID, rating, comment})
}

func TestRecordUserFeedbackMonitor(t *testing.T) {
	repo := &feedbackMonitorRepo{}
	uc := NewMonitorUsecase(repo, repo, repo, repo, repo, nil)
	if err := RecordUserFeedbackMonitor(context.Background(), uc, "sess-1", "msg-1", "negative", "too verbose"); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(repo.events))
	}
	ev := repo.events[0]
	if ev.EventKey != "chat.user_feedback" || ev.Status != "warning" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestUserFeedbackConsumer_handle(t *testing.T) {
	repo := &feedbackMonitorRepo{}
	uc := NewMonitorUsecase(repo, repo, repo, repo, repo, nil)
	feedback := &testFeedbackEnqueuer{}
	worker := NewTurnMemoryWorker(feedback, noopSessionLogWriter{})

	// bus is nil because handle is called directly without going through Start.
	var bus EventBus
	c := &userFeedbackConsumer{bus: bus, monitor: uc, memWorker: worker}
	c.handle(context.Background(), NewSystemNoticeEvent("sess-1", "user_feedback", "", map[string]any{
		"message_id": "msg-9",
		"rating":     "positive",
		"comment":    "helpful",
	}))
	if len(repo.events) != 1 {
		t.Fatalf("expected monitor event, got %d", len(repo.events))
	}
	if len(feedback.calls) != 1 || feedback.calls[0].MessageID != "msg-9" {
		t.Fatalf("expected feedback enqueue, got %+v", feedback.calls)
	}
}
