package monitor_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

func TestNewFlowFileAppender_NonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	if a == nil {
		t.Fatal("NewFlowFileAppender returned nil")
	}
	if a.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", a.Dir(), dir)
	}
}

func TestNewFlowFileAppender_EmptyDir(t *testing.T) {
	a := monitor.NewFlowFileAppender("", loggateway.NewNoop())
	if a == nil {
		t.Fatal("NewFlowFileAppender returned nil")
	}
	want := "/var/log/aranea"
	if runtime.GOOS == "windows" {
		want = "./logs"
	}
	if a.Dir() != want {
		t.Errorf("Dir() = %q, want %q", a.Dir(), want)
	}
}

func TestNewFlowFileAppender_StartCancelledContext(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Start(ctx, nil)
	time.Sleep(50 * time.Millisecond)
}

func TestNewFlowFileAppender_StartNilAppender(t *testing.T) {
	var a *monitor.FlowFileAppender
	a.Start(context.Background(), nil)
}

func TestFlowFileAppender_OnMonitorEvent_FlowLog(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	ev := contract.MonitorEvent{
		ID:        "env-1",
		Type:      contract.MonitorEventTypeFlowLog,
		SessionID: "sess-1",
		Timestamp: time.Now().UTC(),
		Source:    "chat",
		Metadata:  map[string]any{"key1": "value1"},
		Message:   "hello",
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	pattern := filepath.Join(dir, "flow-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no flow file found in %q", dir)
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}

	if row["_id"] != "env-1" {
		t.Errorf("_id = %v, want env-1", row["_id"])
	}
	if row["_session_id"] != "sess-1" {
		t.Errorf("_session_id = %v, want sess-1", row["_session_id"])
	}
	if row["_text"] != "hello" {
		t.Errorf("_text = %v, want hello", row["_text"])
	}
	if row["key1"] != "value1" {
		t.Errorf("key1 = %v, want value1", row["key1"])
	}
}

func TestFlowFileAppender_OnMonitorEvent_SystemLog(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	// Source="flow" routes to systemFile (preserves legacy Channel="monitor" behavior;
	// FlowTracker emits with Source="flow").
	ev := contract.MonitorEvent{
		ID:        "env-sys",
		Type:      contract.MonitorEventTypeFlowLog,
		SessionID: "sess-2",
		Timestamp: time.Now().UTC(),
		Source:    "flow",
		Metadata:  map[string]any{},
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	pattern := filepath.Join(dir, "system-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no system file found in %q", dir)
	}
}

func TestFlowFileAppender_OnMonitorEvent_AlertNotify(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	ev := contract.MonitorEvent{
		ID:        "env-alert",
		Type:      contract.MonitorEventTypeAlertNotify,
		SessionID: "sess-3",
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"alert_key": "test"},
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	pattern := filepath.Join(dir, "alert-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no alert file found in %q", dir)
	}
}

func TestFlowFileAppender_OnMonitorEvent_MCPHealthAlert(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	ev := contract.MonitorEvent{
		ID:        "env-mcp",
		Type:      contract.MonitorEventTypeMCPHealthAlert,
		SessionID: "sess-4",
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{},
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	pattern := filepath.Join(dir, "alert-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no alert file found for MCPHealthAlert in %q", dir)
	}
}

func TestFlowFileAppender_OnMonitorEvent_NilMetadata(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	ev := contract.MonitorEvent{
		ID:        "env-nil",
		Type:      contract.MonitorEventTypeFlowLog,
		SessionID: "sess-6",
		Timestamp: time.Now().UTC(),
		Metadata:  nil,
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files when Metadata is nil, got %d", len(entries))
	}
}

func TestFlowFileAppender_OnMonitorEvent_NilAppender(t *testing.T) {
	var a *monitor.FlowFileAppender
	ev := contract.MonitorEvent{
		ID:       "env-nil-appender",
		Type:     contract.MonitorEventTypeFlowLog,
		Metadata: map[string]any{},
	}
	a.OnMonitorEventExposed(ev)
}

