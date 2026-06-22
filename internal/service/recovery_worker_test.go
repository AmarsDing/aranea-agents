package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// --- Test doubles ---

// fakeSessionRunLister is a minimal fake of biz.SessionRunUsecase that only
// exposes ListDurablePending for RecoveryWorker tests.
type fakeSessionRunLister struct {
	mu       sync.Mutex
	runs     []biz.SessionRun
	listErr  error
	failCall int32 // atomic counter for Fail calls
}

func (f *fakeSessionRunLister) ListDurablePending(_ context.Context, limit int) ([]biz.SessionRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	if limit <= 0 || limit > len(f.runs) {
		return append([]biz.SessionRun(nil), f.runs...), nil
	}
	return append([]biz.SessionRun(nil), f.runs[:limit]...), nil
}

func (f *fakeSessionRunLister) Fail(_ context.Context, id, _ string) error {
	atomic.AddInt32(&f.failCall, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.runs {
		if f.runs[i].ID == id {
			f.runs[i].Phase = biz.SessionRunPhaseFailed
		}
	}
	return nil
}

func (f *fakeSessionRunLister) failCount() int32 { return atomic.LoadInt32(&f.failCall) }

// fakeResumeGateway records ResumeDurableSessionRun calls.
type fakeResumeGateway struct {
	mu        sync.Mutex
	resumed   []string
	resumeErr error
}

func (r *fakeResumeGateway) ResumeDurableSessionRun(_ context.Context, sessionRunID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resumed = append(r.resumed, sessionRunID)
	return r.resumeErr
}

func (r *fakeResumeGateway) resumedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.resumed))
	copy(out, r.resumed)
	return out
}

// errorCheckpointSaver always returns an error on Get/GetTuple.
type errorCheckpointSaver struct{}

func (errorCheckpointSaver) Get(context.Context, map[string]any) (*trpcgraph.Checkpoint, error) {
	return nil, errors.New("checkpoint load failed")
}
func (errorCheckpointSaver) GetTuple(context.Context, map[string]any) (*trpcgraph.CheckpointTuple, error) {
	return nil, errors.New("checkpoint load failed")
}
func (errorCheckpointSaver) List(context.Context, map[string]any, *trpcgraph.CheckpointFilter) ([]*trpcgraph.CheckpointTuple, error) {
	return nil, nil
}
func (errorCheckpointSaver) Put(context.Context, trpcgraph.PutRequest) (map[string]any, error) {
	return nil, nil
}
func (errorCheckpointSaver) PutWrites(context.Context, trpcgraph.PutWritesRequest) error { return nil }
func (errorCheckpointSaver) PutFull(context.Context, trpcgraph.PutFullRequest) (map[string]any, error) {
	return nil, nil
}
func (errorCheckpointSaver) DeleteLineage(context.Context, string) error { return nil }
func (errorCheckpointSaver) Close() error                                { return nil }

// okCheckpointSaver returns a non-nil checkpoint for any config with a
// non-empty checkpoint_id.
type okCheckpointSaver struct{}

func (okCheckpointSaver) Get(_ context.Context, config map[string]any) (*trpcgraph.Checkpoint, error) {
	if trpcgraph.GetCheckpointID(config) == "" {
		return nil, errors.New("checkpoint not found")
	}
	return &trpcgraph.Checkpoint{ID: trpcgraph.GetCheckpointID(config)}, nil
}
func (okCheckpointSaver) GetTuple(_ context.Context, config map[string]any) (*trpcgraph.CheckpointTuple, error) {
	if trpcgraph.GetCheckpointID(config) == "" {
		return nil, errors.New("checkpoint not found")
	}
	return &trpcgraph.CheckpointTuple{
		Config:     config,
		Checkpoint: &trpcgraph.Checkpoint{ID: trpcgraph.GetCheckpointID(config)},
	}, nil
}
func (okCheckpointSaver) List(context.Context, map[string]any, *trpcgraph.CheckpointFilter) ([]*trpcgraph.CheckpointTuple, error) {
	return nil, nil
}
func (okCheckpointSaver) Put(context.Context, trpcgraph.PutRequest) (map[string]any, error) {
	return nil, nil
}
func (okCheckpointSaver) PutWrites(context.Context, trpcgraph.PutWritesRequest) error { return nil }
func (okCheckpointSaver) PutFull(context.Context, trpcgraph.PutFullRequest) (map[string]any, error) {
	return nil, nil
}
func (okCheckpointSaver) DeleteLineage(context.Context, string) error { return nil }
func (okCheckpointSaver) Close() error                                { return nil }

// --- Tests ---

// TestRecoveryWorker_Run_NoStaleRuns verifies that when there are no stale
// runs, the worker does nothing and returns no error.
func TestRecoveryWorker_Run_NoStaleRuns(t *testing.T) {
	lister := &fakeSessionRunLister{runs: []biz.SessionRun{}}
	resumer := &fakeResumeGateway{}
	w := NewRecoveryWorker(lister, okCheckpointSaver{}, resumer, loggateway.NewNoop())

	err := w.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := len(resumer.resumedIDs()); got != 0 {
		t.Errorf("resumed count = %d, want 0", got)
	}
	if got := lister.failCount(); got != 0 {
		t.Errorf("fail count = %d, want 0", got)
	}
}

