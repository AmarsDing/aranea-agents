package biz

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// F4：并发 ensureCheckpointRuntime（TimeTravel/EditState/ListCheckpoints 与
// Resume 竞速）必须只安装一个 runtime；败构建的 runtime 必须被 Cancel 释放，
// 不得泄漏 runner/事件通道。

type raceCheckpointRuntime struct {
	cancels *int32
}

func (r *raceCheckpointRuntime) Run(context.Context, map[string]any) (<-chan GraphRuntimeEvent, error) {
	ch := make(chan GraphRuntimeEvent)
	close(ch)
	return ch, nil
}
func (r *raceCheckpointRuntime) Resume(context.Context, string, map[string]any) (<-chan GraphRuntimeEvent, error) {
	ch := make(chan GraphRuntimeEvent)
	close(ch)
	return ch, nil
}
func (r *raceCheckpointRuntime) Cancel() error {
	atomic.AddInt32(r.cancels, 1)
	return nil
}
func (r *raceCheckpointRuntime) GetLineageID() string { return "lin-race" }
func (r *raceCheckpointRuntime) TimeTravelGetState(context.Context, string, string, string) (*GraphCheckpointState, error) {
	return nil, nil
}
func (r *raceCheckpointRuntime) TimeTravelHistory(context.Context, string, string, int) (GraphCheckpointList, error) {
	return nil, nil
}
func (r *raceCheckpointRuntime) TimeTravelEditState(context.Context, string, string, string, map[string]any) (*GraphEditedState, error) {
	return nil, nil
}
func (r *raceCheckpointRuntime) ListCheckpoints(context.Context, string, string, int) (GraphCheckpointList, error) {
	return nil, nil
}

type raceCheckpointFactory struct {
	built   *int32
	cancels *int32
}

func (f *raceCheckpointFactory) BuildAndRun(context.Context, GraphBuildConfig, string, string, string, string, map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error) {
	return nil, nil, errRaceFactoryUnsupported
}
func (f *raceCheckpointFactory) BuildAndResume(context.Context, GraphBuildConfig, string, string, string, string, string, map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error) {
	return nil, nil, errRaceFactoryUnsupported
}
func (f *raceCheckpointFactory) BuildRuntime(context.Context, GraphBuildConfig, string, string, string, string, string) (GraphRuntime, error) {
	atomic.AddInt32(f.built, 1)
	return &raceCheckpointRuntime{cancels: f.cancels}, nil
}
func (f *raceCheckpointFactory) HasCheckpoint(context.Context, string) (bool, error) { return true, nil }

var errRaceFactoryUnsupported = errors.New("race factory supports BuildRuntime only")

func TestEnsureCheckpointRuntime_ConcurrentBuilds_InstallsOnce(t *testing.T) {
	built := int32(0)
	cancels := int32(0)
	factory := &raceCheckpointFactory{built: &built, cancels: &cancels}
	cacheMgr := NewGraphCacheManager(nil, nil, nil, loggateway.NewNoop())
	execID := "exec-race-runtime"
	cacheMgr.SetTeamBuildConfig(execID, NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil))
	uc := NewGraphExecutionUsecase(nil, factory, nil, cacheMgr, nil, loggateway.NewNoop(), DefaultGraphGCConfig())

	exec := NewGraphExecution(context.Background(), execID, "g-1", "sess-1", string(GraphExecRunning))
	exec.LineageID = "lin-1"

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- uc.ensureCheckpointRuntime(context.Background(), exec)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ensureCheckpointRuntime err=%v", err)
		}
	}

	b := atomic.LoadInt32(&built)
	c := atomic.LoadInt32(&cancels)
	if b < 1 {
		t.Fatal("runtime never built")
	}
	// 不变式：安装 1 个，其余全部 Cancel 释放（双检锁保证）。
	if c != b-1 {
		t.Fatalf("built=%d cancelled=%d: loser runtimes must be cancelled exactly built-1 times", b, c)
	}
	exec.execMu.RLock()
	installed := exec.runtime != nil
	exec.execMu.RUnlock()
	if !installed {
		t.Fatal("exec.runtime not installed")
	}
}
