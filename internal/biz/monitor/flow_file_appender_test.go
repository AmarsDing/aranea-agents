package monitor_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	a.Start(ctx)
	time.Sleep(50 * time.Millisecond)
}

func TestNewFlowFileAppender_StartNilAppender(t *testing.T) {
	var a *monitor.FlowFileAppender
	a.Start(context.Background())
}

func TestFlowFileAppender_OnEnvelope_FlowLog(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	env := contract.Envelope{
		ID:        "env-1",
		Type:      contract.EnvelopeTypeFlowLog,
		SessionID: "sess-1",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Channel:   "chat",
		Metadata:  map[string]any{"key1": "value1"},
		Content:   &contract.EnvelopeContent{Text: "hello"},
	}
	a.OnEnvelopeExposed(env)
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

func TestFlowFileAppender_OnEnvelope_SystemLog(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	env := contract.Envelope{
		ID:        "env-sys",
		Type:      contract.EnvelopeTypeFlowLog,
		SessionID: "sess-2",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Channel:   "monitor",
		Metadata:  map[string]any{},
	}
	a.OnEnvelopeExposed(env)
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

func TestFlowFileAppender_OnEnvelope_AlertNotify(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	env := contract.Envelope{
		ID:        "env-alert",
		Type:      contract.EnvelopeTypeAlertNotify,
		SessionID: "sess-3",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Metadata:  map[string]any{"alert_key": "test"},
	}
	a.OnEnvelopeExposed(env)
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

func TestFlowFileAppender_OnEnvelope_MCPHealthAlert(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	env := contract.Envelope{
		ID:        "env-mcp",
		Type:      contract.EnvelopeTypeMCPHealthAlert,
		SessionID: "sess-4",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Metadata:  map[string]any{},
	}
	a.OnEnvelopeExposed(env)
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

func TestFlowFileAppender_OnEnvelope_RunnerCompletion(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	env := contract.Envelope{
		ID:        "env-trace",
		Type:      contract.EnvelopeTypeRunnerCompletion,
		SessionID: "sess-5",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Metadata:  map[string]any{},
	}
	a.OnEnvelopeExposed(env)
	a.SyncOpenFilesExposed()

	pattern := filepath.Join(dir, "trace-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no trace file found in %q", dir)
	}
}

func TestFlowFileAppender_OnEnvelope_NilMetadata(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	env := contract.Envelope{
		ID:        "env-nil",
		Type:      contract.EnvelopeTypeFlowLog,
		SessionID: "sess-6",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Metadata:  nil,
	}
	a.OnEnvelopeExposed(env)
	a.SyncOpenFilesExposed()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files when Metadata is nil, got %d", len(entries))
	}
}

func TestFlowFileAppender_OnEnvelope_NilAppender(t *testing.T) {
	var a *monitor.FlowFileAppender
	env := contract.Envelope{
		ID:       "env-nil-appender",
		Type:     contract.EnvelopeTypeFlowLog,
		Metadata: map[string]any{},
	}
	a.OnEnvelopeExposed(env)
}

func TestFlowFileAppender_OnEnvelope_UnknownType(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	env := contract.Envelope{
		ID:        "env-unknown",
		Type:      contract.EnvelopeTypeTextDelta,
		SessionID: "sess-7",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Metadata:  map[string]any{},
	}
	a.OnEnvelopeExposed(env)
	a.SyncOpenFilesExposed()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files for unknown envelope type, got %d", len(entries))
	}
}

func TestFlowFileAppender_OnEnvelope_RoutesToCorrectFiles(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	envs := []contract.Envelope{
		{ID: "f1", Type: contract.EnvelopeTypeFlowLog, SessionID: "s1", Timestamp: time.Now().UTC().Format(time.RFC3339), Channel: "chat", Metadata: map[string]any{}},
		{ID: "s1", Type: contract.EnvelopeTypeFlowLog, SessionID: "s2", Timestamp: time.Now().UTC().Format(time.RFC3339), Channel: "monitor", Metadata: map[string]any{}},
		{ID: "a1", Type: contract.EnvelopeTypeAlertNotify, SessionID: "s3", Timestamp: time.Now().UTC().Format(time.RFC3339), Metadata: map[string]any{}},
		{ID: "t1", Type: contract.EnvelopeTypeRunnerCompletion, SessionID: "s4", Timestamp: time.Now().UTC().Format(time.RFC3339), Metadata: map[string]any{}},
	}
	for _, env := range envs {
		a.OnEnvelopeExposed(env)
	}
	a.SyncOpenFilesExposed()

	paths := a.RotatingFilePaths()
	if len(paths) != 4 {
		t.Errorf("RotatingFilePaths() = %d files, want 4", len(paths))
	}

	prefixes := map[string]bool{}
	for _, p := range paths {
		prefix := strings.SplitN(p, "-", 2)[0]
		prefixes[prefix] = true
	}
	for _, want := range []string{"flow", "system", "alert", "trace"} {
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

	env := contract.Envelope{
		ID:        "env-sync",
		Type:      contract.EnvelopeTypeFlowLog,
		SessionID: "sess-sync",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Metadata:  map[string]any{},
	}
	a.OnEnvelopeExposed(env)

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

	env := contract.Envelope{
		ID:        "env-maint",
		Type:      contract.EnvelopeTypeFlowLog,
		SessionID: "sess-maint",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Metadata:  map[string]any{},
	}
	a.OnEnvelopeExposed(env)

	tmpPath := filepath.Join(dir, "stale.jsonl.gz.tmp")
	if err := os.WriteFile(tmpPath, []byte("tmp"), 0644); err != nil {
		t.Fatalf("write tmp error: %v", err)
	}

	a.MaintenanceExposed()

	if _, err := os.Stat(tmpPath); err == nil {
		t.Error(".gz.tmp file should be purged by maintenance")
	}
}

func TestFlowFileAppender_MultipleEnvelopes(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	for i := 0; i < 5; i++ {
		env := contract.Envelope{
			ID:        "env-multi-" + string(rune('0'+i)),
			Type:      contract.EnvelopeTypeFlowLog,
			SessionID: "sess-multi",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Channel:   "chat",
			Metadata:  map[string]any{"index": i},
		}
		a.OnEnvelopeExposed(env)
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

func TestFlowFileAppender_OnEnvelope_ContentFields(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	ts := "2025-06-15T10:30:00Z"
	env := contract.Envelope{
		ID:        "env-content",
		Type:      contract.EnvelopeTypeFlowLog,
		SessionID: "sess-content",
		Timestamp: ts,
		Channel:   "chat",
		Metadata:  map[string]any{"custom_field": 42, "nested": "val"},
		Content:   &contract.EnvelopeContent{Text: "test content"},
	}
	a.OnEnvelopeExposed(env)
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

	if row["_ts"] != ts {
		t.Errorf("_ts = %v, want %q", row["_ts"], ts)
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

func TestFlowFileAppender_OnEnvelope_NilContent(t *testing.T) {
	dir := t.TempDir()
	a := monitor.NewFlowFileAppender(dir, loggateway.NewNoop())
	defer a.CloseAllFiles()

	env := contract.Envelope{
		ID:        "env-no-content",
		Type:      contract.EnvelopeTypeFlowLog,
		SessionID: "sess-no-content",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Channel:   "chat",
		Metadata:  map[string]any{},
		Content:   nil,
	}
	a.OnEnvelopeExposed(env)
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
		t.Error("_text should not be present when Content is nil")
	}
}
