package modelregistry

import (
	"os"
	"testing"
)

func TestAppendSyncLog_NilStore(t *testing.T) {
	if err := AppendSyncLog(nil, SyncLogEntry{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestAppendSyncLog_AppendsEntry(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	entry := SyncLogEntry{
		ID:        "log-1",
		StartedAt: "2026-01-01T00:00:00Z",
		Status:    "success",
		SourceURL: "https://example.com",
		Stats:     SyncStats{Providers: 2, Models: 10},
	}
	if err := AppendSyncLog(st, entry); err != nil {
		t.Fatal(err)
	}
	logs, err := ReadSyncLogs(st, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].ID != "log-1" {
		t.Fatalf("expected ID log-1, got %q", logs[0].ID)
	}
	if logs[0].Stats.Providers != 2 || logs[0].Stats.Models != 10 {
		t.Fatalf("stats mismatch: %+v", logs[0].Stats)
	}
}

func TestAppendSyncLog_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	for i := 0; i < 5; i++ {
		entry := SyncLogEntry{
			ID:     "log-" + string(rune('A'+i)),
			Status: "success",
		}
		if err := AppendSyncLog(st, entry); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := ReadSyncLogs(st, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
	if logs[0].ID != "log-E" || logs[1].ID != "log-D" || logs[2].ID != "log-C" {
		t.Fatalf("expected newest-first [E D C], got %v", []string{logs[0].ID, logs[1].ID, logs[2].ID})
	}
}

func TestReadSyncLogs_NilStore(t *testing.T) {
	logs, err := ReadSyncLogs(nil, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if logs != nil {
		t.Fatalf("expected nil, got %v", logs)
	}
}

func TestReadSyncLogs_NoFile(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	logs, err := ReadSyncLogs(st, 10)
	if err != nil {
		t.Fatal(err)
	}
	if logs != nil {
		t.Fatalf("expected nil, got %v", logs)
	}
}

func TestReadSyncLogs_DefaultLimit(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	for i := 0; i < 60; i++ {
		if err := AppendSyncLog(st, SyncLogEntry{ID: "log", Status: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := ReadSyncLogs(st, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 50 {
		t.Fatalf("expected 50 (default limit), got %d", len(logs))
	}
}

func TestReadSyncLogs_RespectsLimit(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	for i := 0; i < 10; i++ {
		if err := AppendSyncLog(st, SyncLogEntry{ID: "log", Status: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := ReadSyncLogs(st, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3, got %d", len(logs))
	}
}

func TestReadSyncLogs_SkipsBadJSON(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.ensureDir(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(st.LogsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("bad json line\n")
	_ = f.Close()

	if err := AppendSyncLog(st, SyncLogEntry{ID: "good-1", Status: "ok"}); err != nil {
		t.Fatal(err)
	}

	logs, err := ReadSyncLogs(st, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 valid log, got %d", len(logs))
	}
	if logs[0].ID != "good-1" {
		t.Fatalf("expected ID good-1, got %q", logs[0].ID)
	}
}

func TestUpdateSyncLogEntry_NilStore(t *testing.T) {
	if err := UpdateSyncLogEntry(nil, SyncLogEntry{ID: "x"}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestUpdateSyncLogEntry_EmptyID(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := UpdateSyncLogEntry(st, SyncLogEntry{ID: ""}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := UpdateSyncLogEntry(st, SyncLogEntry{ID: "   "}); err != nil {
		t.Fatalf("expected nil for whitespace ID, got %v", err)
	}
}

func TestUpdateSyncLogEntry_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	orig := SyncLogEntry{
		ID:        "log-1",
		StartedAt: "2026-01-01T00:00:00Z",
		Status:    "running",
		SourceURL: "https://example.com",
	}
	if err := AppendSyncLog(st, orig); err != nil {
		t.Fatal(err)
	}
	updated := orig
	updated.Status = "success"
	updated.FinishedAt = "2026-01-01T00:05:00Z"
	updated.Stats = SyncStats{Providers: 3, Models: 15}
	if err := UpdateSyncLogEntry(st, updated); err != nil {
		t.Fatal(err)
	}
	logs, err := ReadSyncLogs(st, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Status != "success" {
		t.Fatalf("expected status success, got %q", logs[0].Status)
	}
	if logs[0].FinishedAt != "2026-01-01T00:05:00Z" {
		t.Fatalf("expected updated FinishedAt, got %q", logs[0].FinishedAt)
	}
	if logs[0].Stats.Providers != 3 || logs[0].Stats.Models != 15 {
		t.Fatalf("stats mismatch: %+v", logs[0].Stats)
	}
}

func TestUpdateSyncLogEntry_NoFileFallback(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	entry := SyncLogEntry{
		ID:     "log-fallback",
		Status: "success",
	}
	if err := UpdateSyncLogEntry(st, entry); err != nil {
		t.Fatal(err)
	}
	logs, err := ReadSyncLogs(st, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log (fallback append), got %d", len(logs))
	}
	if logs[0].ID != "log-fallback" {
		t.Fatalf("expected ID log-fallback, got %q", logs[0].ID)
	}
}

func TestUpdateSyncLogEntry_PreservesOtherEntries(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	entry1 := SyncLogEntry{ID: "log-A", Status: "success", SourceURL: "https://a.com"}
	entry2 := SyncLogEntry{ID: "log-B", Status: "running", SourceURL: "https://b.com"}
	entry3 := SyncLogEntry{ID: "log-C", Status: "success", SourceURL: "https://c.com"}
	if err := AppendSyncLog(st, entry1); err != nil {
		t.Fatal(err)
	}
	if err := AppendSyncLog(st, entry2); err != nil {
		t.Fatal(err)
	}
	if err := AppendSyncLog(st, entry3); err != nil {
		t.Fatal(err)
	}

	entry2Updated := entry2
	entry2Updated.Status = "failed"
	entry2Updated.Message = "timeout"
	if err := UpdateSyncLogEntry(st, entry2Updated); err != nil {
		t.Fatal(err)
	}

	logs, err := ReadSyncLogs(st, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}

	byID := make(map[string]SyncLogEntry)
	for _, l := range logs {
		byID[l.ID] = l
	}
	if byID["log-A"].Status != "success" {
		t.Fatalf("log-A should be unchanged, got status %q", byID["log-A"].Status)
	}
	if byID["log-B"].Status != "failed" || byID["log-B"].Message != "timeout" {
		t.Fatalf("log-B should be updated, got status=%q message=%q", byID["log-B"].Status, byID["log-B"].Message)
	}
	if byID["log-C"].Status != "success" {
		t.Fatalf("log-C should be unchanged, got status %q", byID["log-C"].Status)
	}
}
