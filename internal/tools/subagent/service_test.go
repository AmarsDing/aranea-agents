package subagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 3); got != "hel" {
		t.Fatalf("expected hel, got %q", got)
	}
}

func TestTruncateRunes_NoLimit(t *testing.T) {
	if got := truncateRunes("hello", 0); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestTruncateRunes_UnderLimit(t *testing.T) {
	if got := truncateRunes("hi", 10); got != "hi" {
		t.Fatalf("expected hi, got %q", got)
	}
}

func TestTruncateRunes_TrimSpace(t *testing.T) {
	if got := truncateRunes("  hello  ", 10); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestSanitizeStoredResult(t *testing.T) {
	long := strings.Repeat("x", 5000)
	got := sanitizeStoredResult(long, defaultStoredResultRunes)
	if len([]rune(got)) != defaultStoredResultRunes {
		t.Fatalf("expected %d runes, got %d", defaultStoredResultRunes, len([]rune(got)))
	}
}

func TestSummarizeResult(t *testing.T) {
	long := strings.Repeat("y", 300)
	got := summarizeResult(long, defaultStoredSummaryRunes)
	if len([]rune(got)) != defaultStoredSummaryRunes {
		t.Fatalf("expected %d runes, got %d", defaultStoredSummaryRunes, len([]rune(got)))
	}
}

func TestNewChildSessionID(t *testing.T) {
	now := time.Now()
	id := newChildSessionID("run-123", now)
	if !strings.HasPrefix(id, subagentSessionPrefix) {
		t.Fatalf("expected prefix %q, got %q", subagentSessionPrefix, id)
	}
	if !strings.Contains(id, "run-123") {
		t.Fatalf("expected run-123 in id, got %q", id)
	}
}

func TestNewRequestID(t *testing.T) {
	now := time.Now()
	id := newRequestID("run-456", now)
	if !strings.HasPrefix(id, subagentRequestPrefix) {
		t.Fatalf("expected prefix %q, got %q", subagentRequestPrefix, id)
	}
}

func TestCloneTime(t *testing.T) {
	now := time.Now()
	p := cloneTime(now)
	if p == nil {
		t.Fatal("expected non-nil")
	}
	if !p.Equal(now) {
		t.Fatal("expected equal time")
	}
}

func TestNormalizeLoadedRuns_NonTerminal(t *testing.T) {
	now := time.Now()
	runs := map[string]*runRecord{
		"r1": {Run: trpcsubagent.Run{ID: "r1", Status: trpcsubagent.StatusQueued}},
	}
	changed := normalizeLoadedRuns(runs, now)
	if !changed {
		t.Fatal("expected changed for non-terminal run")
	}
	if runs["r1"].Status != trpcsubagent.StatusFailed {
		t.Fatalf("expected failed, got %q", runs["r1"].Status)
	}
	if runs["r1"].Error != "interrupted" {
		t.Fatalf("expected interrupted, got %q", runs["r1"].Error)
	}
}

func TestNormalizeLoadedRuns_TerminalUnchanged(t *testing.T) {
	now := time.Now()
	runs := map[string]*runRecord{
		"r1": {Run: trpcsubagent.Run{ID: "r1", Status: trpcsubagent.StatusCompleted, CreatedAt: now, UpdatedAt: now}},
	}
	changed := normalizeLoadedRuns(runs, now)
	if changed {
		t.Fatal("terminal run should not be changed")
	}
}

func TestNormalizeLoadedRuns_ZeroTimes(t *testing.T) {
	now := time.Now()
	runs := map[string]*runRecord{
		"r1": {Run: trpcsubagent.Run{ID: "r1", Status: trpcsubagent.StatusCompleted}},
	}
	changed := normalizeLoadedRuns(runs, now)
	if !changed {
		t.Fatal("zero times should be changed")
	}
	if runs["r1"].CreatedAt.IsZero() || runs["r1"].UpdatedAt.IsZero() {
		t.Fatal("zero times should be filled")
	}
}

func TestLoadRuns_MissingFile(t *testing.T) {
	runs, err := loadRuns(filepath.Join(t.TempDir(), "missing.json"), loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected empty, got %d", len(runs))
	}
}

func TestLoadRuns_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "runs.json")
	os.WriteFile(p, []byte{}, 0o644)
	runs, err := loadRuns(p, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected empty, got %d", len(runs))
	}
}

