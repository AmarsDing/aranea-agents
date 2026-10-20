package subagent

import (
	"aranea-agents/pkg/loggateway"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
