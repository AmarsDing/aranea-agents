package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// mocks
// ---------------------------------------------------------------------------

type mockL1IdleTaskReader struct {
	listFn func(ctx context.Context, cutoffRFC3339 string) ([][]byte, error)
}

func (m *mockL1IdleTaskReader) ListIdleL1Tasks(ctx context.Context, cutoffRFC3339 string) ([][]byte, error) {
	if m.listFn != nil {
		return m.listFn(ctx, cutoffRFC3339)
	}
	return nil, nil
}

type mockL1TaskWriter struct {
	endFn      func(ctx context.Context, sessionID, taskID, status string) ([]byte, error)
	archiveFn  func(ctx context.Context, sessionID, taskID string, episode biz.L1ArchiveEpisodeInsert) ([]byte, error)
	startFn    func(ctx context.Context, in biz.L1TaskInsert) ([]byte, error)
	getFn      func(ctx context.Context, sessionID, id string) ([]byte, error)
	archFn     func(ctx context.Context, sessionID, taskID string) ([]byte, error)
	unarchFn   func(ctx context.Context, sessionID, taskID string) error
}

func (m *mockL1TaskWriter) StartL1Task(ctx context.Context, in biz.L1TaskInsert) ([]byte, error) {
	if m.startFn != nil {
		return m.startFn(ctx, in)
	}
	return nil, nil
}

func (m *mockL1TaskWriter) EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error) {
	if m.endFn != nil {
		return m.endFn(ctx, sessionID, taskID, status)
	}
	return nil, nil
}

func (m *mockL1TaskWriter) GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	if m.getFn != nil {
		return m.getFn(ctx, sessionID, id)
	}
	return nil, nil
}

func (m *mockL1TaskWriter) ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	if m.archFn != nil {
		return m.archFn(ctx, sessionID, taskID)
	}
	return nil, nil
}

func (m *mockL1TaskWriter) UnarchiveL1Task(ctx context.Context, sessionID, taskID string) error {
	if m.unarchFn != nil {
		return m.unarchFn(ctx, sessionID, taskID)
	}
	return nil
}

func (m *mockL1TaskWriter) ArchiveAndCreateEpisodeTx(ctx context.Context, sessionID, taskID string, episode biz.L1ArchiveEpisodeInsert) ([]byte, error) {
	if m.archiveFn != nil {
		return m.archiveFn(ctx, sessionID, taskID, episode)
	}
	return nil, nil
}

type mockL1ExpiredFieldCleaner struct {
	deleteFn func(ctx context.Context) (int, error)
}

func (m *mockL1ExpiredFieldCleaner) DeleteExpiredL1Fields(ctx context.Context) (int, error) {
	if m.deleteFn != nil {
		return m.deleteFn(ctx)
	}
	return 0, nil
}

// compositeStore embeds all SessionAdminStore sub-interfaces except
// L1AdminReader (whose GetL1TaskRow/GetL1FieldRow overlap with L1TaskWriter/
// L1FieldWriter and cause ambiguous selector errors). The 2 non-overlapping
// L1AdminReader methods are stubbed manually. Only the 3 interfaces the worker
// actually uses (L1IdleTaskReader, L1TaskWriter, L1ExpiredFieldCleaner) are
// populated; the rest remain nil and are never called.
type compositeStore struct {
	biz.L0AdminStore
	biz.L1TaskWriter
	biz.L1FieldWriter
	biz.L1IdleTaskReader
	biz.L1ExpiredFieldCleaner
	biz.L2EpisodeWriter
	biz.L2RecallStore
	biz.L3FactReader
	biz.L3FactWriter
	biz.L3ConflictStore
	biz.PIIReviewStore
	biz.L4EntityStore
	biz.L4EvolutionStore
}

// ListL1TaskRows stubs the L1AdminReader method (not used by the worker).
func (c *compositeStore) ListL1TaskRows(_ context.Context, _, _, _, _ string) ([][]byte, error) {
	return nil, nil
}

