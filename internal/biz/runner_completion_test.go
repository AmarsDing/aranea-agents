package biz

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type completionMonitorRepo struct {
	inserted    int
	exists      bool
	patches     []string
	patchResult bool
}

func (r *completionMonitorRepo) ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error) {
	return AuditListResult{}, nil
}
func (r *completionMonitorRepo) InsertAuditLog(ctx context.Context, entry AuditLog) error {
	return nil
}
func (r *completionMonitorRepo) InsertMonitorEvent(ctx context.Context, ev MonitorEventWrite) error {
	r.inserted++
	return nil
}
func (r *completionMonitorRepo) ListMonitorEvents(ctx context.Context, query MonitorEventsQuery) (MonitorListResult, error) {
	return MonitorListResult{}, nil
}
func (r *completionMonitorRepo) GetMonitorEvent(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return MonitorPlatformRow{}, nil
}
func (r *completionMonitorRepo) ListMonitorTraces(ctx context.Context, query MonitorTracesQuery) (MonitorListResult, error) {
	return MonitorListResult{}, nil
}
func (r *completionMonitorRepo) GetMonitorTrace(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return MonitorPlatformRow{}, nil
}
func (r *completionMonitorRepo) ListAlertRules(ctx context.Context) ([]MonitorAlertRule, error) {
	return nil, nil
}
func (r *completionMonitorRepo) ReplaceAlertRules(ctx context.Context, rules []MonitorAlertRule) error {
	return nil
}
func (r *completionMonitorRepo) UpdateAlertFiringState(_ context.Context, _ string, _ MonitorAlertFiringState, _ *time.Time, _ float64, _ *time.Time) error {
	return nil
}
func (r *completionMonitorRepo) CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339, untilRFC3339 string) (int32, error) {
	return 0, nil
}

func (r *completionMonitorRepo) AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error) {
	return 0, nil
}
func (r *completionMonitorRepo) ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error) {
	return r.exists, nil
}
func (r *completionMonitorRepo) PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error) {
	if !r.patchResult {
		return false, nil
	}
	r.patches = append(r.patches, patchJSON)
	return true, nil
}

func (r *completionMonitorRepo) EnsureTraceSchema(context.Context) error { return nil }
func (r *completionMonitorRepo) InterruptStaleTraces(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *completionMonitorRepo) DeleteMonitorEventsOlderThan(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (r *completionMonitorRepo) InsertMonitorTrace(context.Context, MonitorTraceWrite) error {
	return nil
}
func (r *completionMonitorRepo) UpsertMonitorTraceSpan(context.Context, MonitorTraceSpanWrite) error {
	return nil
}
func (r *completionMonitorRepo) UpdateMonitorTraceCompletion(_ context.Context, _ string, _ MonitorTraceCompletion) error {
	return nil
}
func (r *completionMonitorRepo) ListRecentRunnerCompletions(_ context.Context, _ time.Duration, _ int) ([]RunnerCompletionRow, error) {
	return nil, nil
}
func (r *completionMonitorRepo) LatencyPercentilesSince(_ context.Context, _ string) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}

func TestBuildRunnerCompletionMetadataJSON_v1(t *testing.T) {
	de := DomainEvent{
		SessionID:        "sess-1",
		RunID:            "run-1",
		TraceID:          "tr-1",
		InvocationID:     "run-1",
		AgentID:          "ag-1",
		AgentDisplayName: "Demo",
		RunKind:          "chat",
		DurationMS:       1200,
		Usage:            &DomainUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
	raw := BuildRunnerCompletionMetadataJSON(de, "ok")
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["schema_version"] != runnerCompletionSchemaV1 {
		t.Fatalf("schema_version: %v", meta["schema_version"])
	}
	if meta["trace_id"] != "tr-1" || meta["run_id"] != "run-1" {
		t.Fatalf("correlation: %+v", meta)
	}
}

func TestRecordRunnerCompletion_idempotent(t *testing.T) {
	repo := &completionMonitorRepo{exists: true}
	uc := NewMonitorUsecase(repo, repo, repo, repo, repo, nil)
	de := DomainEvent{SessionID: "s1", InvocationID: "r1", RunID: "r1", Timestamp: time.Now()}
	if err := RecordRunnerCompletion(context.Background(), uc, de); err != nil {
		t.Fatal(err)
	}
	if repo.inserted != 0 {
		t.Fatalf("expected skip insert, got %d", repo.inserted)
	}
}

func TestRecordRunnerCompletion_appliesPendingUsageWhenExists(t *testing.T) {
	bridge := &TurnCompletionBridge{}
	de := DomainEvent{SessionID: "s1", RunID: "r1", InvocationID: "r1", Timestamp: time.Now()}
	bridge.RegisterTurnUsage("s1", "r1", "usage-1", "tr-1", "", "")
	repo := &completionMonitorRepo{exists: true, patchResult: true}
	uc := NewMonitorUsecase(repo, repo, repo, repo, repo, nil)
	old := defaultTurnCompletionBridge
	defaultTurnCompletionBridge = bridge
	defer func() { defaultTurnCompletionBridge = old }()

	if err := RecordRunnerCompletion(context.Background(), uc, de); err != nil {
		t.Fatal(err)
	}
	if len(repo.patches) != 1 {
		t.Fatalf("expected patch on exists row, got %d patches", len(repo.patches))
	}
	var patch map[string]any
	if err := json.Unmarshal([]byte(repo.patches[0]), &patch); err != nil {
		t.Fatal(err)
	}
	if patch["usage_event_id"] != "usage-1" || patch["trace_id"] != "tr-1" {
		t.Fatalf("patch: %+v", patch)
	}
}

func TestLinkRunnerCompletionUsage_stagesBeforeCompletionRow(t *testing.T) {
	bridge := &TurnCompletionBridge{}
	repo := &completionMonitorRepo{patchResult: false}
	uc := NewMonitorUsecase(repo, repo, repo, repo, repo, nil)
	old := defaultTurnCompletionBridge
	defaultTurnCompletionBridge = bridge
	defer func() { defaultTurnCompletionBridge = old }()

	if err := LinkRunnerCompletionUsage(context.Background(), uc, "s1", "r1", "usage-9", "tr-9"); err != nil {
		t.Fatal(err)
	}
	if len(repo.patches) != 0 {
		t.Fatalf("expected no patch before completion row, got %d", len(repo.patches))
	}
	if _, _, ok := bridge.PendingUsage("s1", "r1"); !ok {
		t.Fatal("expected pending usage staged")
	}
}

func TestCompletionDurationMS(t *testing.T) {
	start := time.Now().Add(-2 * time.Second)
	de := DomainEvent{Timestamp: time.Now()}
	if ms := CompletionDurationMS(de, start); ms < 1900 || ms > 2100 {
		t.Fatalf("duration_ms=%d", ms)
	}
}