func TestFlowFileAppender_OnMonitorEvent_UnknownType(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	ev := contract.MonitorEvent{
		ID:        "env-unknown",
		Type:      contract.MonitorEventType("unknown_type"),
		SessionID: "sess-7",
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{},
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files for unknown monitor event type, got %d", len(entries))
	}
}

func TestFlowFileAppender_OnMonitorEvent_RoutesToCorrectFiles(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	now := time.Now().UTC()
	evs := []contract.MonitorEvent{
		{ID: "f1", Type: contract.MonitorEventTypeFlowLog, SessionID: "s1", Timestamp: now, Source: "chat", Metadata: map[string]any{}},
		{ID: "s1", Type: contract.MonitorEventTypeFlowLog, SessionID: "s2", Timestamp: now, Source: "flow", Metadata: map[string]any{}},
		{ID: "a1", Type: contract.MonitorEventTypeAlertNotify, SessionID: "s3", Timestamp: now, Metadata: map[string]any{}},
	}
	for _, ev := range evs {
		a.OnMonitorEventExposed(ev)
	}
	a.SyncOpenFilesExposed()

	paths := a.RotatingFilePaths()
	if len(paths) != 3 {
		t.Errorf("RotatingFilePaths() = %d files, want 3", len(paths))
	}

	prefixes := map[string]bool{}
	for _, p := range paths {
		prefix := strings.SplitN(p, "-", 2)[0]
		prefixes[prefix] = true
	}
	for _, want := range []string{"flow", "system", "alert"} {
		if !prefixes[want] {
			t.Errorf("missing file prefix %q in paths %v", want, paths)
		}
	}
}