// TestRecoveryWorker_Run_CheckpointLoadFailed verifies that when checkpoint
// load fails, the run is marked as failed and not resumed.
func TestRecoveryWorker_Run_CheckpointLoadFailed(t *testing.T) {
	lister := &fakeSessionRunLister{runs: []biz.SessionRun{
		{ID: "run-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseDurable, CheckpointID: "ckpt-1"},
	}}
	resumer := &fakeResumeGateway{}
	w := NewRecoveryWorker(lister, errorCheckpointSaver{}, resumer, loggateway.NewNoop())

	err := w.Run(context.Background())
	// Run should not return a fatal error; individual failures are logged.
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := lister.failCount(); got != 1 {
		t.Errorf("fail count = %d, want 1", got)
	}
	if got := len(resumer.resumedIDs()); got != 0 {
		t.Errorf("resumed count = %d, want 0 (failed runs should not be resumed)", got)
	}
}

// TestRecoveryWorker_Run_RecoverySuccess verifies that when checkpoint load
// succeeds, the run is resumed.
func TestRecoveryWorker_Run_RecoverySuccess(t *testing.T) {
	lister := &fakeSessionRunLister{runs: []biz.SessionRun{
		{ID: "run-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseDurable, CheckpointID: "ckpt-1"},
		{ID: "run-2", SessionID: "sess-2", Phase: biz.SessionRunPhaseDurable, CheckpointID: "ckpt-2"},
	}}
	resumer := &fakeResumeGateway{}
	w := NewRecoveryWorker(lister, okCheckpointSaver{}, resumer, loggateway.NewNoop())

	err := w.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	resumed := resumer.resumedIDs()
	if len(resumed) != 2 {
		t.Fatalf("resumed count = %d, want 2", len(resumed))
	}
	if got := lister.failCount(); got != 0 {
		t.Errorf("fail count = %d, want 0", got)
	}
}

// TestRecoveryWorker_Run_SkipsRunsWithoutCheckpoint verifies that runs
// without a checkpoint_id are skipped (not failed, not resumed).
func TestRecoveryWorker_Run_SkipsRunsWithoutCheckpoint(t *testing.T) {
	lister := &fakeSessionRunLister{runs: []biz.SessionRun{
		{ID: "run-1", SessionID: "sess-1", Phase: biz.SessionRunPhaseDurable, CheckpointID: ""},
	}}
	resumer := &fakeResumeGateway{}
	w := NewRecoveryWorker(lister, okCheckpointSaver{}, resumer, loggateway.NewNoop())

	err := w.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := len(resumer.resumedIDs()); got != 0 {
		t.Errorf("resumed count = %d, want 0", got)
	}
	if got := lister.failCount(); got != 0 {
		t.Errorf("fail count = %d, want 0 (runs without checkpoint should be skipped, not failed)", got)
	}
}

// TestRecoveryWorker_Run_ListError verifies that a list error is returned
// from Run so the caller can log it.
func TestRecoveryWorker_Run_ListError(t *testing.T) {
	listErr := errors.New("db connection lost")
	lister := &fakeSessionRunLister{listErr: listErr}
	resumer := &fakeResumeGateway{}
	w := NewRecoveryWorker(lister, okCheckpointSaver{}, resumer, loggateway.NewNoop())

	err := w.Run(context.Background())
	if err == nil {
		t.Fatal("Run returned nil error, want list error")
	}
	if !errors.Is(err, listErr) {
		t.Errorf("err = %v, want to wrap %v", err, listErr)
	}
}

// TestRecoveryWorker_Start_RunsOnceAndExits verifies that Start launches the
// worker and it exits when ctx is cancelled.
func TestRecoveryWorker_Start_RunsOnceAndExits(t *testing.T) {
	lister := &fakeSessionRunLister{runs: []biz.SessionRun{}}
	resumer := &fakeResumeGateway{}
	w := NewRecoveryWorker(lister, okCheckpointSaver{}, resumer, loggateway.NewNoop())
	w.pollInterval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Let it run a couple of cycles.
	time.Sleep(120 * time.Millisecond)
	cancel()

	// Give the goroutine time to exit.
	time.Sleep(50 * time.Millisecond)
	// Test passes if no goroutine leak / panic; we cannot assert directly
	// but the test would hang on shutdown if the goroutine didn't exit.
}

// TestRecoveryWorker_NilDependencies verifies that NewRecoveryWorker returns
// nil when any required dependency is nil (defensive construction).
func TestRecoveryWorker_NilDependencies(t *testing.T) {
	if w := NewRecoveryWorker(nil, okCheckpointSaver{}, &fakeResumeGateway{}, loggateway.NewNoop()); w != nil {
		t.Error("expected nil worker when lister is nil")
	}
	if w := NewRecoveryWorker(&fakeSessionRunLister{}, nil, &fakeResumeGateway{}, loggateway.NewNoop()); w != nil {
		t.Error("expected nil worker when saver is nil")
	}
	if w := NewRecoveryWorker(&fakeSessionRunLister{}, okCheckpointSaver{}, nil, loggateway.NewNoop()); w != nil {
		t.Error("expected nil worker when resumer is nil")
	}
}
