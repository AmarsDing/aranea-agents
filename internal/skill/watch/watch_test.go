package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestSlugFromEvent_Basic(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/data/skills/my-skill/SKILL.md")
	if slug != "my-skill" {
		t.Fatalf("expected my-skill, got %q", slug)
	}
}

func TestSlugFromEvent_RootPath(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/data/skills")
	if slug != "" {
		t.Fatalf("expected empty for root path, got %q", slug)
	}
}

func TestSlugFromEvent_DotDir(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/data/skills/.hidden/file.md")
	if slug != "" {
		t.Fatalf("expected empty for dot dir, got %q", slug)
	}
}

func TestSlugFromEvent_DeepPath(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/data/skills/my-skill/sub/file.md")
	if slug != "my-skill" {
		t.Fatalf("expected my-skill, got %q", slug)
	}
}

func TestSlugFromEvent_UnrelatedPath(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/other/path/file.md")
	if slug != "" {
		t.Fatalf("expected empty for unrelated path, got %q", slug)
	}
}

func TestNewMonitorSyncReporter_BothNil(t *testing.T) {
	if NewMonitorSyncReporter(nil, nil, nil) != nil {
		t.Fatal("should return nil when both args are nil")
	}
}

func TestSetSyncReporter_NilRunner(t *testing.T) {
	SetSyncReporter(nil, nil)
}

func TestSetAlertEvaluator_NilRunner(t *testing.T) {
	SetAlertEvaluator(nil, nil)
}

func TestNewRunner(t *testing.T) {
	r := NewRunner(nil, nil, nil, loggateway.NewNoop())
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
}

func TestNewRunnerWithBus(t *testing.T) {
	r := NewRunnerWithBus(nil, nil, nil, nil, loggateway.NewNoop())
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
}

func TestRunner_Start_NilRunner(t *testing.T) {
	var r *Runner
	r.Start(context.Background())
}

func TestRunner_reportSync_NilRunner(t *testing.T) {
	var r *Runner
	r.reportSync(context.Background(), "test", "slug", "msg", "info")
}

func TestRunner_reportSync_NilReporter(t *testing.T) {
	r := NewRunner(nil, nil, nil, loggateway.NewNoop())
	r.reportSync(context.Background(), "test", "slug", "msg", "info")
}

// stubMonitorPersister records persisted monitor events / audit entries.
type stubMonitorPersister struct {
	events []biz.MonitorEventWrite
	audits []biz.AdminAuditEntry
}

func (s *stubMonitorPersister) RecordMonitorEvent(_ context.Context, ev biz.MonitorEventWrite) error {
	s.events = append(s.events, ev)
	return nil
}

func (s *stubMonitorPersister) RecordAdminAudit(_ context.Context, e biz.AdminAuditEntry) {
	s.audits = append(s.audits, e)
}

// S2: the reporter must dedup repeated (eventKey, slug) governance events
// within reportDedupWindow — a persistently missing/invalid skill dir would
// otherwise write monitor_events + admin_audit rows every reconcile round.
func TestMonitorSyncReporter_DeduplicatesWithinWindow(t *testing.T) {
	stub := &stubMonitorPersister{}
	r := &monitorSyncReporter{mon: stub, lg: loggateway.NewNoop(), dedupLast: map[string]time.Time{}}
	ctx := context.Background()
	report := SyncReport{EventKey: "skill.filesystem.missing", Slug: "demo", Message: "missing", Severity: "warn"}

	r.ReportFilesystemSync(ctx, report)
	r.ReportFilesystemSync(ctx, report)
	r.ReportFilesystemSync(ctx, report)
	if len(stub.events) != 1 {
		t.Fatalf("expected 1 persisted event within window, got %d", len(stub.events))
	}
	if len(stub.audits) != 1 {
		t.Fatalf("expected 1 persisted audit within window, got %d", len(stub.audits))
	}

	// A different slug for the same event key is not deduped.
	r.ReportFilesystemSync(ctx, SyncReport{EventKey: "skill.filesystem.missing", Slug: "other", Message: "missing", Severity: "warn"})
	if len(stub.events) != 2 {
		t.Fatalf("expected distinct slug to persist, got %d events", len(stub.events))
	}

	// After the window expires the same key persists again (simulate by
	// backdating the recorded timestamp).
	r.dedupMu.Lock()
	r.dedupLast["skill.filesystem.missing|demo"] = time.Now().Add(-2 * reportDedupWindow)
	r.dedupMu.Unlock()
	r.ReportFilesystemSync(ctx, report)
	if len(stub.events) != 3 {
		t.Fatalf("expected persist after window expiry, got %d events", len(stub.events))
	}
}

// stubSkillWriter records MarkFilesystemMissing / UpsertSkillFromDisk calls.
type stubSkillWriter struct {
	markMissing []string
	upserts     []string
}

func (s *stubSkillWriter) MarkFilesystemMissing(_ context.Context, slug string, _ bool) error {
	s.markMissing = append(s.markMissing, slug)
	return nil
}

func (s *stubSkillWriter) UpsertSkillFromDisk(_ context.Context, input biz.SkillDiskSyncInput) (biz.Skill, biz.SkillDiskSyncOutcome, error) {
	s.upserts = append(s.upserts, input.Slug)
	return biz.Skill{}, biz.SkillDiskSyncOutcome{}, nil
}

// S2: applyOverwrite crash residue is named ".<slug>.overwrite-backup" (dot
// prefix) so scanAll's hidden-dir skip keeps it from being synced as a skill.
func TestScanAll_SkipsOverwriteBackupDir(t *testing.T) {
	root := t.TempDir()
	backup := filepath.Join(root, ".demo.overwrite-backup")
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "SKILL.md"), []byte("# demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	writer := &stubSkillWriter{}
	r := NewRunner(nil, writer, nil, loggateway.NewNoop())
	r.scanAll(context.Background(), root, biz.SkillInvocationSourceFilesystemScan)
	if len(writer.markMissing) != 0 || len(writer.upserts) != 0 {
		t.Fatalf("expected dot-prefixed backup dir to be skipped, got missing=%v upserts=%v", writer.markMissing, writer.upserts)
	}
}