func TestSaveAndLoadRuns(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "subagents", "runs.json")
	now := time.Now()
	runs := map[string]*runRecord{
		"r1": {Run: trpcsubagent.Run{ID: "r1", Status: trpcsubagent.StatusCompleted, CreatedAt: now, UpdatedAt: now}, OwnerUserID: "user-1"},
	}
	if err := saveRuns(p, runs); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRuns(p, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	if loaded["r1"].ID != "r1" {
		t.Fatalf("expected r1, got %q", loaded["r1"].ID)
	}
	if loaded["r1"].OwnerUserID != "user-1" {
		t.Fatalf("expected user-1, got %q", loaded["r1"].OwnerUserID)
	}
}

func TestNewService_EmptyStateDir(t *testing.T) {
	_, err := NewService("", nil, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty state dir")
	}
}

func TestNewService_NilRunner(t *testing.T) {
	// Nil runner is allowed at construction; SetRunner is called later at runtime.
	svc, err := NewService(t.TempDir(), nil, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	// Spawn should fail gracefully when runner is not yet configured.
	svc.Start(context.Background())
	_, spawnErr := svc.Spawn(context.Background(), SpawnRequest{
		OwnerUserID:    "u1",
		ParentSessionID: "s1",
		Task:           "do something",
	})
	if spawnErr == nil {
		t.Fatal("expected error when spawning with nil runner")
	}
}

func TestService_ListForUser_NilService(t *testing.T) {
	var s *Service
	if s.ListForUser("user", trpcsubagent.ListFilter{}) != nil {
		t.Fatal("nil service should return nil")
	}
}

func TestService_GetForUser_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.GetForUser("user", "nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestService_CancelForUser_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, _, err = svc.CancelForUser("user", "nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestService_Spawn_NilService(t *testing.T) {
	var s *Service
	_, err := s.Spawn(context.Background(), SpawnRequest{})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestService_Spawn_NotStarted(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "s", Task: "t"})
	if err == nil {
		t.Fatal("expected error for not started")
	}
}

func TestService_Spawn_EmptyOwner(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "", ParentSessionID: "s", Task: "t"})
	if err == nil {
		t.Fatal("expected error for empty owner")
	}
}

func TestService_Spawn_EmptyParentSession(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "", Task: "t"})
	if err == nil {
		t.Fatal("expected error for empty parent session")
	}
}

func TestService_Spawn_EmptyTask(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "s", Task: ""})
	if err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestService_Close_NilService(t *testing.T) {
	var s *Service
	if err := s.Close(); err != nil {
		t.Fatalf("nil service Close should not error: %v", err)
	}
}

func TestService_Start_NilService(t *testing.T) {
	var s *Service
	s.Start(context.Background())
}

func TestRunRecord_PublicView_Nil(t *testing.T) {
	var r *runRecord
	if v := r.publicView(); v.ID != "" {
		t.Fatal("nil record should return zero Run")
	}
}

func TestRunRecord_Clone_Nil(t *testing.T) {
	var r *runRecord
	if c := r.clone(); c != nil {
		t.Fatal("nil clone should return nil")
	}
}

func TestRunRecord_Clone_DeepCopy(t *testing.T) {
	now := time.Now()
	r := &runRecord{
		Run: trpcsubagent.Run{
			ID:         "r1",
			StartedAt:  &now,
			FinishedAt: &now,
		},
	}
	c := r.clone()
	if c == r {
		t.Fatal("clone should be different pointer")
	}
	if c.StartedAt == r.StartedAt {
		t.Fatal("StartedAt should be deep copied")
	}
}

func TestReplyAccumulator_NilEvent(t *testing.T) {
	a := replyAccumulator{}
	a.consume(nil)
	if a.text != "" {
		t.Fatal("nil event should not produce text")
	}
}

func TestDecodeRunIDArgs_InvalidJSON(t *testing.T) {
	_, _, err := decodeRunIDArgs(context.Background(), []byte(`not json`), loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestDecodeRunIDArgs_EmptyID(t *testing.T) {
	_, _, err := decodeRunIDArgs(context.Background(), []byte(`{"id":""}`), loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

type stubRunner struct{}

func (s *stubRunner) Run(_ context.Context, _ string, _ string, _ trpcmodel.Message, _ ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
	ch := make(chan *trpcevent.Event)
	close(ch)
	return ch, nil
}

func (s *stubRunner) Close() error { return nil }