// ListL1FieldRows stubs the L1AdminReader method (not used by the worker).
func (c *compositeStore) ListL1FieldRows(_ context.Context, _ string, _ bool, _ ...string) ([][]byte, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestWorker(t *testing.T, interval time.Duration, idleReader biz.L1IdleTaskReader, writer biz.L1TaskWriter, cleaner biz.L1ExpiredFieldCleaner) *jobs.MemoryL1ArchiveWorker {
	t.Helper()
	store := &compositeStore{
		L1IdleTaskReader:     idleReader,
		L1TaskWriter:         writer,
		L1ExpiredFieldCleaner: cleaner,
	}
	return jobs.NewMemoryL1ArchiveWorker(interval, store, nil, loggateway.NewNoop())
}

func taskJSON(id, sessionID, agentID string) []byte {
	b, _ := json.Marshal(map[string]string{
		"id":         id,
		"session_id": sessionID,
		"agent_id":   agentID,
	})
	return b
}

// ---------------------------------------------------------------------------
// tests: cleanupExpiredFields
// ---------------------------------------------------------------------------

func TestMemoryL1Archive_CleanupDeletesExpiredFields(t *testing.T) {
	var deleted int
	cleaner := &mockL1ExpiredFieldCleaner{
		deleteFn: func(_ context.Context) (int, error) {
			deleted = 3
			return 3, nil
		},
	}
	w := newTestWorker(t, 0, &mockL1IdleTaskReader{}, &mockL1TaskWriter{}, cleaner)
	w.RunOnceExposed(context.Background())

	if deleted != 3 {
		t.Errorf("expected cleaner to be called, deleted=%d", deleted)
	}
}

func TestMemoryL1Archive_CleanupErrorDoesNotPanic(t *testing.T) {
	cleaner := &mockL1ExpiredFieldCleaner{
		deleteFn: func(_ context.Context) (int, error) {
			return 0, errors.New("db unavailable")
		},
	}
	w := newTestWorker(t, 0, &mockL1IdleTaskReader{}, &mockL1TaskWriter{}, cleaner)

	// Should not panic.
	w.RunOnceExposed(context.Background())
}

func TestMemoryL1Archive_CleanupZeroDeletionsNoError(t *testing.T) {
	called := false
	cleaner := &mockL1ExpiredFieldCleaner{
		deleteFn: func(_ context.Context) (int, error) {
			called = true
			return 0, nil
		},
	}
	w := newTestWorker(t, 0, &mockL1IdleTaskReader{}, &mockL1TaskWriter{}, cleaner)
	w.RunOnceExposed(context.Background())

	if !called {
		t.Error("expected cleaner to be called even with 0 deletions")
	}
}

// ---------------------------------------------------------------------------
// tests: archiveIdleTasks
// ---------------------------------------------------------------------------

func TestMemoryL1Archive_ArchivesIdleTasks(t *testing.T) {
	tasks := [][]byte{
		taskJSON("task-1", "sess-1", "agent-1"),
		taskJSON("task-2", "sess-2", "agent-2"),
	}
	idleReader := &mockL1IdleTaskReader{
		listFn: func(_ context.Context, _ string) ([][]byte, error) {
			return tasks, nil
		},
	}

	var endedTasks []string
	var archivedEpisodes []string
	writer := &mockL1TaskWriter{
		endFn: func(_ context.Context, _, taskID, status string) ([]byte, error) {
			endedTasks = append(endedTasks, taskID+":"+status)
			return []byte("{}"), nil
		},
		archiveFn: func(_ context.Context, _, taskID string, ep biz.L1ArchiveEpisodeInsert) ([]byte, error) {
			archivedEpisodes = append(archivedEpisodes, ep.TaskID+":"+ep.SessionID+":"+ep.AgentID)
			return []byte("{}"), nil
		},
	}

	w := newTestWorker(t, 0, idleReader, writer, &mockL1ExpiredFieldCleaner{})
	w.RunOnceExposed(context.Background())

	if len(endedTasks) != 2 {
		t.Errorf("expected 2 EndL1Task calls, got %d", len(endedTasks))
	}
	if len(archivedEpisodes) != 2 {
		t.Errorf("expected 2 ArchiveAndCreateEpisodeTx calls, got %d", len(archivedEpisodes))
	}

	// Verify task-1 episode metadata.
	if len(archivedEpisodes) > 0 {
		want := "task-1:sess-1:agent-1"
		if archivedEpisodes[0] != want {
			t.Errorf("episode[0]=%q, want %q", archivedEpisodes[0], want)
		}
	}

	// Verify status passed to EndL1Task is "cancelled".
	if len(endedTasks) > 0 && endedTasks[0] != "task-1:cancelled" {
		t.Errorf("endL1Task[0]=%q, want 'task-1:cancelled'", endedTasks[0])
	}
}

func TestMemoryL1Archive_ListErrorDoesNotPanic(t *testing.T) {
	idleReader := &mockL1IdleTaskReader{
		listFn: func(_ context.Context, _ string) ([][]byte, error) {
			return nil, errors.New("db error")
		},
	}
	endCalled := false
	writer := &mockL1TaskWriter{
		endFn: func(_ context.Context, _, _, _ string) ([]byte, error) {
			endCalled = true
			return nil, nil
		},
	}
	w := newTestWorker(t, 0, idleReader, writer, &mockL1ExpiredFieldCleaner{})
	w.RunOnceExposed(context.Background())

	if endCalled {
		t.Error("EndL1Task should not be called when list fails")
	}
}

func TestMemoryL1Archive_EndErrorSkipsEpisode(t *testing.T) {
	tasks := [][]byte{
		taskJSON("task-1", "sess-1", "agent-1"),
		taskJSON("task-2", "sess-2", "agent-2"),
	}
	idleReader := &mockL1IdleTaskReader{
		listFn: func(_ context.Context, _ string) ([][]byte, error) {
			return tasks, nil
		},
	}

	var archiveCalls []string
	writer := &mockL1TaskWriter{
		endFn: func(_ context.Context, _, taskID, _ string) ([]byte, error) {
			if taskID == "task-1" {
				return nil, errors.New("end failed")
			}
			return []byte("{}"), nil
		},
		archiveFn: func(_ context.Context, _, taskID string, _ biz.L1ArchiveEpisodeInsert) ([]byte, error) {
			archiveCalls = append(archiveCalls, taskID)
			return []byte("{}"), nil
		},
	}

	w := newTestWorker(t, 0, idleReader, writer, &mockL1ExpiredFieldCleaner{})
	w.RunOnceExposed(context.Background())

	// task-1 should be skipped (EndL1Task failed); task-2 should be archived.
	if len(archiveCalls) != 1 || archiveCalls[0] != "task-2" {
		t.Errorf("archiveCalls=%v, want [task-2]", archiveCalls)
	}
}

func TestMemoryL1Archive_EpisodeErrorContinues(t *testing.T) {
	tasks := [][]byte{
		taskJSON("task-1", "sess-1", "agent-1"),
		taskJSON("task-2", "sess-2", "agent-2"),
	}
	idleReader := &mockL1IdleTaskReader{
		listFn: func(_ context.Context, _ string) ([][]byte, error) {
			return tasks, nil
		},
	}

	var archiveCalls []string
	writer := &mockL1TaskWriter{
		endFn: func(_ context.Context, _, _, _ string) ([]byte, error) {
			return []byte("{}"), nil
		},
		archiveFn: func(_ context.Context, _, taskID string, _ biz.L1ArchiveEpisodeInsert) ([]byte, error) {
			archiveCalls = append(archiveCalls, taskID)
			if taskID == "task-1" {
				return nil, errors.New("episode insert failed")
			}
			return []byte("{}"), nil
		},
	}

	w := newTestWorker(t, 0, idleReader, writer, &mockL1ExpiredFieldCleaner{})
	w.RunOnceExposed(context.Background())

	// Both tasks should attempt archive; task-1 fails but task-2 should still proceed.
	if len(archiveCalls) != 2 {
		t.Errorf("archiveCalls=%v, expected 2 attempts", archiveCalls)
	}
}

func TestMemoryL1Archive_EmptyTaskListNoCalls(t *testing.T) {
	idleReader := &mockL1IdleTaskReader{
		listFn: func(_ context.Context, _ string) ([][]byte, error) {
			return nil, nil
		},
	}
	endCalled := false
	archiveCalled := false
	writer := &mockL1TaskWriter{
		endFn: func(_ context.Context, _, _, _ string) ([]byte, error) {
			endCalled = true
			return nil, nil
		},
		archiveFn: func(_ context.Context, _, _ string, _ biz.L1ArchiveEpisodeInsert) ([]byte, error) {
			archiveCalled = true
			return nil, nil
		},
	}

	w := newTestWorker(t, 0, idleReader, writer, &mockL1ExpiredFieldCleaner{})
	w.RunOnceExposed(context.Background())

	if endCalled {
		t.Error("EndL1Task should not be called for empty task list")
	}
	if archiveCalled {
		t.Error("ArchiveAndCreateEpisodeTx should not be called for empty task list")
	}
}

func TestMemoryL1Archive_SkipsInvalidRows(t *testing.T) {
	tasks := [][]byte{
		// Missing id.
		taskJSON("", "sess-1", "agent-1"),
		// Missing session_id.
		taskJSON("task-2", "", "agent-2"),
		// Valid.
		taskJSON("task-3", "sess-3", "agent-3"),
	}
	idleReader := &mockL1IdleTaskReader{
		listFn: func(_ context.Context, _ string) ([][]byte, error) {
			return tasks, nil
		},
	}

	var endCalls []string
	writer := &mockL1TaskWriter{
		endFn: func(_ context.Context, _, taskID, _ string) ([]byte, error) {
			endCalls = append(endCalls, taskID)
			return []byte("{}"), nil
		},
		archiveFn: func(_ context.Context, _, _ string, _ biz.L1ArchiveEpisodeInsert) ([]byte, error) {
			return []byte("{}"), nil
		},
	}

	w := newTestWorker(t, 0, idleReader, writer, &mockL1ExpiredFieldCleaner{})
	w.RunOnceExposed(context.Background())

	// Only task-3 should be processed.
	if len(endCalls) != 1 || endCalls[0] != "task-3" {
		t.Errorf("endCalls=%v, want [task-3]", endCalls)
	}
}

// ---------------------------------------------------------------------------
// tests: constructor & env
// ---------------------------------------------------------------------------

func TestNewMemoryL1ArchiveWorker_DefaultInterval(t *testing.T) {
	w := newTestWorker(t, 0, &mockL1IdleTaskReader{}, &mockL1TaskWriter{}, &mockL1ExpiredFieldCleaner{})
	if w == nil {
		t.Fatal("expected non-nil worker")
	}
	// Verify default interval is applied by checking that Start doesn't immediately
	// return (it would if interval were invalid). We just verify construction succeeds.
}

func TestMemoryL1ArchiveDisabled_EnvVar(t *testing.T) {
	tests := []struct {
		env string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
		{"1", true},
		{"true", true},
		{"YES", true},
		{"True", true},
	}
	for _, tt := range tests {
		t.Setenv("MEMORY_L1_ARCHIVE_DISABLED", tt.env)
		if got := jobs.MemoryL1ArchiveDisabled(); got != tt.want {
			t.Errorf("env=%q: got %v, want %v", tt.env, got, tt.want)
		}
	}
}

func TestMemoryL1ArchiveDisabled_DefaultNotDisabled(t *testing.T) {
	t.Setenv("MEMORY_L1_ARCHIVE_DISABLED", "")
	if jobs.MemoryL1ArchiveDisabled() {
		t.Error("worker should not be disabled by default")
	}
}

// ---------------------------------------------------------------------------
// tests: Start lifecycle
// ---------------------------------------------------------------------------

func TestMemoryL1Archive_Start_NilStoreReturns(t *testing.T) {
	// A worker constructed with a nil store should return immediately from Start.
	w := jobs.NewMemoryL1ArchiveWorker(0, nil, nil, loggateway.NewNoop())
	if w == nil {
		t.Fatal("expected non-nil worker")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should return immediately because store is nil.
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good: returned immediately.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Start should return immediately when store is nil")
	}
}

func TestMemoryL1Archive_Start_ContextCancel(t *testing.T) {
	idleReader := &mockL1IdleTaskReader{}
	w := newTestWorker(t, 10*time.Millisecond, idleReader, &mockL1TaskWriter{}, &mockL1ExpiredFieldCleaner{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good: goroutine exited after cancel.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Start did not exit after context cancellation")
	}
}