func TestFlowFileAppender_CompressOldFiles(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetCompressAge(1 * time.Hour)

	oldDate := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02")
	oldFileName := "flow-" + oldDate + ".jsonl"
	oldPath := filepath.Join(dir, oldFileName)

	content := `{"_id":"old","_ts":"2025-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(oldPath, []byte(content), 0644); err != nil {
		t.Fatalf("write old file error: %v", err)
	}

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes error: %v", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	recentFileName := "flow-" + today + ".jsonl"
	recentPath := filepath.Join(dir, recentFileName)
	if err := os.WriteFile(recentPath, []byte(`{"_id":"recent"}`+"\n"), 0644); err != nil {
		t.Fatalf("write recent file error: %v", err)
	}

	compressed := a.CompressOldFilesExposed()
	if compressed != 1 {
		t.Errorf("compressed = %d, want 1", compressed)
	}

	gzPath := oldPath + ".gz"
	if _, err := os.Stat(gzPath); err != nil {
		t.Errorf("compressed file %q should exist: %v", gzPath, err)
	}

	if _, err := os.Stat(recentPath); err != nil {
		t.Error("recent .jsonl file should not be compressed")
	}

	f, err := os.Open(gzPath)
	if err != nil {
		t.Fatalf("open gz error: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader error: %v", err)
	}
	defer gr.Close()

	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read decompressed error: %v", err)
	}

	if string(decompressed) != content {
		t.Errorf("decompressed content = %q, want %q", string(decompressed), content)
	}
}

func TestFlowFileAppender_CompressOldFiles_SkipExistingGz(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetCompressAge(1 * time.Millisecond)

	oldDate := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02")
	oldFileName := "flow-" + oldDate + ".jsonl"
	oldPath := filepath.Join(dir, oldFileName)

	if err := os.WriteFile(oldPath, []byte(`{"_id":"old"}`+"\n"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes error: %v", err)
	}

	gzPath := oldPath + ".gz"
	if err := os.WriteFile(gzPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("write gz error: %v", err)
	}

	compressed := a.CompressOldFilesExposed()
	if compressed != 0 {
		t.Errorf("compressed = %d, want 0 (gz already exists)", compressed)
	}
}

func TestFlowFileAppender_CompressOldFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetCompressAge(1 * time.Millisecond)

	compressed := a.CompressOldFilesExposed()
	if compressed != 0 {
		t.Errorf("compressed = %d, want 0 for empty dir", compressed)
	}
}

func TestFlowFileAppender_CompressOldFiles_ZeroCompressAge(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetCompressAge(0)

	oldDate := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02")
	oldPath := filepath.Join(dir, "flow-"+oldDate+".jsonl")
	if err := os.WriteFile(oldPath, []byte("data"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	compressed := a.CompressOldFilesExposed()
	if compressed != 0 {
		t.Errorf("compressed = %d, want 0 when compressAge is zero", compressed)
	}
}

func TestFlowFileAppender_PurgeExpiredFiles(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetRetentionDays(1)

	oldDate := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02")
	oldFile := "flow-" + oldDate + ".jsonl.gz"
	oldPath := filepath.Join(dir, oldFile)
	if err := os.WriteFile(oldPath, []byte("old data"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes error: %v", err)
	}

	recentFile := "flow-" + time.Now().UTC().Format("2006-01-02") + ".jsonl.gz"
	recentPath := filepath.Join(dir, recentFile)
	if err := os.WriteFile(recentPath, []byte("recent data"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	purged := a.PurgeExpiredFilesExposed()
	if purged != 1 {
		t.Errorf("purged = %d, want 1", purged)
	}

	if _, err := os.Stat(oldPath); err == nil {
		t.Error("old .jsonl.gz file should be purged")
	}

	if _, err := os.Stat(recentPath); err != nil {
		t.Error("recent .jsonl.gz file should not be purged")
	}
}

func TestFlowFileAppender_PurgeExpiredFiles_JsonlToo(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetRetentionDays(1)

	oldDate := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02")
	oldFile := "flow-" + oldDate + ".jsonl"
	oldPath := filepath.Join(dir, oldFile)
	if err := os.WriteFile(oldPath, []byte("old"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes error: %v", err)
	}

	purged := a.PurgeExpiredFilesExposed()
	if purged != 1 {
		t.Errorf("purged = %d, want 1", purged)
	}

	if _, err := os.Stat(oldPath); err == nil {
		t.Error("old .jsonl file should be purged")
	}
}

func TestFlowFileAppender_PurgeExpiredFiles_ZeroRetention(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetRetentionDays(0)

	oldDate := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02")
	oldPath := filepath.Join(dir, "flow-"+oldDate+".jsonl.gz")
	if err := os.WriteFile(oldPath, []byte("old"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	purged := a.PurgeExpiredFilesExposed()
	if purged != 0 {
		t.Errorf("purged = %d, want 0 when retentionDays is zero", purged)
	}
}

func TestFlowFileAppender_PurgeExpiredFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetRetentionDays(1)

	purged := a.PurgeExpiredFilesExposed()
	if purged != 0 {
		t.Errorf("purged = %d, want 0 for empty dir", purged)
	}
}

func TestFlowFileAppender_PurgeExpiredFiles_NonJsonlIgnored(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetRetentionDays(1)

	oldDate := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02")
	otherPath := filepath.Join(dir, "other-"+oldDate+".txt")
	if err := os.WriteFile(otherPath, []byte("old"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	if err := os.Chtimes(otherPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes error: %v", err)
	}

	purged := a.PurgeExpiredFilesExposed()
	if purged != 0 {
		t.Errorf("purged = %d, want 0 (non-jsonl files ignored)", purged)
	}

	if _, err := os.Stat(otherPath); err != nil {
		t.Error("non-jsonl file should not be purged")
	}
}

func TestFlowFileAppender_PurgeTmpFiles(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())

	tmpPath := filepath.Join(dir, "flow-2025-01-01.jsonl.gz.tmp")
	if err := os.WriteFile(tmpPath, []byte("tmp data"), 0644); err != nil {
		t.Fatalf("write tmp error: %v", err)
	}

	normalPath := filepath.Join(dir, "flow-2025-01-01.jsonl")
	if err := os.WriteFile(normalPath, []byte("normal data"), 0644); err != nil {
		t.Fatalf("write normal error: %v", err)
	}

	a.PurgeTmpFilesExposed()

	if _, err := os.Stat(tmpPath); err == nil {
		t.Error(".gz.tmp file should be purged")
	}

	if _, err := os.Stat(normalPath); err != nil {
		t.Error("normal .jsonl file should not be purged")
	}
}

func TestFlowFileAppender_PurgeTmpFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())

	a.PurgeTmpFilesExposed()
}

func TestFlowFileAppender_PurgeTmpFiles_MultipleTmpFiles(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())

	tmp1 := filepath.Join(dir, "flow-2025-01-01.jsonl.gz.tmp")
	tmp2 := filepath.Join(dir, "alert-2025-01-01.jsonl.gz.tmp")
	if err := os.WriteFile(tmp1, []byte("tmp1"), 0644); err != nil {
		t.Fatalf("write tmp1 error: %v", err)
	}
	if err := os.WriteFile(tmp2, []byte("tmp2"), 0644); err != nil {
		t.Fatalf("write tmp2 error: %v", err)
	}

	a.PurgeTmpFilesExposed()

	if _, err := os.Stat(tmp1); err == nil {
		t.Error("tmp1 should be purged")
	}
	if _, err := os.Stat(tmp2); err == nil {
		t.Error("tmp2 should be purged")
	}
}

func TestFlowFileAppender_SyncOpenFiles(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	ev := contract.MonitorEvent{
		ID:        "env-sync",
		Type:      contract.MonitorEventTypeFlowLog,
		SessionID: "sess-sync",
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{},
	}
	a.OnMonitorEventExposed(ev)

	a.SyncOpenFilesExposed()
}

func TestFlowFileAppender_SyncOpenFiles_NoOpenFiles(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())

	a.SyncOpenFilesExposed()
}

func TestFlowFileAppender_Maintenance(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetCompressAge(1 * time.Millisecond)
	a.SetRetentionDays(1)
	defer a.CloseAllFiles()

	ev := contract.MonitorEvent{
		ID:        "env-maint",
		Type:      contract.MonitorEventTypeFlowLog,
		SessionID: "sess-maint",
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{},
	}
	a.OnMonitorEventExposed(ev)

	tmpPath := filepath.Join(dir, "stale.jsonl.gz.tmp")
	if err := os.WriteFile(tmpPath, []byte("tmp"), 0644); err != nil {
		t.Fatalf("write tmp error: %v", err)
	}

	a.MaintenanceExposed()

	if _, err := os.Stat(tmpPath); err == nil {
		t.Error(".gz.tmp file should be purged by maintenance")
	}
}

func TestFlowFileAppender_MultipleMonitorEvents(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		ev := contract.MonitorEvent{
			ID:        "env-multi-" + string(rune('0'+i)),
			Type:      contract.MonitorEventTypeFlowLog,
			SessionID: "sess-multi",
			Timestamp: now,
			Source:    "chat",
			Metadata:  map[string]any{"index": i},
		}
		a.OnMonitorEventExposed(ev)
	}
	a.SyncOpenFilesExposed()

	pattern := filepath.Join(dir, "flow-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 flow file, got %d", len(matches))
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
}

func TestFlowFileAppender_OnMonitorEvent_ContentFields(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	// Use a fixed timestamp to make _ts assertion deterministic. The new
	// MonitorEvent carries time.Time; the appender formats it as RFC3339Nano.
	ts, err := time.Parse(time.RFC3339, "2025-06-15T10:30:00Z")
	if err != nil {
		t.Fatalf("parse ts error: %v", err)
	}
	wantTS := ts.UTC().Format(time.RFC3339Nano)
	ev := contract.MonitorEvent{
		ID:        "env-content",
		Type:      contract.MonitorEventTypeFlowLog,
		SessionID: "sess-content",
		Timestamp: ts,
		Source:    "chat",
		Metadata:  map[string]any{"custom_field": 42, "nested": "val"},
		Message:   "test content",
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	pattern := filepath.Join(dir, "flow-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no flow file found")
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}

	if row["_ts"] != wantTS {
		t.Errorf("_ts = %v, want %q", row["_ts"], wantTS)
	}
	if row["_id"] != "env-content" {
		t.Errorf("_id = %v, want env-content", row["_id"])
	}
	if row["_session_id"] != "sess-content" {
		t.Errorf("_session_id = %v, want sess-content", row["_session_id"])
	}
	if row["_text"] != "test content" {
		t.Errorf("_text = %v, want test content", row["_text"])
	}
	if row["custom_field"] != float64(42) {
		t.Errorf("custom_field = %v, want 42", row["custom_field"])
	}
}

func TestFlowFileAppender_OnMonitorEvent_EmptyMessage(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	// When Message is empty, the appender omits the _text field (mirrors the
	// legacy nil-Content behavior).
	ev := contract.MonitorEvent{
		ID:        "env-no-content",
		Type:      contract.MonitorEventTypeFlowLog,
		SessionID: "sess-no-content",
		Timestamp: time.Now().UTC(),
		Source:    "chat",
		Metadata:  map[string]any{},
		Message:   "",
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	pattern := filepath.Join(dir, "flow-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no flow file found")
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}

	if _, ok := row["_text"]; ok {
		t.Error("_text should not be present when Message is empty")
	}
}

// warnCountingLogger counts Warn calls so tests can assert on log volume.
type warnCountingLogger struct {
	warnCount int
}

func (l *warnCountingLogger) Debug(string, ...loggateway.Field) {}
func (l *warnCountingLogger) Info(string, ...loggateway.Field)  {}
func (l *warnCountingLogger) Warn(string, ...loggateway.Field)  { l.warnCount++ }
func (l *warnCountingLogger) Error(string, ...loggateway.Field) {}
func (l *warnCountingLogger) With(...loggateway.Field) loggateway.Logger {
	return l
}

// Fix A: events emitted by FlowFileAppender itself (step_id prefix
// "monitor.flow_file.") must be dropped to break the write-fail → Warn →
// MonitorBus → write-fail self-feedback loop.
func TestFlowFileAppender_OnMonitorEvent_SkipsSelfOriginatedEvents(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	selfStepIDs := []string{
		"monitor.flow_file.write_fail",
		"monitor.flow_file.open_fail",
		"monitor.flow_file.mkdir_fail",
		"monitor.flow_file.write_muted",
		"monitor.flow_file.maintenance",
	}
	for i, stepID := range selfStepIDs {
		ev := contract.MonitorEvent{
			ID:        "self-" + strings.Repeat("x", i+1),
			Type:      contract.MonitorEventTypeLog,
			Timestamp: time.Now().UTC(),
			Source:    "system",
			Message:   "FlowFileAppender: write failed",
			Metadata:  map[string]any{"step_id": stepID},
		}
		a.OnMonitorEventExposed(ev)
	}
	a.SyncOpenFilesExposed()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir error: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("self-originated events must not create files, got %v", names)
	}
}

// Fix A 对照组：非自身产生的 log 事件仍然正常落盘。
func TestFlowFileAppender_OnMonitorEvent_AcceptsOtherLogEvents(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	ev := contract.MonitorEvent{
		ID:        "log-normal",
		Type:      contract.MonitorEventTypeLog,
		Timestamp: time.Now().UTC(),
		Source:    "system",
		Message:   "some other warning",
		Metadata:  map[string]any{"step_id": "chat.turn"},
	}
	a.OnMonitorEventExposed(ev)
	a.SyncOpenFilesExposed()

	matches, err := filepath.Glob(filepath.Join(dir, "log-*.jsonl"))
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("non-self log event should be written to log-*.jsonl")
	}
}

// Fix B: consecutive write failures trip a circuit breaker that mutes both
// file writes and Warn logs for a cooldown window, preventing log storms when
// the disk is full / the file is unwritable.
func TestFlowFileAppender_WriteFailureCircuitBreaker(t *testing.T) {
	dir := t.TempDir()
	lg := &warnCountingLogger{}
	a := monitor.NewFlowFileAppender(dir, lg)
	defer a.CloseAllFiles()

	badEv := func(id string) contract.MonitorEvent {
		return contract.MonitorEvent{
			ID:        id,
			Type:      contract.MonitorEventTypeFlowLog,
			Timestamp: time.Now().UTC(),
			Source:    "chat",
			// json.Encoder.Encode fails on chan values → deterministic write failure.
			Metadata: map[string]any{"bad": make(chan int)},
		}
	}

	// First threshold-1 failures each emit one Warn; the failure that reaches
	// the threshold emits exactly one "muted" Warn and opens the circuit.
	for i := 0; i < 3; i++ {
		a.OnMonitorEventExposed(badEv("bad-" + strings.Repeat("x", i+1)))
	}
	warnsAtTrip := lg.warnCount
	if warnsAtTrip != 3 {
		t.Fatalf("warn count at circuit trip = %d, want 3 (2 write_fail + 1 muted)", warnsAtTrip)
	}

	// While muted, further failures are dropped silently.
	for i := 0; i < 5; i++ {
		a.OnMonitorEventExposed(badEv("bad-muted-" + strings.Repeat("x", i+1)))
	}
	if lg.warnCount != warnsAtTrip {
		t.Errorf("warn count after muted failures = %d, want %d (no new warns while muted)", lg.warnCount, warnsAtTrip)
	}

	// While muted, even valid writes are dropped.
	goodEv := contract.MonitorEvent{
		ID:        "good-1",
		Type:      contract.MonitorEventTypeFlowLog,
		Timestamp: time.Now().UTC(),
		Source:    "chat",
		Metadata:  map[string]any{"key": "value"},
	}
	a.OnMonitorEventExposed(goodEv)
	a.SyncOpenFilesExposed()

	matches, err := filepath.Glob(filepath.Join(dir, "flow-*.jsonl"))
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("flow file should exist (created before circuit opened)")
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if len(strings.TrimSpace(string(data))) != 0 {
		t.Errorf("flow file should be empty (all writes failed or muted), got %q", string(data))
	}
}

// P1: open failures (bad dir / full disk) must feed the same circuit breaker
// as write failures; otherwise every incoming event emits an unthrottled
// open_fail Warn for as long as the directory stays unwritable.
func TestFlowFileAppender_OpenFailureCircuitBreaker(t *testing.T) {
	// dir points at a regular file → every os.OpenFile beneath it fails.
	tmp := t.TempDir()
	fileAsDir := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	lg := &warnCountingLogger{}
	a := monitor.NewFlowFileAppender(fileAsDir, lg)

	ev := func(id string) contract.MonitorEvent {
		return contract.MonitorEvent{
			ID:        id,
			Type:      contract.MonitorEventTypeLog,
			Timestamp: time.Now().UTC(),
			Source:    "system",
			Metadata:  map[string]any{"step_id": "chat.turn"},
		}
	}
	for i := 0; i < 10; i++ {
		a.OnMonitorEventExposed(ev("ev-" + strconv.Itoa(i)))
	}
	if lg.warnCount != 3 {
		t.Errorf("warn count = %d after 10 open failures, want 3 (2 open_fail + 1 muted, then silent)", lg.warnCount)
	}
}

// P1 恢复路径：熔断窗口过期后，下一个事件作为 half-open 探针重试打开文件，
// 失败时记一条 Warn 并重新计数（不静默、不刷屏）。
func TestFlowFileAppender_OpenFailureCircuitBreaker_HalfOpenProbe(t *testing.T) {
	tmp := t.TempDir()
	fileAsDir := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	lg := &warnCountingLogger{}
	a := monitor.NewFlowFileAppender(fileAsDir, lg)

	ev := contract.MonitorEvent{
		ID:        "ev",
		Type:      contract.MonitorEventTypeLog,
		Timestamp: time.Now().UTC(),
		Source:    "system",
		Metadata:  map[string]any{"step_id": "chat.turn"},
	}
	for i := 0; i < 3; i++ {
		a.OnMonitorEventExposed(ev)
	}
	if lg.warnCount != 3 {
		t.Fatalf("warn count at trip = %d, want 3", lg.warnCount)
	}

	// Expire the mute window → the next event probes the directory again and
	// its failure is logged once (streak restarts at 1).
	a.SetWriteMutedUntilForTest(time.Now().Add(-time.Second))
	a.OnMonitorEventExposed(ev)
	if lg.warnCount != 4 {
		t.Errorf("warn count after half-open probe = %d, want 4", lg.warnCount)
	}
}

// Fix C: rotated backup files (prefix-date.jsonl.N) are capped at maxBackups
// per base name; the oldest excess backups are purged by maintenance.
func TestFlowFileAppender_PurgeExcessBackups(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetMaxBackups(5)

	today := time.Now().UTC().Format("2006-01-02")
	base := "flow-" + today + ".jsonl"
	if err := os.WriteFile(filepath.Join(dir, base), []byte("base"), 0644); err != nil {
		t.Fatalf("write base error: %v", err)
	}
	for seq := 2; seq <= 12; seq++ {
		p := filepath.Join(dir, base+"."+strconv.Itoa(seq))
		if err := os.WriteFile(p, []byte("backup"), 0644); err != nil {
			t.Fatalf("write backup %d error: %v", seq, err)
		}
	}

	purged := a.PurgeExcessBackupsExposed()
	if purged != 6 {
		t.Errorf("purged = %d, want 6 (seq 2..7 removed, keeping newest 5)", purged)
	}

	for seq := 2; seq <= 7; seq++ {
		p := filepath.Join(dir, base+"."+strconv.Itoa(seq))
		if _, err := os.Stat(p); err == nil {
			t.Errorf("old backup seq %d should be purged", seq)
		}
	}
	for seq := 8; seq <= 12; seq++ {
		p := filepath.Join(dir, base+"."+strconv.Itoa(seq))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("recent backup seq %d should be kept: %v", seq, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, base)); err != nil {
		t.Error("base file should be kept")
	}
}

// Fix C: retention purge also catches rotated backups (.jsonl.N / .jsonl.N.gz),
// which previously lived forever because the suffix filter missed them.
func TestFlowFileAppender_PurgeExpiredFiles_RotatedBackups(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	a.SetRetentionDays(1)

	oldDate := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02")
	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	files := []string{
		"flow-" + oldDate + ".jsonl.2",
		"flow-" + oldDate + ".jsonl.3.gz",
	}
	for _, name := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("old"), 0644); err != nil {
			t.Fatalf("write %s error: %v", name, err)
		}
		if err := os.Chtimes(p, oldTime, oldTime); err != nil {
			t.Fatalf("chtimes %s error: %v", name, err)
		}
	}

	purged := a.PurgeExpiredFilesExposed()
	if purged != 2 {
		t.Errorf("purged = %d, want 2 (rotated backups included in retention)", purged)
	}
	for _, name := range files {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("expired rotated backup %s should be purged", name)
		}
	}
}
