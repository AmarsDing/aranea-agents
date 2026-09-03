package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── RecoverOrphanedExecution（83-长时运行韧性）────────────────────────────────

type fakeRecoverRuntime struct{}

func (r *fakeRecoverRuntime) Run(context.Context, map[string]any) (<-chan GraphRuntimeEvent, error) {
	ch := make(chan GraphRuntimeEvent)
	close(ch)
	return ch, nil
}
func (r *fakeRecoverRuntime) Resume(context.Context, string, map[string]any) (<-chan GraphRuntimeEvent, error) {
	ch := make(chan GraphRuntimeEvent)
	close(ch)
	return ch, nil
}
func (r *fakeRecoverRuntime) Cancel() error        { return nil }
func (r *fakeRecoverRuntime) GetLineageID() string { return "lin-fake" }
func (r *fakeRecoverRuntime) TimeTravelGetState(context.Context, string, string, string) (*GraphCheckpointState, error) {
	return nil, nil
}
func (r *fakeRecoverRuntime) TimeTravelHistory(context.Context, string, string, int) (GraphCheckpointList, error) {
	return nil, nil
}
func (r *fakeRecoverRuntime) TimeTravelEditState(context.Context, string, string, string, map[string]any) (*GraphEditedState, error) {
	return nil, nil
}
func (r *fakeRecoverRuntime) ListCheckpoints(context.Context, string, string, int) (GraphCheckpointList, error) {
	return nil, nil
}

var _ GraphRuntime = (*fakeRecoverRuntime)(nil)

type fakeRecoverFactory struct {
	hasCheckpoint bool
	checkpointErr error
	buildErr      error
	gotLineage    string
	gotResumeNil  bool
	built         bool
}

func (f *fakeRecoverFactory) BuildAndRun(context.Context, GraphBuildConfig, string, string, string, string, map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error) {
	return nil, nil, errors.New("not implemented")
}
func (f *fakeRecoverFactory) BuildAndResume(_ context.Context, _ GraphBuildConfig, _, _, _, _, lineageID string, resumeValue map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error) {
	f.built = true
	f.gotLineage = lineageID
	f.gotResumeNil = resumeValue == nil
	if f.buildErr != nil {
		return nil, nil, f.buildErr
	}
	ch := make(chan GraphRuntimeEvent)
	close(ch)
	return &fakeRecoverRuntime{}, ch, nil
}
func (f *fakeRecoverFactory) BuildRuntime(context.Context, GraphBuildConfig, string, string, string, string, string) (GraphRuntime, error) {
	return &fakeRecoverRuntime{}, nil
}
func (f *fakeRecoverFactory) HasCheckpoint(_ context.Context, lineageID string) (bool, error) {
	return f.hasCheckpoint, f.checkpointErr
}

var _ GraphRunnerFactory = (*fakeRecoverFactory)(nil)

func newRecoverTestUsecase(repo GraphRunRepo, factory GraphRunnerFactory, ct *CompiledTeam, execID string) *GraphExecutionUsecase {
	cacheMgr := NewGraphCacheManager(nil, nil, nil, loggateway.NewNoop())
	if ct != nil {
		cacheMgr.SetTeamBuildConfig(execID, ct)
	}
	return NewGraphExecutionUsecase(repo, factory, nil, cacheMgr, nil, loggateway.NewNoop(), DefaultGraphGCConfig())
}

func runningRecoverExec(id string) *GraphExecution {
	return &GraphExecution{
		ID:              id,
		GraphID:         "team:t1",
		SessionID:       "sess-1",
		SpiritSessionID: "spirit-1",
		Status:          string(GraphExecRunning),
		LineageID:       "lin-1",
		ctx:             context.Background(),
	}
}

func TestRecoverOrphanedExecution_NoCheckpointRejected(t *testing.T) {
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{"exec-1": runningRecoverExec("exec-1")}}
	factory := &fakeRecoverFactory{hasCheckpoint: false}
	uc := newRecoverTestUsecase(repo, factory, NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil), "exec-1")

	_, err := uc.RecoverOrphanedExecution(context.Background(), "exec-1")
	if !errors.Is(err, ErrGraphCheckpointMissing) {
		t.Fatalf("err=%v, want ErrGraphCheckpointMissing", err)
	}
	if factory.built {
		t.Fatal("BuildAndResume must not be called without checkpoint")
	}
	// 状态不被改写：保持 running 由原调用方决定回退。
	exec := repo.runs["exec-1"]
	if exec.Status != string(GraphExecRunning) {
		t.Fatalf("status=%q, want running unchanged", exec.Status)
	}
}

func TestRecoverOrphanedExecution_EmptyLineageRejected(t *testing.T) {
	exec := runningRecoverExec("exec-1")
	exec.LineageID = ""
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{"exec-1": exec}}
	factory := &fakeRecoverFactory{hasCheckpoint: true}
	uc := newRecoverTestUsecase(repo, factory, nil, "exec-1")

	_, err := uc.RecoverOrphanedExecution(context.Background(), "exec-1")
	if !errors.Is(err, ErrGraphCheckpointMissing) {
		t.Fatalf("err=%v, want ErrGraphCheckpointMissing", err)
	}
}

func TestRecoverOrphanedExecution_NonRunningRejected(t *testing.T) {
	exec := runningRecoverExec("exec-1")
	exec.Status = string(GraphExecWaitingHuman)
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{"exec-1": exec}}
	factory := &fakeRecoverFactory{hasCheckpoint: true}
	uc := newRecoverTestUsecase(repo, factory, nil, "exec-1")

	_, err := uc.RecoverOrphanedExecution(context.Background(), "exec-1")
	if !errors.Is(err, ErrGraphInvalidStatus) {
		t.Fatalf("err=%v, want ErrGraphInvalidStatus", err)
	}
	if factory.built {
		t.Fatal("BuildAndResume must not be called for non-running status")
	}
}

func TestRecoverOrphanedExecution_HashMismatchRejected(t *testing.T) {
	exec := runningRecoverExec("exec-1")
	exec.DefinitionHash = "stale-hash"
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{"exec-1": exec}}
	factory := &fakeRecoverFactory{hasCheckpoint: true}
	uc := newRecoverTestUsecase(repo, factory, NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil), "exec-1")

	_, err := uc.RecoverOrphanedExecution(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected hash mismatch error")
	}
	if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("err=%v, want apierror Conflict", err)
	}
	if factory.built {
		t.Fatal("BuildAndResume must not be called on hash mismatch")
	}
	if exec.Status != string(GraphExecRunning) {
		t.Fatalf("status=%q, want running unchanged", exec.Status)
	}
}

func TestRecoverOrphanedExecution_Success(t *testing.T) {
	ct := NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil)
	exec := runningRecoverExec("exec-1")
	exec.DefinitionHash = ComputeGraphBuildConfigHash(ct.GraphBuildConfig)
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{"exec-1": exec}}
	factory := &fakeRecoverFactory{hasCheckpoint: true}
	uc := newRecoverTestUsecase(repo, factory, ct, "exec-1")

	got, err := uc.RecoverOrphanedExecution(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.runtime == nil {
		t.Fatal("runtime not rebuilt")
	}
	if got.Status != string(GraphExecRunning) {
		t.Fatalf("status=%q, want running", got.Status)
	}
	if factory.gotLineage != "lin-1" {
		t.Fatalf("lineage=%q, want lin-1", factory.gotLineage)
	}
	if !factory.gotResumeNil {
		t.Fatal("resumeValue must be nil for crash recover")
	}
}
