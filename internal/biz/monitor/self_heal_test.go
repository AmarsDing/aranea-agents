package monitor_test

import (
	"context"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// mockHealHandler records fix action calls for test assertions.
type mockHealHandler struct {
	calls      atomic.Int32
	lastAction monitor.FixAction
	lastMeta   map[string]any
	shouldErr  bool
}

func (h *mockHealHandler) HandleFixAction(_ context.Context, action monitor.FixAction, meta map[string]any) error {
	h.calls.Add(1)
	h.lastAction = action
	h.lastMeta = meta
	if h.shouldErr {
		return errFixFailed
	}
	return nil
}

var errFixFailed = &errMarker{"fix action failed"}

type errMarker struct{ msg string }

func (e *errMarker) Error() string { return e.msg }

func TestNewSelfHealUsecase_NilDeps(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{}

	if monitor.NewSelfHealUsecase(nil, handler, loggateway.NewNoop()) != nil {
		t.Error("NewSelfHealUsecase(nil diag) should return nil")
	}
	if monitor.NewSelfHealUsecase(diag, nil, loggateway.NewNoop()) != nil {
		t.Error("NewSelfHealUsecase(nil handler) should return nil")
	}
}

func TestSelfHealUsecase_NilReceiver(t *testing.T) {
	var uc *monitor.SelfHealUsecase
	_, err := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "tool-1", "auto", 5)
	if err == nil {
		t.Error("nil receiver should return error")
	}
}

func TestSelfHealUsecase_NoRootCauses(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{}
	uc := monitor.NewSelfHealUsecase(diag, handler, loggateway.NewNoop())

	rec, err := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "", "manual", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Status != "skipped_no_action" {
		t.Errorf("Status = %q, want %q", rec.Status, "skipped_no_action")
	}
}

func TestSelfHealUsecase_LowConfidenceSkipped(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.step.session-check", Name: "step", Status: "error",
						MetadataJSON: `{"step_id":"session-check","flow_phase":"error","error_message":"session expired","session_id":"s1"}`},
				},
				Total: 1,
			}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{}
	uc := monitor.NewSelfHealUsecase(diag, handler, loggateway.NewNoop())

	rec, err := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "session-check", "auto", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// session-expired rule has log_only action, so it should be skipped as "no fixable cause"
	if rec.Status != "skipped_no_action" {
		t.Errorf("Status = %q, want %q (log_only actions are not fixable)", rec.Status, "skipped_no_action")
	}
}

func TestSelfHealUsecase_AppliedFix(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.step.mcp-connect", Name: "step", Status: "error",
						MetadataJSON: `{"step_id":"mcp-connect","flow_phase":"error","error_message":"connection refused","session_id":"s1"}`},
				},
				Total: 1,
			}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{}
	uc := monitor.NewSelfHealUsecase(diag, handler, loggateway.NewNoop())

	rec, err := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "mcp-connect", "auto_error_event", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Status != "applied" {
		t.Errorf("Status = %q, want %q; Reason: %q", rec.Status, "applied", rec.Reason)
	}
	if rec.RuleID != "rc-mcp-connection-failure" {
		t.Errorf("RuleID = %q, want %q", rec.RuleID, "rc-mcp-connection-failure")
	}
	if handler.calls.Load() != 1 {
		t.Errorf("HandleFixAction called %d times, want 1", handler.calls.Load())
	}
	if handler.lastAction.Type != "reconnect" {
		t.Errorf("FixAction.Type = %q, want %q", handler.lastAction.Type, "reconnect")
	}
}

func TestSelfHealUsecase_FixActionFailed(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.step.mcp-connect", Name: "step", Status: "error",
						MetadataJSON: `{"step_id":"mcp-connect","flow_phase":"error","error_message":"connection refused","session_id":"s1"}`},
				},
				Total: 1,
			}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{shouldErr: true}
	uc := monitor.NewSelfHealUsecase(diag, handler, loggateway.NewNoop())

	rec, err := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "mcp-connect", "auto", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Status != "failed" {
		t.Errorf("Status = %q, want %q", rec.Status, "failed")
	}
}

func TestSelfHealUsecase_CooldownPreventsRepeat(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.step.mcp-connect", Name: "step", Status: "error",
						MetadataJSON: `{"step_id":"mcp-connect","flow_phase":"error","error_message":"connection refused","session_id":"s1"}`},
				},
				Total: 1,
			}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{}
	uc := monitor.NewSelfHealUsecase(diag, handler, loggateway.NewNoop())

	// First heal should succeed
	rec1, _ := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "mcp-connect", "auto", 5)
	if rec1.Status != "applied" {
		t.Fatalf("first heal Status = %q, want applied", rec1.Status)
	}

	// Second heal immediately after should be in cooldown
	rec2, _ := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "mcp-connect", "auto", 5)
	if rec2.Status != "skipped_cooldown" {
		t.Errorf("second heal Status = %q, want skipped_cooldown", rec2.Status)
	}
}

func TestSelfHealUsecase_ListHealRecords(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{}
	uc := monitor.NewSelfHealUsecase(diag, handler, loggateway.NewNoop())

	// Generate a record
	uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "", "manual", 5)

	records, total := uc.ListHealRecords(10, 0)
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(records) != 1 {
		t.Errorf("records len = %d, want 1", len(records))
	}
}

func TestSelfHealUsecase_ProviderTimeoutRetry(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.step.llm-call", Name: "step", Status: "error",
						MetadataJSON: `{"step_id":"llm-call","flow_phase":"error","error_message":"request timeout exceeded","session_id":"s1"}`},
				},
				Total: 1,
			}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{}
	uc := monitor.NewSelfHealUsecase(diag, handler, loggateway.NewNoop())

	rec, err := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "llm-call", "auto", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Status != "applied" {
		t.Errorf("Status = %q, want %q; Reason: %q", rec.Status, "applied", rec.Reason)
	}
	if rec.RuleID != "rc-provider-timeout" {
		t.Errorf("RuleID = %q, want %q", rec.RuleID, "rc-provider-timeout")
	}
	if handler.lastAction.Type != "retry" {
		t.Errorf("FixAction.Type = %q, want %q", handler.lastAction.Type, "retry")
	}
}

func TestSelfHealUsecase_RateLimitRetry(t *testing.T) {
	repo := &mockRepo{
		listMonitorEventsFn: func(_ context.Context, _ monitor.EventsQuery) (monitor.ListResult, error) {
			return monitor.ListResult{
				Items: []monitor.PlatformRow{
					{ID: "e1", Key: "runner.step.llm-call", Name: "step", Status: "error",
						MetadataJSON: `{"step_id":"llm-call","flow_phase":"error","error_code":"429","session_id":"s1"}`},
				},
				Total: 1,
			}, nil
		},
	}
	diag := monitor.NewDiagBundleGenerator(repo, repo, monitor.NewRootCauseEngine(loggateway.NewNoop()))
	handler := &mockHealHandler{}
	uc := monitor.NewSelfHealUsecase(diag, handler, loggateway.NewNoop())

	rec, _ := uc.DiagnoseAndHeal(context.Background(), "", "s1", "", "llm-call", "auto", 5)
	if rec.Status != "applied" {
		t.Errorf("Status = %q, want applied; Reason: %q", rec.Status, rec.Reason)
	}
	if handler.lastAction.Type != "retry" {
		t.Errorf("FixAction.Type = %q, want retry", handler.lastAction.Type)
	}
	if handler.lastAction.MaxAttempts != 3 {
		t.Errorf("FixAction.MaxAttempts = %d, want 3", handler.lastAction.MaxAttempts)
	}
}
