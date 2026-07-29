package monitor_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/auth"
)

func TestAuditAction(t *testing.T) {
	if got := monitor.AuditAction(monitor.AuditVerbCreate, "agent"); got != "create.agent" {
		t.Errorf("AuditAction = %q, want %q", got, "create.agent")
	}
	if got := monitor.AuditAction(" delete ", " session "); got != "delete.session" {
		t.Errorf("AuditAction should trim spaces, got %q", got)
	}
}

func TestAuditSeverity(t *testing.T) {
	cases := map[string]string{
		"create.agent":               monitor.AuditSeverityInfo,
		"update.session":             monitor.AuditSeverityInfo,
		"delete.agent":               monitor.AuditSeverityWarning,
		"delete.session":             monitor.AuditSeverityWarning,
		"credentials.update.channel": monitor.AuditSeverityWarning,
		"archive.session":            monitor.AuditSeverityInfo,
		"sync.skill":                 monitor.AuditSeverityInfo,
	}
	for action, want := range cases {
		if got := monitor.AuditSeverity(action); got != want {
			t.Errorf("AuditSeverity(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestRecordAdminAudit_DetailJSON(t *testing.T) {
	var got monitor.AuditLog
	repo := &mockRepo{
		insertAuditLogFn: func(_ context.Context, entry monitor.AuditLog) error {
			got = entry
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.RecordAdminAudit(context.Background(), monitor.AdminAuditEntry{
		Action:     monitor.AuditAction(monitor.AuditVerbUpdate, "agent"),
		Resource:   "agent",
		ResourceID: "a-1",
		Summary:    "key=demo",
		After:      map[string]any{"name": "new"},
	})

	var detail struct {
		Summary string         `json:"summary"`
		Before  map[string]any `json:"before"`
		After   map[string]any `json:"after"`
	}
	if err := json.Unmarshal([]byte(got.Detail), &detail); err != nil {
		t.Fatalf("Detail should be JSON, got %q: %v", got.Detail, err)
	}
	if detail.Summary != "key=demo" {
		t.Errorf("summary = %q, want %q", detail.Summary, "key=demo")
	}
	if detail.After["name"] != "new" {
		t.Errorf("after.name = %v, want %q", detail.After["name"], "new")
	}
	if strings.Contains(got.Detail, `"before"`) {
		t.Errorf("nil Before should be omitted, got %q", got.Detail)
	}
}

func TestRecordAdminAudit_SummaryOnlyProducesJSONObject(t *testing.T) {
	var got monitor.AuditLog
	repo := &mockRepo{
		insertAuditLogFn: func(_ context.Context, entry monitor.AuditLog) error {
			got = entry
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	uc.RecordAdminAudit(context.Background(), monitor.AdminAuditEntry{
		Action:   monitor.AuditAction(monitor.AuditVerbCreate, "tool"),
		Resource: "tool",
		Summary:  "key=search",
	})
	var detail struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(got.Detail), &detail); err != nil {
		t.Fatalf("Detail should be JSON object, got %q: %v", got.Detail, err)
	}
	if detail.Summary != "key=search" {
		t.Errorf("summary = %q, want %q", detail.Summary, "key=search")
	}
}

func TestRecordAdminAudit_AutoSeverity(t *testing.T) {
	var got monitor.AuditLog
	repo := &mockRepo{
		insertAuditLogFn: func(_ context.Context, entry monitor.AuditLog) error {
			got = entry
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)

	uc.RecordAdminAudit(context.Background(), monitor.AdminAuditEntry{
		Action:   monitor.AuditAction(monitor.AuditVerbDelete, "agent"),
		Resource: "agent",
	})
	if got.Severity != monitor.AuditSeverityWarning {
		t.Errorf("delete severity = %q, want %q", got.Severity, monitor.AuditSeverityWarning)
	}

	uc.RecordAdminAudit(context.Background(), monitor.AdminAuditEntry{
		Action:   monitor.AuditAction(monitor.AuditVerbCreate, "agent"),
		Resource: "agent",
	})
	if got.Severity != monitor.AuditSeverityInfo {
		t.Errorf("create severity = %q, want %q", got.Severity, monitor.AuditSeverityInfo)
	}

	uc.RecordAdminAudit(context.Background(), monitor.AdminAuditEntry{
		Action:   monitor.AuditAction(monitor.AuditVerbUpdate, "agent"),
		Resource: "agent",
		Severity: "critical",
	})
	if got.Severity != "critical" {
		t.Errorf("explicit severity should be preserved, got %q", got.Severity)
	}
}

func TestRecordAdminAudit_ActorAndMeta(t *testing.T) {
	var got monitor.AuditLog
	repo := &mockRepo{
		insertAuditLogFn: func(_ context.Context, entry monitor.AuditLog) error {
			got = entry
			return nil
		},
	}
	uc := monitor.NewUsecase(repo, repo, repo, repo, repo, nil)
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 42})
	uc.RecordAdminAudit(ctx, monitor.AdminAuditEntry{
		Action:    monitor.AuditAction(monitor.AuditVerbUpdate, "provider"),
		Resource:  "provider",
		IP:        "10.0.0.1",
		UserAgent: "Mozilla/5.0",
		RequestID: "req-1",
	})
	if got.Actor != "42" {
		t.Errorf("Actor = %q, want %q", got.Actor, "42")
	}
	if got.IP != "10.0.0.1" || got.UserAgent != "Mozilla/5.0" || got.RequestID != "req-1" {
		t.Errorf("meta not propagated: ip=%q ua=%q rid=%q", got.IP, got.UserAgent, got.RequestID)
	}
}

func TestRecordAdminAudit_NilUsecase(t *testing.T) {
	var uc *monitor.Usecase
	uc.RecordAdminAudit(context.Background(), monitor.AdminAuditEntry{Action: "create.agent"}) // must not panic
}
